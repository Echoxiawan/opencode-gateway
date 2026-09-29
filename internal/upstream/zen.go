// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package upstream talks to OpenCode Zen: free-tier requests with the
// opencode client signature, and paid direct forwarding.
package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Client is the zen HTTP client shared by all request paths.
type Client struct {
	upstream string
	http     *http.Client

	// keyIndex rotates through configured paid keys.
	keyMu  sync.Mutex
	keyIdx int
	Keys   []string
}

// NewClient creates a zen client.
func NewClient(upstream string, keys []string, timeout time.Duration) *Client {
	return &Client{
		upstream: strings.TrimRight(upstream, "/"),
		http: &http.Client{
			Timeout: timeout,
		},
		Keys: keys,
	}
}

// Upstream returns the base URL.
func (c *Client) Upstream() string { return c.upstream }

// HasKeys reports whether paid keys are configured.
func (c *Client) HasKeys() bool { return len(c.Keys) > 0 }

// nextKey returns the next paid key round-robin.
func (c *Client) nextKey() (string, bool) {
	if len(c.Keys) == 0 {
		return "", false
	}
	c.keyMu.Lock()
	k := c.Keys[c.keyIdx%len(c.Keys)]
	c.keyIdx++
	c.keyMu.Unlock()
	return k, true
}

// FetchModelsJSON fetches the raw /v1/models response (works anonymously).
func (c *Client) FetchModelsJSON(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upstream+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer public")
	req.Header.Set("X-Opencode-Client", "cli")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("models endpoint HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// UserAgent mimics the opencode CLI signature. The free tier checks it.
// Dynamic like opencode2api: GOOS/GOARCH come from the runtime so the string
// is correct on Linux servers too.
var UserAgent = fmt.Sprintf("opencode/1.18.31 (%s %s; %s)", runtime.GOOS, runtime.GOARCH, runtime.Version())

// Required free-tier tool names. The zen free tier (Bearer public) only
// accepts requests whose tools array contains both.
// freeTierTools are the core tool names the Zen free tier looks for.
// Requests that do not carry all five are rejected with 403 FreeTierError.
// The reference implementation (opencode2api) confirms all five must be
// present; two was a conservative estimate that was never validated upstream.
var freeTierTools = []string{"bash", "read", "edit", "glob", "grep"}

// canonicalSessionPattern matches OpenCode's canonical session format:
// "ses_" + 12 lowercase hex characters + 14 Base62 characters.
// Since 2026-09-16 the zen free tier rejects other session shapes with 403.
var canonicalSessionPattern = regexp.MustCompile(`^ses_[0-9a-f]{12}[0-9A-Za-z]{14}$`)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// CanonicalSessionID deterministically maps any session signal to the
// canonical shape (pass-through when already canonical).
func CanonicalSessionID(signal string) string {
	if canonicalSessionPattern.MatchString(signal) {
		return signal
	}
	sum := sha256.Sum256([]byte("ses\x00" + signal))
	timePart := hex.EncodeToString(sum[:6])
	randomPart := base62Fixed(new(big.Int).SetBytes(sum[6:16]), 14)
	return "ses_" + timePart + randomPart
}

func base62Fixed(n *big.Int, width int) string {
	base := big.NewInt(62)
	out := make([]byte, width)
	remainder := new(big.Int)
	for i := width - 1; i >= 0; i-- {
		n.DivMod(n, base, remainder)
		out[i] = base62Alphabet[remainder.Int64()]
	}
	return string(out)
}

// StableID hashes value into a deterministic "{prefix}_{hex12}" identifier,
// matching opencode2api's identity.StableID. Used for x-opencode-project so
// the same project keeps a stable ID across sessions.
func StableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(prefix + "\x00" + value))
	return prefix + "_" + hex.EncodeToString(sum[:12])
}

