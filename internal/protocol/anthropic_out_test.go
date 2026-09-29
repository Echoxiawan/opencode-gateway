// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// parseSSEFrames splits an SSE body into (event, data) pairs and reports any
// frame that carried data without an event name.
func parseSSEFrames(body string) (frames [][2]string, missingEvent []string) {
	blocks := strings.Split(strings.TrimSpace(body), "\n\n")
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var event, data string
		for _, line := range strings.Split(block, "\n") {
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				event = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				data = v
			}
		}
		if data != "" && event == "" {
			missingEvent = append(missingEvent, data)
		}
		frames = append(frames, [2]string{event, data})
	}
	return frames, missingEvent
}

// TestAnthropicFramesCarryEventName guards a regression: content_block_delta
// frames were emitted with only a "data:" line, so Anthropic clients — which
// dispatch on the event name — dropped all real text and rendered an empty
// response even though the request returned 200.
func TestAnthropicFramesCarryEventName(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewAnthropicStreamWriter(rec, "mimo-v2.6-flash-free")

	events := []UnifiedStreamEvent{
		{Type: "reasoning_delta", Text: "thinking..."},
		{Type: "text_delta", Text: "Hello"},
		{Type: "text_delta", Text: " world"},
		{Type: "finish", Finish: "stop"},
	}
	for _, ev := range events {
		if err := sw.WriteEvent(ev); err != nil {
			t.Fatalf("WriteEvent(%s): %v", ev.Type, err)
		}
	}
	sw.Finish("stop")

	body := rec.Body.String()
	frames, missing := parseSSEFrames(body)
	if len(missing) > 0 {
		t.Fatalf("%d frame(s) missing an event name, e.g. %q", len(missing), missing[0])
	}

	// The client must be able to dispatch on these names.
	want := map[string]bool{
		"message_start":       false,
		"content_block_start": false,
		"content_block_delta": false,
		"content_block_stop":  false,
		"message_delta":       false,
		"message_stop":        false,
	}
	for _, f := range frames {
		if _, ok := want[f[0]]; ok {
			want[f[0]] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("event %q never emitted", name)
		}
	}

	// Deltas must survive with their text intact.
	var textSeen string
	for _, f := range frames {
		if f[0] != "content_block_delta" {
			continue
		}
		var payload struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(f[1]), &payload); err != nil {
			t.Fatalf("delta frame is not valid JSON: %v", err)
		}
		if payload.Delta.Type == "text_delta" {
			textSeen += payload.Delta.Text
		}
	}
	if textSeen != "Hello world" {
		t.Errorf("text deltas lost content: got %q, want %q", textSeen, "Hello world")
	}
}

