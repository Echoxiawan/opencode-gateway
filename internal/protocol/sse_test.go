// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"strings"
	"testing"
)

// An injected signature tool is streamed like any other tool: one chunk with
// id+name, then chunks carrying only argument fragments. Filtering by name
// drops the first but lets the rest through, and those unattributed fragments
// are what made strict Anthropic clients abort with "Content block is not a
// input_json block". Every fragment of a suppressed tool must disappear, and a
// real tool call in the same turn must survive untouched.
func TestInjectedToolFragmentsAreFullySuppressed(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"grep","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pattern\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"todo\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"Bash","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"command\":\"ls\"}"}}]}}]}`,
		``,
	}, "\n")

	var got []UnifiedStreamEvent
	err := ParseSSEStream(strings.NewReader(stream), StreamOptions{}, func(ev UnifiedStreamEvent) error {
		if ev.Type == "tool_call_delta" && ev.ToolCall != nil {
			got = append(got, ev)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tool events, want 2 (suppressed grep + kept Bash): %+v", len(got), got)
	}
	if got[0].ToolCall.Name != "Bash" || got[0].ToolCall.ID != "call_2" {
		t.Errorf("first surviving event = %+v, want the Bash opening chunk", got[0].ToolCall)
	}
	if got[1].ToolCall.ArgsDelta != `{"command":"ls"}` {
		t.Errorf("second event args = %q", got[1].ToolCall.ArgsDelta)
	}
}

// Every name the free tier injects must be filtered. A name missing here slips
// through as a phantom tool_use block the client never asked for.
func TestInjectedToolCoversEveryInjectedName(t *testing.T) {
	for _, name := range []string{"bash", "read", "edit", "glob", "grep"} {
		if !InjectedTool(name) {
			t.Errorf("InjectedTool(%q) = false, want true", name)
		}
	}
	// Client tools must not be filtered by accident.
	for _, name := range []string{"Bash", "Read", "Edit", "Glob", "Grep", "Task", "Write"} {
		if InjectedTool(name) {
			t.Errorf("InjectedTool(%q) = true, want false (client tool)", name)
		}
	}
}

// An upstream that answers 200 and then sends an error frame must surface that
// error, not end the stream as a silent empty completion.
func TestParseSSEStreamSurfacesUpstreamErrorFrame(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"id":"c1","model":"m","choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		``,
		`data: {"error":{"message":"This model is currently unavailable.","type":"invalid_request_error"}}`,
		``,
	}, "\n")

	var got []UnifiedStreamEvent
	err := ParseSSEStream(strings.NewReader(stream), StreamOptions{}, func(ev UnifiedStreamEvent) error {
		got = append(got, ev)
		return nil
	})

	se, ok := err.(*UpstreamStreamError)
	if !ok {
		t.Fatalf("err = %v (%T), want *UpstreamStreamError", err, err)
	}
	if se.Message != "This model is currently unavailable." {
		t.Errorf("message = %q", se.Message)
	}
	// The content that did arrive before the error is still delivered.
	if len(got) != 1 || got[0].Text != "hi" {
		t.Errorf("events before the error = %+v", got)
	}
}

// Provider variations: a bare type, a code, or a null error must not be
// mistaken for a failure (or crash on the nil).
func TestUpstreamStreamErrorVariants(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
		isErr   bool
	}{
		{"message", `{"error":{"message":"boom"}}`, "boom", true},
		{"type only", `{"error":{"type":"overloaded_error"}}`, "overloaded_error", true},
		{"code only", `{"error":{"code":"rate_limited"}}`, "rate_limited", true},
		{"empty object", `{"error":{}}`, "upstream returned an error event", true},
		{"null error", `{"error":null}`, "", false},
		{"normal chunk", `{"choices":[{"delta":{"content":"x"}}]}`, "", false},
		{"unknown shape", `{"whatever":1}`, "", false},
	}
	for _, c := range cases {
		msg, isErr := upstreamStreamError(c.payload)
		if isErr != c.isErr || msg != c.want {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.name, msg, isErr, c.want, c.isErr)
		}
	}
}

// A stream that ends normally, or with an unmodelled chunk, must not error.
func TestParseSSEStreamToleratesUnknownChunks(t *testing.T) {
	stream := "data: {\"kind\":\"heartbeat\"}\n\ndata: [DONE]\n\n"
	if err := ParseSSEStream(strings.NewReader(stream), StreamOptions{}, func(UnifiedStreamEvent) error { return nil }); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

// The usage frame's details blocks are optional. Reading one without a nil
// check used to panic the handler mid-stream, killing the connection while the
// client was still reading — the request died with no response and no reason.
func TestUsageFrameWithMissingDetailsDoesNotPanic(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"no details at all", `{"usage":{"prompt_tokens":10,"completion_tokens":5}}`},
		{"only cached", `{"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":8}}}`},
		{"only reasoning", `{"usage":{"prompt_tokens":10,"completion_tokens_details":{"reasoning_tokens":3}}}`},
		{"both", `{"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":8},"completion_tokens_details":{"reasoning_tokens":3}}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var usage *Usage
			err := ParseSSEStream(strings.NewReader("data: "+c.data+"\n\n"), StreamOptions{}, func(ev UnifiedStreamEvent) error {
				if ev.Usage != nil {
					usage = ev.Usage
				}
				return nil
			})
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if usage == nil {
				t.Fatal("no usage event")
			}
			if usage.InputTokens != 10 {
				t.Errorf("input = %d, want 10", usage.InputTokens)
			}
		})
	}
}

// The model often calls an injected stub rather than the client's tool, because
// the stub has a trivial schema. With the client's own tool declared for the
// same capability, that call must be handed back under the client's name — not
// silently dropped, which leaves the turn doing nothing.
func TestInjectedToolCallIsAliasedToClientTool(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"ls\"}"}}]}}]}`,
		``,
	}, "\n")

	// Claude Code declares Bash/Read/Edit/Glob/Grep.
	opts := StreamOptions{ToolAliases: ToolAliases([]Tool{
		{Name: "Bash"}, {Name: "Read"}, {Name: "Edit"}, {Name: "Glob"}, {Name: "Grep"}, {Name: "Task"},
	})}
	var got []UnifiedStreamEvent
	err := ParseSSEStream(strings.NewReader(stream), opts, func(ev UnifiedStreamEvent) error {
		if ev.Type == "tool_call_delta" && ev.ToolCall != nil {
			got = append(got, ev)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (opening chunk + args): %+v", len(got), got)
	}
	if got[0].ToolCall.Name != "Bash" {
		t.Errorf("name = %q, want the client's own Bash", got[0].ToolCall.Name)
	}
	if got[1].ToolCall.ArgsDelta != `{"command":"ls"}` {
		t.Errorf("args = %q", got[1].ToolCall.ArgsDelta)
	}
}

// With no client tool to alias to, the call carries no meaning for the client
// and must stay dropped. Task is not one of the five required names.
func TestToolAliasesIgnoreNonInjectedTools(t *testing.T) {
	aliases := ToolAliases([]Tool{{Name: "Task"}, {Name: "WebFetch"}})
	if len(aliases) != 0 {
		t.Errorf("aliases = %v, want empty", aliases)
	}
}
