// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ParseChatRequest converts an OpenAI Chat Completions request body into the
// unified representation.
func ParseChatRequest(body []byte) (*UnifiedRequest, error) {
	var raw struct {
		Model       string            `json:"model"`
		Stream      bool              `json:"stream"`
		Temperature *float64          `json:"temperature"`
		TopP        *float64          `json:"top_p"`
		MaxTokens   int               `json:"max_tokens"`
		Messages    []json.RawMessage `json:"messages"`
		// Chat Completions has no session field, but OpenAI-compatible clients
		// sprinkle one of these in: `user` is the official end-user field, and
		// metadata/session_id is a common extension.
		User     string `json:"user"`
		Metadata *struct {
			SessionID string `json:"session_id"`
		} `json:"metadata"`
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse chat request: %w", err)
	}
	req := &UnifiedRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		Temperature: raw.Temperature,
		TopP:        raw.TopP,
		MaxTokens:   raw.MaxTokens,
	}
	if raw.Metadata != nil {
		req.SessionHint = compactSession(raw.Metadata.SessionID)
	}
	if req.SessionHint == "" {
		req.SessionHint = compactSession(raw.User)
	}
	var sysParts []string
	for _, rm := range raw.Messages {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(rm, &m); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		um := UnifiedMessage{Role: m.Role}
		// content may be a string or an array of parts
		var textStr string
		if err := json.Unmarshal(m.Content, &textStr); err == nil {
			um.Content = append(um.Content, ContentBlock{Type: "text", Text: textStr})
		} else {
			var parts []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL *struct {
					URL    string `json:"url"`
					Detail string `json:"detail"`
				} `json:"image_url"`
			}
			if err := json.Unmarshal(m.Content, &parts); err != nil {
				return nil, fmt.Errorf("parse message content: %w", err)
			}
			for _, p := range parts {
				switch p.Type {
				case "text":
					um.Content = append(um.Content, ContentBlock{Type: "text", Text: p.Text})
				case "image_url":
					if p.ImageURL != nil {
						media, data := parseDataURL(p.ImageURL.URL)
						um.Content = append(um.Content, ContentBlock{Type: "image", MediaType: media, Data: data})
					}
				}
			}
		}
		// Tool calls live in assistant messages as tool_calls array.
		var extras struct {
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal(rm, &extras); err == nil {
			for _, tc := range extras.ToolCalls {
				um.Content = append(um.Content, ContentBlock{
					Type: "tool_call", ToolCallID: tc.ID, ToolName: tc.Function.Name, Text: tc.Function.Arguments,
				})
			}
			if m.Role == "tool" && extras.ToolCallID != "" {
				// find tool name from prior assistant tool_call — not tracked here;
				// the tool role message content is the result.
				um.Content = append(um.Content, ContentBlock{Type: "tool_result", ToolUseID: extras.ToolCallID, Text: textStr})
			}
		}
		if m.Role == "system" {
			sysParts = append(sysParts, textStr)
			continue
		}
		req.Messages = append(req.Messages, um)
	}
	req.System = strings.Join(sysParts, "\n\n")
	for _, t := range raw.Tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		req.Tools = append(req.Tools, Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  string(t.Function.Parameters),
		})
	}
	return req, nil
}

// parseDataURL extracts mime type and base64 data from a data: URL.
func parseDataURL(u string) (media, data string) {
	if !strings.HasPrefix(u, "data:") {
		return "image/png", ""
	}
	rest := strings.TrimPrefix(u, "data:")
	semi := strings.Index(rest, ",")
	if semi < 0 {
		return "image/png", ""
	}
	return rest[:semi], rest[semi+1:]
}

// BuildChatBody renders the unified request as an upstream chat/completions
// JSON body.
func BuildChatBody(req *UnifiedRequest) map[string]any {
	body := map[string]any{
		"model": req.Model,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	var msgs []map[string]any
	if req.System != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, renderChatMessage(m)...)
	}
	// Upstream rejects an empty messages array; keep a placeholder so the
	// error surfaces as a normal API error instead of a malformed request.
	if len(msgs) == 0 {
		msgs = append(msgs, map[string]any{"role": "user", "content": ""})
	}
	body["messages"] = msgs
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			var params any
			if t.Parameters != "" && t.Parameters != "null" {
				if err := json.Unmarshal([]byte(t.Parameters), &params); err != nil {
					params = map[string]any{"type": "object", "properties": map[string]any{}}
				}
			} else {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  params,
				},
			})
		}
		body["tools"] = tools
	}
	return body
}