// TestAnthropicContentBlockShapes checks that each block carries only its own
// fields — a text block has no "thinking" key and a thinking block carries a
// signature, as strict clients expect.
func TestAnthropicContentBlockShapes(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewAnthropicStreamWriter(rec, "m")
	_ = sw.WriteEvent(UnifiedStreamEvent{Type: "reasoning_delta", Text: "r"})
	_ = sw.WriteEvent(UnifiedStreamEvent{Type: "text_delta", Text: "t"})
	sw.Finish("stop")

	frames, _ := parseSSEFrames(rec.Body.String())
	var thinkingBlock, textBlock map[string]any
	for _, f := range frames {
		if f[0] != "content_block_start" {
			continue
		}
		var payload struct {
			ContentBlock map[string]any `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(f[1]), &payload); err != nil {
			t.Fatalf("content_block_start is not valid JSON: %v", err)
		}
		switch payload.ContentBlock["type"] {
		case "thinking":
			thinkingBlock = payload.ContentBlock
		case "text":
			textBlock = payload.ContentBlock
		}
	}
	if thinkingBlock == nil || textBlock == nil {
		t.Fatalf("missing blocks: thinking=%v text=%v", thinkingBlock, textBlock)
	}
	if _, ok := thinkingBlock["text"]; ok {
		t.Error("thinking block must not carry a text field")
	}
	if _, ok := thinkingBlock["signature"]; !ok {
		t.Error("thinking block must carry a signature field")
	}
	if _, ok := textBlock["thinking"]; ok {
		t.Error("text block must not carry a thinking field")
	}
}

// assembleToolInputs replays the Anthropic block protocol the way a client
// does: a tool_use block's input is the concatenation of every input_json_delta
// sent at its index.
func assembleToolInputs(t *testing.T, body string) map[int]string {
	t.Helper()
	frames, _ := parseSSEFrames(body)
	tools := map[int]string{}
	for _, f := range frames {
		if f[0] != "content_block_start" && f[0] != "content_block_delta" {
			continue
		}
		var d struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(f[1]), &d); err != nil {
			t.Fatalf("frame %s: %v", f[0], err)
		}
		switch d.Type {
		case "content_block_start":
			if d.ContentBlock.Type == "tool_use" {
				tools[d.Index] = ""
			}
		case "content_block_delta":
			if d.Delta.Type == "input_json_delta" {
				js, ok := tools[d.Index]
				if !ok {
					t.Errorf("input_json_delta at index %d, but no tool_use block was opened there", d.Index)
					continue
				}
				tools[d.Index] = js + d.Delta.PartialJSON
			}
		}
	}
	return tools
}

// A reasoning delta arriving between a tool call's argument fragments must not
// interrupt the tool_use block. Closing it there truncates the arguments, and
// the client reports "The model's tool call could not be parsed".
func TestReasoningBetweenToolArgsKeepsJSONValid(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewAnthropicStreamWriter(rec, "m")
	for _, ev := range []UnifiedStreamEvent{
		{Type: "reasoning_delta", Text: "I should read the file."},
		{Type: "tool_call_delta", ToolCall: &ToolCallEvent{Index: 0, ID: "call_R", Name: "Read"}},
		{Type: "tool_call_delta", ToolCall: &ToolCallEvent{Index: 0, ArgsDelta: `{"file_`}},
		// Interleaved reasoning mid-call: must be held back, not emitted here.
		{Type: "reasoning_delta", Text: "path looks right."},
		{Type: "tool_call_delta", ToolCall: &ToolCallEvent{Index: 0, ArgsDelta: `path":"main.go"}`}},
		{Type: "finish", Finish: "tool_calls"},
	} {
		if err := sw.WriteEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	sw.Finish("tool_calls")

	tools := assembleToolInputs(t, rec.Body.String())
	if len(tools) != 1 {
		t.Fatalf("got %d tool blocks, want 1: %+v", len(tools), tools)
	}
	var assembled string
	for _, v := range tools {
		assembled = v
	}
	var args map[string]string
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Fatalf("assembled tool input is not valid JSON: %q (%v)", assembled, err)
	}
	if args["file_path"] != "main.go" {
		t.Errorf("args = %+v, want file_path=main.go", args)
	}
	// The held-back reasoning must still reach the client, after the tool block.
	if !strings.Contains(rec.Body.String(), "path looks right.") {
		t.Error("buffered reasoning was dropped instead of emitted after the tool block")
	}
}

// A signature tool the gateway injected is filtered upstream, so its argument
// fragments arrive with no id and no name. They must not be attached to
// whatever block happens to be open.
func TestOrphanToolFragmentsAreNotEmitted(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewAnthropicStreamWriter(rec, "m")
	for _, ev := range []UnifiedStreamEvent{
		{Type: "reasoning_delta", Text: "thinking"},
		{Type: "tool_call_delta", ToolCall: &ToolCallEvent{Index: 0, ArgsDelta: `{"pattern":"todo"}`}},
		{Type: "finish", Finish: "stop"},
	} {
		if err := sw.WriteEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	sw.Finish("stop")
	if strings.Contains(rec.Body.String(), "input_json_delta") {
		t.Error("orphan fragments were emitted, they belong to a filtered signature tool")
	}
}
