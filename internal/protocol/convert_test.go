// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"testing"
)

// mustJSON marshals v or fails the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// contentParts asserts every message carrying array content has a "text"
// string (or image_url) on each part — the shape upstream rejects otherwise.
func assertContentPartsValid(t *testing.T, body map[string]any) {
	t.Helper()
	msgs, _ := body["messages"].([]map[string]any)
	if msgs == nil {
		raw, _ := body["messages"].([]any)
		for _, r := range raw {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		t.Fatal("no messages produced")
	}
	for i, m := range msgs {
		content, has := m["content"]
		if !has {
			t.Errorf("message %d (%v) has no content field", i, m["role"])
			continue
		}
		arr, ok := content.([]map[string]any)
		if !ok {
			continue // string content is always fine
		}
		for j, part := range arr {
			typ, _ := part["type"].(string)
			switch typ {
			case "text":
				if _, ok := part["text"].(string); !ok {
					t.Errorf("message %d part %d: text part missing string \"text\": %#v", i, j, part)
				}
			case "image_url":
				if _, ok := part["image_url"].(map[string]any); !ok {
					t.Errorf("message %d part %d: image_url malformed: %#v", i, j, part)
				}
			default:
				t.Errorf("message %d part %d: unexpected part type %q", i, j, typ)
			}
		}
	}
}

// TestResponsesInputTextIsRecognized covers the Codex CLI shape, which sends
// content parts typed "input_text" rather than "output_text".
func TestResponsesInputTextIsRecognized(t *testing.T) {
	reqBody := mustJSON(t, map[string]any{
		"model":        "mimo-v2.6-flash-free",
		"instructions": "You are Codex.",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "list the files"},
				},
			},
		},
		"stream": true,
	})
	req, err := ParseResponsesRequest(reqBody)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(req.Messages))
	}
	if got := req.Messages[0].Content[0].Text; got != "list the files" {
		t.Fatalf("input_text not parsed, got %q", got)
	}

	body := BuildChatBody(req)
	assertContentPartsValid(t, body)

	// System instruction must survive.
	if body["messages"].([]map[string]any)[0]["role"] != "system" {
		t.Error("instructions were dropped")
	}
}

// TestResponsesToolRoundTrip covers a Codex follow-up turn carrying a
// function_call and its output, which must become assistant tool_calls plus a
// role:"tool" message.
func TestResponsesToolRoundTrip(t *testing.T) {
	reqBody := mustJSON(t, map[string]any{
		"model": "mimo-v2.6-flash-free",
		"input": []any{
			map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "run ls"}},
			},
			map[string]any{
				"type": "function_call", "call_id": "call_abc", "name": "shell",
				"arguments": `{"command":"ls"}`,
			},
			map[string]any{
				"type": "function_call_output", "call_id": "call_abc",
				"output": "file1.go\nfile2.go",
			},
		},
	})
	req, err := ParseResponsesRequest(reqBody)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body := BuildChatBody(req)
	assertContentPartsValid(t, body)

	msgs := body["messages"].([]map[string]any)
	var sawToolCall, sawToolResult bool
	for _, m := range msgs {
		if m["role"] == "assistant" {
			if tcs, ok := m["tool_calls"].([]map[string]any); ok && len(tcs) > 0 {
				sawToolCall = true
			}
		}
		if m["role"] == "tool" {
			if m["tool_call_id"] != "call_abc" {
				t.Errorf("tool message has wrong tool_call_id: %v", m["tool_call_id"])
			}
			sawToolResult = true
		}
	}
	if !sawToolCall {
		t.Error("function_call did not become assistant tool_calls")
	}
	if !sawToolResult {
		t.Error("function_call_output did not become a role:tool message")
	}
}

// TestAnthropicToolResultBecomesToolMessage covers Claude Code follow-ups:
// tool_result lives inside a user message and must be split out.
func TestAnthropicToolResultBecomesToolMessage(t *testing.T) {
	reqBody := mustJSON(t, map[string]any{
		"model":      "mimo-v2.6-flash-free",
		"max_tokens": 1024,
		"messages": []any{
			map[string]any{"role": "user", "content": "read main.go"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": "toolu_1", "name": "Read",
					"input": map[string]any{"file_path": "main.go"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_1",
					"content": "package main"},
			}},
		},
	})
	req, err := ParseAnthropicRequest(reqBody)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body := BuildChatBody(req)
	assertContentPartsValid(t, body)

	msgs := body["messages"].([]map[string]any)
	var sawTool bool
	for _, m := range msgs {
		if m["role"] == "tool" {
			sawTool = true
			if m["tool_call_id"] != "toolu_1" {
				t.Errorf("wrong tool_call_id: %v", m["tool_call_id"])
			}
		}
	}
	if !sawTool {
		t.Error("anthropic tool_result did not become a tool message")
	}
}

// TestContentNeverUndefined guards the original 400 error: no message may be
// emitted without a content field, even for degenerate inputs.
func TestContentNeverUndefined(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty user content", map[string]any{
			"model": "m", "messages": []any{
				map[string]any{"role": "user", "content": ""},
			},
		}},
		{"assistant with only tool_call", map[string]any{
			"model": "m", "messages": []any{
				map[string]any{"role": "user", "content": "hi"},
				map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{
					map[string]any{"id": "c1", "type": "function",
						"function": map[string]any{"name": "f", "arguments": "{}"}},
				}},
			},
		}},
		{"image only", map[string]any{
			"model": "m", "messages": []any{
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "image_url",
						"image_url": map[string]any{"url": "data:image/png;base64,AAAA"}},
				}},
			},
		}},
		{"empty messages array", map[string]any{"model": "m", "messages": []any{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := ParseChatRequest(mustJSON(t, tc.body))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			assertContentPartsValid(t, BuildChatBody(req))
		})
	}
}

// TestEmptyResponsesInputProducesValidBody covers a Responses request whose
// input yields no blocks (e.g. only reasoning items).
func TestEmptyResponsesInputProducesValidBody(t *testing.T) {
	reqBody := mustJSON(t, map[string]any{
		"model": "m",
		"input": []any{
			map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}},
		},
	})
	req, err := ParseResponsesRequest(reqBody)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	assertContentPartsValid(t, BuildChatBody(req))
}

// TestResponsesBareStringInput covers {"input": "hello"}.
func TestResponsesBareStringInput(t *testing.T) {
	req, err := ParseResponsesRequest(mustJSON(t, map[string]any{
		"model": "m", "input": "hello",
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("bare string input not parsed: %#v", req.Messages)
	}
	assertContentPartsValid(t, BuildChatBody(req))
}

// TestEmptyTextPartsSkipped ensures blank text blocks never reach upstream,
// which would produce an empty "text" part.
func TestEmptyTextPartsSkipped(t *testing.T) {
	req, err := ParseChatRequest(mustJSON(t, map[string]any{
		"model": "m",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": ""},
				map[string]any{"type": "text", "text": "real content"},
			}},
		},
	}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body := BuildChatBody(req)
	assertContentPartsValid(t, body)
	msgs := body["messages"].([]map[string]any)
	if msgs[0]["content"] != "real content" {
		t.Errorf("expected collapsed text, got %#v", msgs[0]["content"])
	}
}
