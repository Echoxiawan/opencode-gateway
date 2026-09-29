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

// ResponsesStreamWriter renders unified events as an OpenAI Responses API SSE
// stream (the format Codex CLI consumes):
//
//	response.created → response.output_item.added → response.output_text.delta*
//	→ response.output_item.done → response.completed
type ResponsesStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	respID  string
	model   string
	seq     int

	textStarted   bool
	reasonStarted bool
	toolIdx       int
	usage         Usage
	finish        string
}

// NewResponsesStreamWriter writes the response.created event.
func NewResponsesStreamWriter(w http.ResponseWriter, model string) *ResponsesStreamWriter {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	sw := &ResponsesStreamWriter{
		w:       w,
		flusher: flusher,
		respID:  "resp_gw_" + fmt.Sprintf("%d", time.Now().UnixNano()),
		model:   model,
	}
	sw.emit("response.created", map[string]any{
		"type":     "response.created",
		"response": sw.responseShell(nil),
	})
	return sw
}

func (sw *ResponsesStreamWriter) responseShell(status *string) map[string]any {
	st := "in_progress"
	if status != nil {
		st = *status
	}
	return map[string]any{
		"id":         sw.respID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     st,
		"model":      sw.model,
		"output":     []any{},
		"usage":      nil,
	}
}

func (sw *ResponsesStreamWriter) nextSeq() int {
	sw.seq++
	return sw.seq
}

func (sw *ResponsesStreamWriter) emit(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(sw.w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return nil
}

// WriteEvent converts one unified event to the Responses event sequence.
func (sw *ResponsesStreamWriter) WriteEvent(ev UnifiedStreamEvent) error {
	switch ev.Type {
	case "reasoning_delta":
		if !sw.reasonStarted {
			sw.reasonStarted = true
			_ = sw.emit("response.output_item.added", map[string]any{
				"type":         "response.output_item.added",
				"output_index": 0,
				"item": map[string]any{
					"type": "reasoning",
					"id":   "rs_gw_0",
				},
			})
		}
		return sw.emit("response.reasoning_summary_text.delta", map[string]any{
			"type":         "response.reasoning_summary_text.delta",
			"item_id":      "rs_gw_0",
			"output_index": 0,
			"delta":        ev.Text,
		})
	case "text_delta":
		if !sw.textStarted {
			if sw.reasonStarted {
				_ = sw.emit("response.output_item.done", map[string]any{
					"type":         "response.output_item.done",
					"output_index": 0,
					"item":         map[string]any{"type": "reasoning", "id": "rs_gw_0"},
				})
			}
			sw.textStarted = true
			_ = sw.emit("response.output_item.added", map[string]any{
				"type":         "response.output_item.added",
				"output_index": 1,
				"item": map[string]any{
					"type":    "message",
					"id":      "msg_gw_0",
					"role":    "assistant",
					"status":  "in_progress",
					"content": []any{},
				},
			})
			_ = sw.emit("response.content_part.added", map[string]any{
				"type":          "response.content_part.added",
				"item_id":       "msg_gw_0",
				"output_index":  1,
				"content_index": 0,
				"part":          map[string]any{"type": "output_text", "text": ""},
			})
		}
		return sw.emit("response.output_text.delta", map[string]any{
			"type":          "response.output_text.delta",
			"item_id":       "msg_gw_0",
			"output_index":  1,
			"content_index": 0,
			"delta":         ev.Text,
		})
	case "tool_call_delta":
		if ev.ToolCall == nil {
			return nil
		}
		if ev.ToolCall.ID != "" {
			// announce the function_call item
			outIdx := 2 + ev.ToolCall.Index
			_ = sw.emit("response.output_item.added", map[string]any{
				"type":         "response.output_item.added",
				"output_index": outIdx,
				"item": map[string]any{
					"type":      "function_call",
					"id":        "fc_gw_" + fmt.Sprintf("%d", ev.ToolCall.Index),
					"call_id":   ev.ToolCall.ID,
					"name":      ev.ToolCall.Name,
					"arguments": "",
					"status":    "in_progress",
				},
			})
			return nil
		}
		if ev.ToolCall.ArgsDelta != "" {
			return sw.emit("response.function_call_arguments.delta", map[string]any{
				"type":         "response.function_call_arguments.delta",
				"item_id":      "fc_gw_" + fmt.Sprintf("%d", ev.ToolCall.Index),
				"output_index": 2 + ev.ToolCall.Index,
				"delta":        ev.ToolCall.ArgsDelta,
			})
		}
		return nil
	case "usage":
		if ev.Usage != nil {
			sw.usage = *ev.Usage
		}
		return nil
	case "finish":
		sw.finish = ev.Finish
		return nil
	}
	return nil
}

// Finish emits the closing events.
func (sw *ResponsesStreamWriter) Finish() {
	if sw.textStarted {
		_ = sw.emit("response.output_text.done", map[string]any{
			"type":          "response.output_text.done",
			"item_id":       "msg_gw_0",
			"output_index":  1,
			"content_index": 0,
			"text":          "",
		})
		_ = sw.emit("response.content_part.done", map[string]any{
			"type":          "response.content_part.done",
			"item_id":       "msg_gw_0",
			"output_index":  1,
			"content_index": 0,
			"part":          map[string]any{"type": "output_text", "text": ""},
		})
		_ = sw.emit("response.output_item.done", map[string]any{
			"type":         "response.output_item.done",
			"output_index": 1,
			"item": map[string]any{
				"type": "message", "id": "msg_gw_0", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": ""}},
			},
		})
	}
	completed := "completed"
	_ = sw.emit("response.completed", map[string]any{
		"type":     "response.completed",
		"response": sw.responseShell(&completed),
	})
}

// WriteError emits an error event.
func (sw *ResponsesStreamWriter) WriteError(msg string) {
	_ = sw.emit("response.failed", map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":     sw.respID,
			"object": "response",
			"status": "failed",
			"error":  map[string]any{"code": "gateway_error", "message": msg},
		},
	})
}

