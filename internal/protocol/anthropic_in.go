// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"fmt"
)

// ParseAnthropicRequest converts an Anthropic Messages request body into the
// unified representation.
func ParseAnthropicRequest(body []byte) (*UnifiedRequest, error) {
	var raw struct {
		Model       string          `json:"model"`
		System      json.RawMessage `json:"system"`
		Stream      bool            `json:"stream"`
		Temperature *float64        `json:"temperature"`
		TopP        *float64        `json:"top_p"`
		MaxTokens   int             `json:"max_tokens"`
		Messages    []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		} `json:"tools"`
		Metadata *struct {
			UserID    string `json:"user_id"`
			SessionID string `json:"session_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	req := &UnifiedRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		Temperature: raw.Temperature,
		TopP:        raw.TopP,
		MaxTokens:   raw.MaxTokens,
	}
	if raw.Metadata != nil {
		req.SessionHint = orDefault(compactSession(raw.Metadata.SessionID),
			SessionFromUserID(raw.Metadata.UserID))
	}
	// system: string or array of blocks
	sys, err := parseAnthropicContent(raw.System)
	if err != nil {
		return nil, err
	}
	for _, c := range sys {
		if c.Type == "text" {
			req.System += c.Text
		}
	}
	for _, m := range raw.Messages {
		blocks, err := parseAnthropicContent(m.Content)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, UnifiedMessage{Role: m.Role, Content: blocks})
	}
	for _, t := range raw.Tools {
		req.Tools = append(req.Tools, Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  string(t.InputSchema),
		})
	}
	return req, nil
}

// parseAnthropicContent handles both string and block-array content.
func parseAnthropicContent(raw json.RawMessage) ([]ContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if str == "" {
			return nil, nil
		}
		return []ContentBlock{{Type: "text", Text: str}}, nil
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`

		Source *struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source"`

		// tool_use (assistant)
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`

		// tool_result (user)
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
		IsError   bool            `json:"is_error"`

		// thinking
		Thinking string `json:"thinking"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("parse anthropic content blocks: %w", err)
	}
	var out []ContentBlock
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, ContentBlock{Type: "text", Text: b.Text})
		case "image":
			if b.Source != nil {
				out = append(out, ContentBlock{Type: "image", MediaType: b.Source.MediaType, Data: b.Source.Data})
			}
		case "tool_use":
			out = append(out, ContentBlock{Type: "tool_call", ToolCallID: b.ID, ToolName: b.Name, Text: string(b.Input)})
		case "tool_result":
			var text string
			// tool_result content may be string or blocks
			var sub []ContentBlock
			var s string
			if err := json.Unmarshal(b.Content, &s); err == nil {
				text = s
			} else if err := json.Unmarshal(b.Content, &sub); err == nil {
				for _, sb := range sub {
					if sb.Type == "text" {
						text += sb.Text
					}
				}
			} else if len(b.Content) > 0 {
				text = string(b.Content)
			}
			out = append(out, ContentBlock{Type: "tool_result", ToolUseID: b.ToolUseID, Text: text, IsError: b.IsError})
		case "thinking":
			out = append(out, ContentBlock{Type: "thinking", Text: b.Thinking})
		}
	}
	return out, nil
}
