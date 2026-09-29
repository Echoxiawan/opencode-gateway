// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AnthropicStreamWriter renders unified events as an Anthropic Messages SSE
// stream with the official event sequence:
//
//	message_start → content_block_start → content_block_delta* →
//	content_block_stop → message_delta → message_stop
type AnthropicStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	id      string
	model   string
	started bool

	blockIdx        int
	openBlock       string // "" | "text" | "thinking" | "tool_use"
	toolIndex       int
	currentToolID   string
	currentToolName string
	currentToolIdx  int  // index of the open tool_use block
	currentToolOpen bool // a tool_use block was opened and can take more args
	toolEmitted     bool // at least one tool_use block was sent downstream
	toolArgsBuf     string
	// Reasoning that arrived while a tool_use block was open. It cannot be
	// emitted there — a thinking block would close the tool_use block and
	// leave its arguments truncated — so it is held and flushed once the tool
	// is done.
	pendingReasoning strings.Builder
	usage            Usage
	msgID            string
}

// NewAnthropicStreamWriter sends the message_start event.
func NewAnthropicStreamWriter(w http.ResponseWriter, model string) *AnthropicStreamWriter {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	sw := &AnthropicStreamWriter{
		w:       w,
		flusher: flusher,
		model:   model,
		msgID:   "msg_gw_" + fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	_ = WriteSSEEvent(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            sw.msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":                0,
				"output_tokens":               0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			},
		},
	})
	if flusher != nil {
		flusher.Flush()
	}
	return sw
}

// WriteEvent converts one unified event into the Anthropic event sequence.
func (sw *AnthropicStreamWriter) WriteEvent(ev UnifiedStreamEvent) error {
	switch ev.Type {
	case "reasoning_delta":
		// A reasoning delta cannot interrupt an open tool_use block: opening a
		// thinking block would close it, and the tool's remaining argument
		// fragments would then have nowhere to go. Hold the text until the
		// tool finishes (see flushPendingReasoning).
		if sw.currentToolOpen {
			sw.pendingReasoning.WriteString(ev.Text)
			return nil
		}
		sw.openBlockKind("thinking")
		return sw.emit(map[string]any{
			"type":  "content_block_delta",
			"index": sw.blockIdx,
			"delta": map[string]any{"type": "thinking_delta", "thinking": ev.Text},
		})
	case "text_delta":
		sw.openBlockKind("text")
		return sw.emit(map[string]any{
			"type":  "content_block_delta",
			"index": sw.blockIdx,
			"delta": map[string]any{"type": "text_delta", "text": ev.Text},
		})
	case "tool_call_delta":
		if ev.ToolCall == nil {
			return nil
		}
		if ev.ToolCall.ID != "" && ev.ToolCall.ID != sw.currentToolID {
			// New tool call: close previous, open a tool_use block.
			sw.closeBlock()
			sw.openBlockKind("tool_use")
			sw.currentToolID = ev.ToolCall.ID
			sw.currentToolName = ev.ToolCall.Name
			sw.currentToolIdx = sw.blockIdx
			sw.currentToolOpen = true
			sw.toolEmitted = true
			sw.toolArgsBuf = ""
			_ = WriteSSEEvent(sw.w, "content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": sw.blockIdx,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    ev.ToolCall.ID,
					"name":  ev.ToolCall.Name,
					"input": map[string]any{},
				},
			})
		}
		if ev.ToolCall.ArgsDelta != "" {
			// Arguments belong to the tool's own block, addressed by the index
			// it was opened at — not by whichever block happens to be open now.
			// Upstream streams the call in pieces (id+name first, then bare
			// argument fragments) and a reasoning model can slot a
			// reasoning_delta between them; addressing by "currently open
			// block" then either drops the later fragments or lands them on the
			// thinking block, and the client ends up assembling truncated JSON
			// ("The model's tool call could not be parsed") or rejecting the
			// frame outright ("Content block is not a input_json block").
			if !sw.currentToolOpen {
				// No tool_use block was ever opened for this stream: the call
				// was a filtered signature tool, whose fragments carry no id.
				return nil
			}
			sw.toolArgsBuf += ev.ToolCall.ArgsDelta
			return sw.emit(map[string]any{
				"type":  "content_block_delta",
				"index": sw.currentToolIdx,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": ev.ToolCall.ArgsDelta,
				},
			})
		}
		return nil
	case "usage":
		if ev.Usage != nil {
			sw.usage = *ev.Usage
		}
		return nil
	case "finish":
		// Handled in Finish.
		return nil
	}
	return nil
}