// WriteResponsesJSONError writes a non-streaming Responses error.
func WriteResponsesJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "gateway_error",
			"message": msg,
		},
	})
}

// RenderResponsesMessage renders the aggregate as a non-streaming Responses
// response object.
func RenderResponsesMessage(w http.ResponseWriter, model string, agg *AggregateChatResponse) error {
	var output []map[string]any
	if agg.Reason.Len() > 0 {
		output = append(output, map[string]any{
			"type": "reasoning",
			"id":   "rs_gw_0",
			"summary": []any{map[string]any{
				"type": "summary_text",
				"text": agg.Reason.String(),
			}},
		})
	}
	msgContent := []any{}
	if agg.Content.Len() > 0 || len(agg.ToolCalls) == 0 {
		msgContent = append(msgContent, map[string]any{
			"type": "output_text",
			"text": agg.Content.String(),
		})
	}
	output = append(output, map[string]any{
		"type":    "message",
		"id":      "msg_gw_0",
		"role":    "assistant",
		"status":  "completed",
		"content": msgContent,
	})
	for i, tc := range agg.ToolCalls {
		args := strings.TrimSpace(tc.Arguments.String())
		if args == "" {
			args = "{}"
		}
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        fmt.Sprintf("fc_gw_%d", i),
			"call_id":   orDefault(tc.ID, fmt.Sprintf("call_gw_%d", i)),
			"name":      tc.Name,
			"arguments": args,
			"status":    "completed",
		})
	}
	resp := map[string]any{
		"id":         orDefault(agg.ID, "resp_gw"),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      model,
		"output":     output,
		"usage": map[string]any{
			"input_tokens":  agg.Usage.InputTokens,
			"output_tokens": agg.Usage.OutputTokens,
			"total_tokens":  agg.Usage.InputTokens + agg.Usage.OutputTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}
