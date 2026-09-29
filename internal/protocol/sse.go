// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ParseSSEStream reads an OpenAI Chat Completions SSE stream and emits
// unified events. It stops on stream end or error.
//
// This parses the upstream zen chat/completions wire format:
//
//	data: {"choices":[{"delta":{"content":"..."}}], ...}
//
// StreamOptions tunes how an upstream stream is translated. The zero value
// keeps every rule at its default.
type StreamOptions struct {
	// ToolAliases maps an injected signature tool name (lowercase, e.g.
	// "bash") to the exact name the client declared for the same capability
	// (e.g. "Bash"). The model frequently calls the injected stub rather than
	// the client's tool — the stub has a trivial schema — and those calls are
	// otherwise discarded, leaving the client with a turn that silently did
	// nothing. See ToolAliases.
	ToolAliases map[string]string
}

// ToolAliases derives the alias map from the tools the client declared:
// lowercase name -> declared name, for every name the free tier requires.
// Claude Code declares Bash/Read/Edit/Glob/Grep, which covers all five, so a
// stub call can always be handed back under the name its client knows.
func ToolAliases(tools []Tool) map[string]string {
	var out map[string]string
	for _, t := range tools {
		lower := strings.ToLower(t.Name)
		if !InjectedTool(lower) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[lower] = t.Name
	}
	return out
}

func ParseSSEStream(r io.Reader, opts StreamOptions, onEvent func(UnifiedStreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	// Tools are streamed in pieces: the first chunk carries id+name, later ones
	// carry only argument fragments. An injected signature tool is recognised
	// purely by name, so its opening chunk is dropped while its argument
	// fragments look anonymous — and those fragments, delivered without a
	// preceding tool_use block, are what make strict clients abort with
	// "Content block is not a input_json block". Remembering which indices were
	// suppressed lets the later fragments be dropped with them.
	suppressed := map[int]bool{}
	var lastErr error
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				return nil
			}
			continue
		}
		// An upstream 200 can still carry a failure inside the stream: zen
		// reports some errors as a single data frame whose body is
		// {"error":{...}}. That shape does not match a chat chunk, so skipping
		// it as an unknown chunk ended the request as a clean 200 with no
		// content — the client saw an empty message and raised its own generic
		// API error, with nothing anywhere saying what actually went wrong.
		if msg, ok := upstreamStreamError(payload); ok {
			return &UpstreamStreamError{Message: msg}
		}
		ev, suppressedIdx, err := parseChatChunk([]byte(payload), opts, suppressed)
		if err != nil {
			// Tolerate genuinely unknown chunks (the upstream is free to add
			// fields and event kinds we do not model).
			continue
		}
		if suppressedIdx >= 0 {
			suppressed[suppressedIdx] = true
		}
		for _, e := range ev {
			if err := onEvent(e); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		lastErr = err
	}
	return lastErr
}