// openBlockKind opens a content block of the given kind if not already open.
// Each block type carries only its own fields — a text block has no
// "thinking" key and vice versa — since strict clients reject mixed shapes.
func (sw *AnthropicStreamWriter) openBlockKind(kind string) {
	if sw.openBlock == kind {
		return
	}
	sw.closeBlock()
	sw.openBlock = kind
	if kind == "tool_use" {
		return // tool_use blocks open on first tool_call event with an ID
	}
	block := map[string]any{"type": kind}
	switch kind {
	case "thinking":
		block["thinking"] = ""
		block["signature"] = ""
	default:
		block["text"] = ""
	}
	_ = WriteSSEEvent(sw.w, "content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         sw.blockIdx,
		"content_block": block,
	})
}

// closeBlock emits content_block_stop for the open block.
func (sw *AnthropicStreamWriter) closeBlock() {
	if sw.openBlock == "" {
		return
	}
	_ = WriteSSEEvent(sw.w, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": sw.blockIdx,
	})
	sw.blockIdx++
	sw.openBlock = ""
	if sw.currentToolOpen {
		// The tool's arguments are complete the moment its block closes.
		sw.currentToolOpen = false
		sw.flushPendingReasoning()
	}
}

// flushPendingReasoning emits reasoning that was held back while a tool_use
// block was open, as its own thinking block.
func (sw *AnthropicStreamWriter) flushPendingReasoning() {
	if sw.pendingReasoning.Len() == 0 {
		return
	}
	text := sw.pendingReasoning.String()
	sw.pendingReasoning.Reset()
	sw.openBlockKind("thinking")
	_ = sw.emit(map[string]any{
		"type":  "content_block_delta",
		"index": sw.blockIdx,
		"delta": map[string]any{"type": "thinking_delta", "thinking": text},
	})
	sw.closeBlock()
}

// Finish closes open blocks and emits message_delta + message_stop.
func (sw *AnthropicStreamWriter) Finish(stopReason string) {
	sw.closeBlock()
	sw.flushPendingReasoning()
	if stopReason == "" {
		stopReason = "end_turn"
	}
	// Map OpenAI finish reasons to Anthropic stop reasons.
	switch stopReason {
	case "stop":
		stopReason = "end_turn"
	case "length":
		stopReason = "max_tokens"
	case "tool_calls", "function_calls":
		// A tool stop with no tool_use block is a phantom: upstream said the
		// model called a tool, but the only call it made was to a filtered
		// signature tool (bash/read/edit/glob/grep), so nothing was sent. The
		// client then hunts for a tool call that does not exist and aborts with
		// "The model's tool call could not be parsed".
		if sw.toolEmitted {
			stopReason = "tool_use"
		} else {
			stopReason = "end_turn"
		}
	}
	_ = WriteSSEEvent(sw.w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": sw.usage.OutputTokens,
		},
	})
	_ = WriteSSEEvent(sw.w, "message_stop", map[string]any{"type": "message_stop"})
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

// WriteError emits an Anthropic-style error event.
func (sw *AnthropicStreamWriter) WriteError(msg string) {
	_ = WriteSSEEvent(sw.w, "error", map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "gateway_error", "message": msg},
	})
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

// WriteAnthropicJSONError writes a non-streaming Anthropic error.
func WriteAnthropicJSONError(w http.ResponseWriter, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    typ,
			"message": msg,
		},
	})
}

// emit writes one Anthropic SSE event. The event name must be present on
// every frame — clients dispatch on it, and a delta frame with only a data
// line is silently dropped (the response then renders as empty).
func (sw *AnthropicStreamWriter) emit(ev map[string]any) error {
	name, _ := ev["type"].(string)
	if err := WriteSSEEvent(sw.w, name, ev); err != nil {
		return err
	}
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return nil
}
