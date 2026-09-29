// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseResponsesRequest converts an OpenAI Responses API request body into
// the unified representation.
func ParseResponsesRequest(body []byte) (*UnifiedRequest, error) {
	var raw struct {
		Model        string          `json:"model"`
		Stream       bool            `json:"stream"`
		Input        json.RawMessage `json:"input"`
		Instructions string          `json:"instructions"`
		Temperature  *float64        `json:"temperature"`
		TopP         *float64        `json:"top_p"`
		MaxTokens    int             `json:"max_output_tokens"`
		// Responses API has no session field either; some clients send one
		// alongside the conversation.
		Metadata *struct {
			SessionID string `json:"session_id"`
		} `json:"metadata"`
		Tools []struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			Function *struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				Parameters  json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}
	req := &UnifiedRequest{
		Model:       raw.Model,
		Stream:      raw.Stream,
		System:      raw.Instructions,
		Temperature: raw.Temperature,
		TopP:        raw.TopP,
		MaxTokens:   raw.MaxTokens,
	}
	if raw.Metadata != nil {
		req.SessionHint = compactSession(raw.Metadata.SessionID)
	}
	// input: string or array of items
	var inputStr string
	if err := json.Unmarshal(raw.Input, &inputStr); err == nil && inputStr != "" {
		req.Messages = append(req.Messages, UnifiedMessage{
			Role:    "user",
			Content: []ContentBlock{{Type: "text", Text: inputStr}},
		})
	} else {
		var items []struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			// function_call item
			CallID    string `json:"call_id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			// function_call_output item
			Output string `json:"output"`
		}
		if err := json.Unmarshal(raw.Input, &items); err != nil {
			return nil, fmt.Errorf("parse responses input: %w", err)
		}
		for _, it := range items {
			switch it.Type {
			case "message", "":
				role := it.Role
				if role == "" {
					role = "user"
				}
				blocks := parseResponsesContent(it.Content)
				if len(blocks) > 0 {
					req.Messages = append(req.Messages, UnifiedMessage{Role: role, Content: blocks})
				}
			case "function_call", "tool_call":
				req.Messages = append(req.Messages, UnifiedMessage{
					Role: "assistant",
					Content: []ContentBlock{{
						Type: "tool_call", ToolCallID: it.CallID, ToolName: it.Name, Text: it.Arguments,
					}},
				})
			case "function_call_output", "tool_call_output":
				out := it.Output
				var s string
				if err := json.Unmarshal([]byte(out), &s); err == nil {
					out = s
				}
				req.Messages = append(req.Messages, UnifiedMessage{
					Role:    "user",
					Content: []ContentBlock{{Type: "tool_result", ToolUseID: it.CallID, Text: out}},
				})
			case "reasoning":
				// server-side reasoning items are ignored (stateless mode)
			}
		}
	}
	for _, t := range raw.Tools {
		switch t.Type {
		case "function":
			if t.Function != nil {
				req.Tools = append(req.Tools, Tool{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  string(t.Function.Parameters),
				})
			}
		}
	}
	_ = strings.TrimSpace
	return req, nil
}

// parseResponsesContent converts a Responses message content field into
// unified content blocks. It accepts a bare string or the array form, and
// recognizes every text part type the Responses API emits:
// input_text (client input), output_text (prior assistant turn),
// summary_text, and text. input_image parts become image blocks.
func parseResponsesContent(raw json.RawMessage) []ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	// Bare string form.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []ContentBlock{{Type: "text", Text: s}}
	}
	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL string `json:"image_url"`
		Detail   string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	var out []ContentBlock
	for _, p := range parts {
		switch p.Type {
		case "input_text", "output_text", "summary_text", "text", "refusal":
			if p.Text != "" {
				out = append(out, ContentBlock{Type: "text", Text: p.Text})
			}
		case "input_image", "image_url":
			if p.ImageURL != "" {
				media, data := parseDataURL(p.ImageURL)
				out = append(out, ContentBlock{Type: "image", MediaType: media, Data: data})
			}
		}
	}
	return out
}
