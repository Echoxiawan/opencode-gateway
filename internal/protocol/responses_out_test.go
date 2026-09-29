// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// TestResponsesFramesCarryEventName applies the same guard as the Anthropic
// test: Responses SSE frames must carry an event name, since clients (Codex)
// dispatch on it and drop anonymous frames.
func TestResponsesFramesCarryEventName(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := NewResponsesStreamWriter(rec, "mimo-v2.6-flash-free")

	for _, ev := range []UnifiedStreamEvent{
		{Type: "reasoning_delta", Text: "thinking"},
		{Type: "text_delta", Text: "Hello"},
		{Type: "text_delta", Text: " world"},
		{Type: "finish", Finish: "stop"},
	} {
		if err := sw.WriteEvent(ev); err != nil {
			t.Fatalf("WriteEvent(%s): %v", ev.Type, err)
		}
	}
	sw.Finish()

	frames, missing := parseSSEFrames(rec.Body.String())
	if len(missing) > 0 {
		t.Fatalf("%d frame(s) missing an event name, e.g. %q", len(missing), missing[0])
	}

	want := map[string]bool{
		"response.created":           false,
		"response.output_text.delta": false,
		"response.completed":         false,
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

	// Every frame's declared event name must match its payload "type".
	for _, f := range frames {
		var payload struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(f[1]), &payload); err != nil {
			t.Errorf("frame %q is not valid JSON: %v", f[0], err)
			continue
		}
		if payload.Type != f[0] {
			t.Errorf("event name %q disagrees with payload type %q", f[0], payload.Type)
		}
	}
}

// TestChatFramesHaveNoEventName pins the OpenAI chat contract: those frames
// carry a data line only. If an event line ever appears there, strict OpenAI
// clients may stop parsing the stream.
func TestChatFramesHaveNoEventName(t *testing.T) {
	rec := httptest.NewRecorder()
	cw := NewChatStreamWriter(rec, "id", "m")
	_ = cw.WriteEvent(UnifiedStreamEvent{Type: "text_delta", Text: "hi"})
	_ = cw.WriteUsageAndDone(Usage{InputTokens: 1, OutputTokens: 1}, "stop")

	body := rec.Body.String()
	if !contains(body, "data: ") {
		t.Fatal("no data frames emitted")
	}
	if contains(body, "event: ") {
		t.Error("chat frames must not carry an event name")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
