// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"opencode-gateway/internal/catalog"
	"opencode-gateway/internal/protocol"
	"opencode-gateway/internal/telemetry"
	"opencode-gateway/internal/upstream"
	"opencode-gateway/internal/usage"
)

// catalogModel aliases the catalog model for handler signatures.
type catalogModel = catalog.Model

// marshalJSON is a helper for body serialization.
func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// writeUpstreamError maps upstream errors to client-facing errors. Upstream
// 4xx codes are preserved so a client-visible problem (unknown model,
// unavailable model, bad request) is not reported as a gateway outage.
func (s *Server) writeUpstreamError(w http.ResponseWriter, err error) {
	if fte, ok := err.(*upstream.FreeTierError); ok {
		protocol.WriteJSONError(w, http.StatusForbidden, "free_tier_error", fte.Error())
		return
	}
	if ue, ok := err.(*upstream.UpstreamError); ok {
		status := ue.Status
		if status < 400 || status > 499 {
			status = http.StatusBadGateway
		}
		protocol.WriteJSONError(w, status, "upstream_error", upstreamErrorMessage(err))
		return
	}
	protocol.WriteJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
}

// upstreamErrorStatus returns the HTTP status to report for an upstream
// failure, preserving client-visible 4xx codes and mapping everything else
// to 502.
func upstreamErrorStatus(err error) int {
	if ue, ok := err.(*upstream.UpstreamError); ok && ue.Status >= 400 && ue.Status < 500 {
		return ue.Status
	}
	return http.StatusBadGateway
}

// upstreamErrorMessage extracts a readable message from an upstream failure,
// unwrapping zen's nested {"error":{"message":...}} envelope when present so
// clients see one clean sentence instead of escaped JSON.
func upstreamErrorMessage(err error) string {
	ue, ok := err.(*upstream.UpstreamError)
	if !ok || ue.Message == "" {
		return err.Error()
	}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(ue.Message), &envelope) == nil && envelope.Error.Message != "" {
		// Strip the gateway-side prefix zen adds to provider errors.
		msg := envelope.Error.Message
		msg = strings.TrimPrefix(msg, "Error from provider (Console): ")
		msg = strings.TrimPrefix(msg, "Upstream request failed: ")
		if msg != "" {
			return msg
		}
	}
	return ue.Message
}

// sessionOf picks the session identifier recorded for a request: an explicit
// hint from the parsed body (Anthropic metadata.user_id, a metadata.session_id
// extension) or a client session header, falling back to the request id — which
// at least groups retries of the same call together instead of leaving the
// console column blank.
func sessionOf(r *http.Request, req *protocol.UnifiedRequest, reqID string) string {
	if req != nil && req.SessionHint != "" {
		return req.SessionHint
	}
	if v := protocol.SessionFromHeaders(r.Header); v != "" {
		return v
	}
	return reqID
}

// requestRecord builds the usage row for a finished request, including the
// catalog-derived cost estimate. The duration is always derived from start
// here rather than passed in: the failure paths used to hand over a literal 0,
// which showed up in the console as "0ms" for requests that had in fact taken
// seconds.
func (s *Server) requestRecord(id, model, proto, channel, session string, status int, start time.Time, u protocol.Usage) usage.RequestRecord {
	rec := usage.RequestRecord{
		ID:              id,
		Time:            start,
		Model:           model,
		Protocol:        proto,
		Channel:         channel,
		Session:         session,
		Status:          status,
		DurationMS:      time.Since(start).Milliseconds(),
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		ReasoningTokens: u.ReasoningTokens,
		CacheReadTokens: u.CacheReadTokens,
	}
	if m, ok := s.cat.Get(model); ok {
		rec.CostUSD = float64(u.InputTokens)/1e6*m.CostInput + float64(u.OutputTokens)/1e6*m.CostOutput
	}
	return rec
}

// logRequest writes the telemetry entry for a finished LLM request, on top of
// the generic HTTP entry written by logRequests. msg carries the reason when
// the request did not complete; without it a failure is a bare status code
// that says nothing about what upstream actually objected to.
func (s *Server) logRequest(model, proto, channel string, status int, start time.Time, u protocol.Usage, msg string) {
	if s.logs == nil {
		return
	}
	level := "info"
	if status >= 400 {
		level = "error"
	}
	s.logs.Add(telemetry.Entry{
		Time:       start,
		Level:      level,
		Kind:       "request",
		Method:     http.MethodPost,
		Path:       pathForProtocol(proto),
		Protocol:   proto,
		Model:      model,
		Channel:    channel,
		Status:     status,
		DurationMS: time.Since(start).Milliseconds(),
		TokensIn:   u.InputTokens,
		TokensOut:  u.OutputTokens,
		Message:    msg,
	})
}

// recordUsage writes a completed request to SQLite and to the in-memory log.
func (s *Server) recordUsage(id, model, proto, channel, session string, status int, start time.Time, u protocol.Usage) {
	if s.usage != nil {
		_ = s.usage.Record(s.requestRecord(id, model, proto, channel, session, status, start, u))
	}
	s.logRequest(model, proto, channel, status, start, u, "")
}

// recordFailed records a request that reached upstream but did not complete:
// a stream that broke, or an upstream error delivered inside a 200 response.
// It exists because the streaming paths used to record http.StatusOK no matter
// what happened — a call the client had already given up on showed up in the
// console as a clean 200 with zero tokens, which is indistinguishable from a
// request that legitimately returned nothing.
func (s *Server) recordFailed(id, model, proto, channel, session string, status int, start time.Time, u protocol.Usage, errMsg string) {
	if status < 400 {
		status = http.StatusBadGateway
	}
	if s.usage != nil {
		_ = s.usage.Record(s.requestRecord(id, model, proto, channel, session, status, start, u))
	}
	s.logRequest(model, proto, channel, status, start, u, errMsg)
}

// pathForProtocol maps a client protocol to its HTTP route, used so LLM log
// entries carry the same path field as generic request entries.
func pathForProtocol(proto string) string {
	switch proto {
	case "anthropic":
		return "/v1/messages"
	case "responses":
		return "/v1/responses"
	default:
		return "/v1/chat/completions"
	}
}

// recordRejected logs a request the gateway refused before contacting
// upstream: unknown model, blacklisted model, a paid model without a
// configured key, or a body that fails validation. Without this the log shows
// only a bare status code with no model name, which makes a misconfigured
// client hard to diagnose.
func (s *Server) recordRejected(id, model, proto, session string, status int, start time.Time) {
	if s.logs != nil {
		entry := telemetry.Entry{
			Time:       start,
			Level:      "warn",
			Kind:       "request",
			Method:     http.MethodPost,
			Path:       pathForProtocol(proto),
			Protocol:   proto,
			Model:      model,
			Status:     status,
			DurationMS: time.Since(start).Milliseconds(),
		}
		switch status {
		case http.StatusPaymentRequired:
			entry.Message = "paid model requires a zen API key (set zen_keys)"
		case http.StatusNotFound:
			entry.Message = "model not available"
		case http.StatusBadRequest:
			entry.Message = "invalid request"
		}
		s.logs.Add(entry)
	}
	if s.usage != nil {
		_ = s.usage.Record(usage.RequestRecord{
			ID: id, Time: start, Model: model, Protocol: proto,
			Session: session,
			Channel: "rejected", Status: status,
			DurationMS: time.Since(start).Milliseconds(),
		})
	}
}
