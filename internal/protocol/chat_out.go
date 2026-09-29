// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ChatStreamWriter renders unified events as an OpenAI Chat SSE stream.
type ChatStreamWriter struct {
	w          http.ResponseWriter
	flusher    http.Flusher
	id         string
	model      string
	sentRole   bool
	sentFinish bool
	toolIdx    map[int]bool
	// toolEmitted records whether any tool_calls chunk was sent, so a tool
	// finish_reason is not passed through when nothing backs it up.
	toolEmitted bool
}

// NewChatStreamWriter creates the SSE writer and writes headers.
func NewChatStreamWriter(w http.ResponseWriter, id, model string) *ChatStreamWriter {
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &ChatStreamWriter{w: w, flusher: flusher, id: id, model: model, toolIdx: map[int]bool{}}
}

func nowUnix() int64 { return time.Now().Unix() }

// WriteEvent emits one SSE chunk for the given unified event.
func (cw *ChatStreamWriter) WriteEvent(ev UnifiedStreamEvent) error {
	chunk := map[string]any{
		"id":      orDefault(cw.id, "chatcmpl-gw"),
		"object":  "chat.completion.chunk",
		"created": nowUnix(),
		"model":   cw.model,
		"choices": []map[string]any{},
	}
	delta := map[string]any{}
	finish := ""

	switch ev.Type {
	case "text_delta":
		if !cw.sentRole {
			delta["role"] = "assistant"
			cw.sentRole = true
		}
		delta["content"] = ev.Text
	case "reasoning_delta":
		if !cw.sentRole {
			delta["role"] = "assistant"
			cw.sentRole = true
		}
		delta["reasoning_content"] = ev.Text
	case "tool_call_delta":
		if ev.ToolCall == nil {
			return nil
		}
		tc := map[string]any{
			"index":    ev.ToolCall.Index,
			"function": map[string]any{},
		}
		if ev.ToolCall.ID != "" {
			tc["id"] = ev.ToolCall.ID
		}
		if ev.ToolCall.Name != "" {
			tc["type"] = "function"
			tc["function"].(map[string]any)["name"] = ev.ToolCall.Name
		}
		if ev.ToolCall.ArgsDelta != "" {
			tc["function"].(map[string]any)["arguments"] = ev.ToolCall.ArgsDelta
		}
		delta["tool_calls"] = []any{tc}
		cw.toolEmitted = true
	case "finish":
		if cw.sentFinish {
			return nil // upstream may repeat the finish signal; emit once
		}
		cw.sentFinish = true
		finish = ev.Finish
		if finish == "" {
			finish = "stop"
		}
		// A tool stop with no tool_calls chunk is a phantom: upstream said the
		// model called a tool, but the only call it made was to a filtered
		// signature tool, so none was forwarded. Clients then wait for a tool
		// call that never arrives.
		if !cw.toolEmitted && (finish == "tool_calls" || finish == "function_calls") {
			finish = "stop"
		}
		delta["role"] = "assistant"
	case "usage":
		// OpenAI emits usage in the final chunk (with stream_options).
		// We fold it into the finish chunk; if no finish came, emit alone.
		return nil // handled at FlushFinal
	}

	if len(delta) > 0 || finish != "" {
		choice := map[string]any{
			"index":         0,
			"delta":         delta,
			"finish_reason": nil,
		}
		if finish != "" {
			choice["finish_reason"] = finish
		}
		chunk["choices"] = []map[string]any{choice}
		if err := WriteSSE(cw.w, chunk); err != nil {
			return err
		}
		if cw.flusher != nil {
			cw.flusher.Flush()
		}
	}
	return nil
}

// WriteUsageAndDone emits the final usage chunk and [DONE] sentinel.
func (cw *ChatStreamWriter) WriteUsageAndDone(u Usage, finish string) error {
	if !cw.toolEmitted && (finish == "tool_calls" || finish == "function_calls") {
		finish = "stop"
	}
	delta := map[string]any{"role": "assistant", "content": ""}
	var fr any
	if !cw.sentFinish {
		fr = orDefault(finish, "stop")
	}
	choice := map[string]any{
		"index":         0,
		"delta":         delta,
		"finish_reason": fr,
	}
	chunk := map[string]any{
		"id":      orDefault(cw.id, "chatcmpl-gw"),
		"object":  "chat.completion.chunk",
		"created": nowUnix(),
		"model":   cw.model,
		"choices": []map[string]any{choice},
	}
	chunk["usage"] = map[string]any{
		"prompt_tokens":     u.InputTokens,
		"completion_tokens": u.OutputTokens,
		"total_tokens":      u.InputTokens + u.OutputTokens,
	}
	if err := WriteSSE(cw.w, chunk); err != nil {
		return err
	}
	if _, err := fmt.Fprint(cw.w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	if cw.flusher != nil {
		cw.flusher.Flush()
	}
	return nil
}

// WriteError emits an error chunk (mid-stream errors).
func (cw *ChatStreamWriter) WriteError(msg string) {
	_ = WriteSSE(cw.w, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "gateway_error",
		},
	})
	if _, err := fmt.Fprint(cw.w, "data: [DONE]\n\n"); err == nil {
		if cw.flusher != nil {
			cw.flusher.Flush()
		}
	}
}

// WriteJSONError writes a non-streaming error response.
func WriteJSONError(w http.ResponseWriter, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    typ,
			"code":    nil,
		},
	})
}
