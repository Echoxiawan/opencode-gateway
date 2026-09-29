// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"net/http"
	"strings"
	"testing"
)

func TestSessionFromUserID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		// Claude Code: only the session tail identifies the conversation.
		{"user_abc123_account_def456_session_8b1f0c2e-1111-2222-3333-444455556666",
			"session_8b1f0c2e-1111-2222-3333-444455556666"},
		// No session marker: the id itself is still the best grouping available.
		{"user_abc123", "user_abc123"},
	}
	for _, c := range cases {
		if got := SessionFromUserID(c.in); got != c.want {
			t.Errorf("SessionFromUserID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSessionFromHeaders(t *testing.T) {
	h := http.Header{}
	if got := SessionFromHeaders(h); got != "" {
		t.Errorf("empty headers = %q, want \"\"", got)
	}
	h.Set("session_id", "codex-session-1")
	if got := SessionFromHeaders(h); got != "codex-session-1" {
		t.Errorf("got %q, want codex-session-1", got)
	}
	// x-opencode-session wins over the generic names.
	h.Set("x-opencode-session", "oc-session")
	if got := SessionFromHeaders(h); got != "oc-session" {
		t.Errorf("got %q, want oc-session", got)
	}
}

func TestSessionHintFromRequestBody(t *testing.T) {
	// Anthropic metadata.user_id.
	req, err := ParseAnthropicRequest([]byte(`{"model":"m","messages":[],
		"metadata":{"user_id":"user_x_account_y_session_z"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.SessionHint != "session_z" {
		t.Errorf("anthropic hint = %q, want session_z", req.SessionHint)
	}

	// Chat: metadata.session_id beats the end-user field.
	req, err = ParseChatRequest([]byte(`{"model":"m","messages":[],"user":"u1",
		"metadata":{"session_id":"s1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.SessionHint != "s1" {
		t.Errorf("chat hint = %q, want s1", req.SessionHint)
	}

	// Responses: metadata.session_id only.
	req, err = ParseResponsesRequest([]byte(`{"model":"m","input":"hi",
		"metadata":{"session_id":"s2"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.SessionHint != "s2" {
		t.Errorf("responses hint = %q, want s2", req.SessionHint)
	}
}

func TestCompactSessionBoundsLength(t *testing.T) {
	long := strings.Repeat("a", sessionMaxLen*2)
	if got := compactSession(long); len(got) != sessionMaxLen {
		t.Errorf("len = %d, want %d", len(got), sessionMaxLen)
	}
}