// RandomID generates a random "{prefix}_{hex}" identifier for x-opencode-request.
// Each request must carry a unique ID so Zen can deduplicate retries.
func RandomID(prefix string, size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is essentially impossible but must not hang.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// upstreamErr is a structured zen error response.
type upstreamErr struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// SendFreeChat issues a free-tier chat/completions request. The caller must
// supply a request body that already includes the client's messages/tools;
// this function injects the opencode signature (bash/read tools, stream:true,
// canonical headers). It always requests SSE, even when the caller wants a
// non-streaming result — the gateway aggregates.
//
// The response body is the raw SSE stream; the caller parses events.
func (c *Client) SendFreeChat(ctx context.Context, body map[string]any, sessionSignal string) (*http.Response, error) {
	// Force stream + ensure the five core tool names are present.
	// The Zen free tier (Bearer public) rejects requests that do not look like
	// a real agent session. Presence of bash/read/edit/glob/grep is validated
	// server-side; the bodies are never executed.
	//
	// Strategy (matching opencode2api): append minimal stubs only for the
	// core tools the client did NOT already declare. Client tools that happen
	// to share a name are left untouched; client tools not in the core list
	// are also kept so tool_result history in the conversation stays coherent.
	body["stream"] = true
	body["stream_options"] = map[string]any{"include_usage": true}

	existing, _ := body["tools"].([]any)
	present := make(map[string]bool, len(existing))
	for _, t := range existing {
		if tm, ok := t.(map[string]any); ok {
			// OpenAI chat format: {"type":"function","function":{"name":...}}
			if fn, ok := tm["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					present[name] = true
				}
			}
		}
	}
	for _, name := range freeTierTools {
		if present[name] {
			continue
		}
		existing = append(existing, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": "Agent tool " + name,
				"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
			},
		})
	}
	body["tools"] = existing

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	session := CanonicalSessionID(sessionSignal)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.upstream+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	// x-opencode-request: per-request unique ID (opencode2api reference: RandomID)
	reqID := RandomID("req", 16)
	// x-opencode-project: stable project hash derived from session, matching
	// opencode2api's StableID("prj", signal) pattern.
	projectID := StableID("prj", sessionSignal)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("x-opencode-client", "cli")
	req.Header.Set("x-opencode-session", session)
	req.Header.Set("X-Session-Id", session)
	req.Header.Set("x-session-affinity", session)
	req.Header.Set("x-opencode-request", reqID)
	req.Header.Set("x-opencode-project", projectID)
	req.Header.Set("Authorization", "Bearer public")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		var ue upstreamErr
		_ = json.Unmarshal(body, &ue)
		if ue.Type == "FreeTierError" || strings.Contains(string(body), "FreeTierError") {
			return nil, &FreeTierError{Message: ue.Message, Body: string(body)}
		}
		return nil, &UpstreamError{Status: resp.StatusCode, Message: string(body)}
	}
	return resp, nil
}

// UpstreamError carries a non-2xx response from zen so callers can preserve
// the upstream semantics (a 400 "model unavailable" must not look like a
// gateway outage to the client).
type UpstreamError struct {
	Status  int
	Message string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream HTTP %d: %s", e.Status, e.Message)
}

// FreeTierError signals the upstream rejected a free-tier request.
type FreeTierError struct {
	Message string
	Body    string
}

func (e *FreeTierError) Error() string {
	if e.Message != "" {
		return "zen free tier rejected request: " + e.Message
	}
	return "zen free tier rejected request: " + e.Body
}

// SendPaid forwards a request with a paid key to the given protocol endpoint.
// The body must already be in the target protocol's wire format.
func (c *Client) SendPaid(ctx context.Context, protocol string, body []byte, sessionSignal string, isAnthropic bool) (*http.Response, error) {
	key, ok := c.nextKey()
	if !ok {
		return nil, fmt.Errorf("no paid keys configured")
	}
	endpoint := c.upstream + pathFor(protocol)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+key)
	session := CanonicalSessionID(sessionSignal)
	req.Header.Set("X-Opencode-Session", session)
	if isAnthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, &UpstreamError{Status: resp.StatusCode, Message: string(body)}
	}
	return resp, nil
}

func pathFor(protocol string) string {
	switch protocol {
	case "responses":
		return "/v1/responses"
	case "anthropic":
		return "/v1/messages"
	default:
		return "/v1/chat/completions"
	}
}

// VerifyKey checks a paid key by hitting a lightweight authenticated
// endpoint. It returns the raw status so callers can report precisely.
func (c *Client) VerifyKey(ctx context.Context, key string) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.upstream+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-5-nano","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return resp.StatusCode, string(body)
}