// renderChatMessage converts one unified message into one or more chat wire
// messages. Tool results become their own role:"tool" messages (chat
// completions carries them outside the assistant turn), so a unified message
// holding both assistant text and tool results expands into several.
//
// Every emitted message carries an explicit "content" field: upstream rejects
// content parts that lack a "text" string, and omitting content entirely
// produces "got undefined" errors.
func renderChatMessage(m UnifiedMessage) []map[string]any {
	var out []map[string]any
	var texts []string
	var parts []map[string]any // multimodal parts (text + image), order preserved
	var toolCalls []map[string]any
	var toolResults []map[string]any

	for _, c := range m.Content {
		switch c.Type {
		case "text":
			if c.Text == "" {
				continue
			}
			texts = append(texts, c.Text)
			parts = append(parts, map[string]any{"type": "text", "text": c.Text})
		case "image":
			dataURL := "data:" + c.MediaType + ";base64," + c.Data
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": dataURL},
			})
		case "thinking":
			// Reasoning is not replayed upstream.
		case "tool_call":
			args := c.Text
			if strings.TrimSpace(args) == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   c.ToolCallID,
				"type": "function",
				"function": map[string]any{
					"name":      c.ToolName,
					"arguments": args,
				},
			})
		case "tool_result":
			toolResults = append(toolResults, map[string]any{
				"role":         "tool",
				"tool_call_id": c.ToolUseID,
				"content":      c.Text,
			})
		}
	}

	// Main message: only emit when it carries something.
	if len(texts) > 0 || len(toolCalls) > 0 || len(parts) > 0 {
		main := map[string]any{"role": m.Role}
		hasImage := false
		for _, p := range m.Content {
			if p.Type == "image" {
				hasImage = true
				break
			}
		}
		switch {
		case hasImage && m.Role != "assistant":
			main["content"] = parts
		default:
			main["content"] = strings.Join(texts, "")
		}
		if len(toolCalls) > 0 {
			main["tool_calls"] = toolCalls
		}
		out = append(out, main)
	}
	out = append(out, toolResults...)
	return out
}

// BuildChatToolResultMessage renders a tool result as a chat message.
func BuildChatToolResultMessage(toolUseID, content string) map[string]any {
	return map[string]any{"role": "tool", "tool_call_id": toolUseID, "content": content}
}

// AggregateChatResponse accumulates stream events and produces a
// non-streaming chat completion response.
type AggregateChatResponse struct {
	ID        string
	Model     string
	Content   strings.Builder
	Reason    strings.Builder
	Usage     Usage
	Finish    string
	ToolCalls []struct {
		ID        string
		Name      string
		Arguments strings.Builder
	}
}

// Add folds one stream event into the aggregate.
func (a *AggregateChatResponse) Add(ev UnifiedStreamEvent) {
	switch ev.Type {
	case "text_delta":
		a.Content.WriteString(ev.Text)
	case "reasoning_delta":
		a.Reason.WriteString(ev.Text)
	case "tool_call_delta":
		if ev.ToolCall == nil {
			return
		}
		idx := ev.ToolCall.Index
		for len(a.ToolCalls) <= idx {
			a.ToolCalls = append(a.ToolCalls, struct {
				ID        string
				Name      string
				Arguments strings.Builder
			}{})
		}
		tc := &a.ToolCalls[idx]
		if ev.ToolCall.ID != "" {
			tc.ID = ev.ToolCall.ID
		}
		if ev.ToolCall.Name != "" {
			tc.Name = ev.ToolCall.Name
		}
		tc.Arguments.WriteString(ev.ToolCall.ArgsDelta)
	case "usage":
		if ev.Usage != nil {
			a.Usage = *ev.Usage
		}
	case "finish":
		a.Finish = ev.Finish
	}
}

// Render produces the final chat.completion JSON body.
func (a *AggregateChatResponse) Render(w io.Writer) error {
	msg := map[string]any{"role": "assistant"}
	if a.Reason.Len() > 0 {
		msg["reasoning_content"] = a.Reason.String()
	}
	msg["content"] = a.Content.String()
	if len(a.ToolCalls) > 0 {
		var tcs []map[string]any
		for i, tc := range a.ToolCalls {
			tcs = append(tcs, map[string]any{
				"id":   orDefault(tc.ID, fmt.Sprintf("call_%d", i)),
				"type": "function",
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": tc.Arguments.String(),
				},
			})
		}
		msg["tool_calls"] = tcs
	}
	resp := map[string]any{
		"id":      orDefault(a.ID, "chatcmpl-gw"),
		"object":  "chat.completion",
		"created": nowUnix(),
		"model":   a.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       msg,
			"finish_reason": orDefault(a.Finish, "stop"),
		}},
		"usage": map[string]any{
			"prompt_tokens":     a.Usage.InputTokens,
			"completion_tokens": a.Usage.OutputTokens,
			"total_tokens":      a.Usage.InputTokens + a.Usage.OutputTokens,
		},
	}
	enc := json.NewEncoder(w)
	return enc.Encode(resp)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