// chatChunk mirrors the upstream chunk shape.
type chatChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			Reasoning        string `json:"reasoning"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

// InjectedTool reports whether a tool name is one of the gateway-injected
// opencode signature tools. The set must match upstream/zen.go freeTierTools
// exactly; missing an entry lets the model's call to that stub slip through the
// filter and appear as a phantom tool_use block to the downstream client.
func InjectedTool(name string) bool {
	switch name {
	case "bash", "read", "edit", "glob", "grep":
		return true
	}
	return false
}

// UpstreamStreamError is a failure the upstream reported inside an otherwise
// successful (HTTP 200) event stream. It is distinct from a transport error:
// the connection was fine, and the upstream chose to answer with an error
// frame instead of a completion.
type UpstreamStreamError struct {
	Message string
}

func (e *UpstreamStreamError) Error() string {
	return "upstream error: " + e.Message
}

// upstreamStreamError reports whether an SSE data frame carries an upstream
// error object, and returns its message. Frames that merely happen to fail to
// unmarshal still return false, so they keep being tolerated.
func upstreamStreamError(payload string) (string, bool) {
	var env struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(payload), &env) != nil || env.Error == nil {
		return "", false
	}
	msg := env.Error.Message
	if msg == "" {
		msg = env.Error.Type
	}
	if msg == "" {
		msg = env.Error.Code
	}
	if msg == "" {
		msg = "upstream returned an error event"
	}
	return msg, true
}

// parseChatChunk converts one upstream chunk into unified events. suppressed
// carries the tool indices already recognised as injected signature tools;
// the returned index is >= 0 when this chunk identifies a new one, so the
// caller can remember it and drop its later argument-only fragments too.
func parseChatChunk(data []byte, opts StreamOptions, suppressed map[int]bool) ([]UnifiedStreamEvent, int, error) {
	var c chatChunk
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, -1, err
	}
	var events []UnifiedStreamEvent
	newlySuppressed := -1
	for _, ch := range c.Choices {
		d := ch.Delta
		if d.ReasoningContent != "" {
			events = append(events, UnifiedStreamEvent{Type: "reasoning_delta", Text: d.ReasoningContent, ID: c.ID, Model: c.Model})
		}
		if d.Reasoning != "" {
			events = append(events, UnifiedStreamEvent{Type: "reasoning_delta", Text: d.Reasoning, ID: c.ID, Model: c.Model})
		}
		if d.Content != "" {
			events = append(events, UnifiedStreamEvent{Type: "text_delta", Text: d.Content, ID: c.ID, Model: c.Model})
		}
		for _, tc := range d.ToolCalls {
			name := tc.Function.Name
			if InjectedTool(name) {
				// The model called a stub the gateway injected as a signature.
				// Hand it back under the client's own name when one exists, so
				// the call still runs; otherwise drop it and remember the index
				// so its bare argument fragments are dropped with it.
				alias := opts.ToolAliases[strings.ToLower(name)]
				if alias == "" {
					newlySuppressed = tc.Index
					continue
				}
				name = alias
			}
			if tc.Function.Name == "" && suppressed[tc.Index] {
				// Argument fragment belonging to a suppressed injected tool.
				continue
			}
			events = append(events, UnifiedStreamEvent{
				Type: "tool_call_delta",
				ToolCall: &ToolCallEvent{
					Index:     tc.Index,
					ID:        tc.ID,
					Name:      name,
					ArgsDelta: tc.Function.Arguments,
				},
				ID: c.ID, Model: c.Model,
			})
		}
		if ch.FinishReason != "" {
			events = append(events, UnifiedStreamEvent{Type: "finish", Finish: ch.FinishReason, ID: c.ID, Model: c.Model})
		}
	}
	if c.Usage != nil {
		events = append(events, UnifiedStreamEvent{
			Type: "usage",
			Usage: &Usage{
				InputTokens:     c.Usage.PromptTokens,
				OutputTokens:    c.Usage.CompletionTokens,
				ReasoningTokens: orZero(c.Usage.CompletionTokensDetails),
				CacheReadTokens: orZeroCached(c.Usage.PromptTokensDetails),
			},
			ID: c.ID, Model: c.Model,
		})
	}
	return events, newlySuppressed, nil
}

// orZero and orZeroCached read optional nested usage objects. Both details
// blocks are pointers and are absent on plenty of upstream responses, so
// dereferencing one directly panics the request mid-stream — the client sees
// the connection die with no explanation, which is the least diagnosable
// failure this gateway can produce.
func orZero(u *struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}) int {
	if u == nil {
		return 0
	}
	return u.ReasoningTokens
}

func orZeroCached(u *struct {
	CachedTokens int `json:"cached_tokens"`
}) int {
	if u == nil {
		return 0
	}
	return u.CachedTokens
}

// WriteSSE writes one SSE event to the response stream.
func WriteSSE(w io.Writer, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

// WriteSSEData writes a pre-formatted SSE event.
func WriteSSEData(w io.Writer, data []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", bytes.TrimSpace(data))
	return err
}

// WriteSSEEvent writes a named SSE event (Anthropic and Responses styles).
// Clients dispatch on the event name, so a frame sent without one is silently
// dropped — which is how a 200 response can render as empty. To make that
// impossible the name falls back to the payload's "type" field, which every
// event in both protocols carries and which must match the event name anyway.
// Use WriteSSE for protocols whose frames intentionally carry no event name
// (OpenAI chat completions).
func WriteSSEEvent(w io.Writer, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if event == "" {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &probe) == nil {
			event = probe.Type
		}
	}
	if event == "" {
		return fmt.Errorf("sse: event name is empty and payload has no \"type\" field")
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}
