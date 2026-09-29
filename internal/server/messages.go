// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"opencode-gateway/internal/protocol"
)

// handleMessages serves POST /v1/messages (Anthropic protocol).
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := s.nextRequestID("msg")
	body, err := readBody(r)
	if err != nil {
		protocol.WriteAnthropicJSONError(w, http.StatusBadRequest, "invalid_request_error", "failed to read body")
		return
	}
	req, err := protocol.ParseAnthropicRequest(body)
	if err != nil {
		protocol.WriteAnthropicJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	// Resolved once, up front: every path below records it, including the
	// rejects, so a client that is simply misconfigured is identifiable in
	// the console instead of showing up as an anonymous failure.
	session := sessionOf(r, req, reqID)
	// The free channel replaces the client's tools with the signature stubs the
	// upstream validates, so the model may call a stub by its lowercase name.
	// Aliasing hands those calls back under the name the client declared.
	streamOpts := protocol.StreamOptions{ToolAliases: protocol.ToolAliases(req.Tools)}
	if req.Model == "" {
		protocol.WriteAnthropicJSONError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		s.recordRejected(reqID, req.Model, "anthropic", session, http.StatusBadRequest, start)
		return
	}
	model, ok := s.cat.Get(req.Model)
	if !ok {
		protocol.WriteAnthropicJSONError(w, http.StatusNotFound, "invalid_request_error",
			fmt.Sprintf("model %q not found", req.Model))
		s.recordRejected(reqID, req.Model, "anthropic", session, http.StatusNotFound, start)
		return
	}
	channel := "free"
	if !model.Free {
		if !s.zen.HasKeys() || !s.allowPaid() {
			protocol.WriteAnthropicJSONError(w, http.StatusPaymentRequired, "invalid_request_error",
				"paid model requires a configured zen API key (set zen_keys)")
			s.recordRejected(reqID, req.Model, "anthropic", session, http.StatusPaymentRequired, start)
			return
		}
		channel = "paid"
	}
	if req.MaxTokens <= 0 {
		protocol.WriteAnthropicJSONError(w, http.StatusBadRequest, "invalid_request_error", "max_tokens is required")
		s.recordRejected(reqID, req.Model, "anthropic", session, http.StatusBadRequest, start)
		return
	}

	// Render into upstream chat body (unified event pipeline).
	chatBody := protocol.BuildChatBody(req)
	chatBody["max_tokens"] = req.MaxTokens

	var upstreamResp *http.Response
	if channel == "paid" {
		payload, err := marshalJSON(chatBody)
		if err != nil {
			protocol.WriteAnthropicJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		upstreamResp, err = s.zen.SendPaid(r.Context(), "chat", payload, session, false)
	} else {
		upstreamResp, err = s.zen.SendFreeChat(r.Context(), chatBody, session)
	}
	if err != nil {
		s.writeAnthropicUpstreamError(w, err)
		s.recordUsage(reqID, req.Model, "anthropic", channel, session, upstreamErrorStatus(err), start, protocol.Usage{})
		return
	}
	defer upstreamResp.Body.Close()

	if req.Stream {
		sw := protocol.NewAnthropicStreamWriter(w, req.Model)
		agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
		err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
			agg.Add(ev)
			return sw.WriteEvent(ev)
		})
		if err != nil {
			sw.WriteError(err.Error())
			sw.Finish(agg.Finish)
			s.recordFailed(reqID, req.Model, "anthropic", channel, session, upstreamErrorStatus(err), start, agg.Usage, err.Error())
			return
		}
		sw.Finish(agg.Finish)
		s.recordUsage(reqID, req.Model, "anthropic", channel, session, http.StatusOK, start, agg.Usage)
		return
	}

	// Non-streaming: aggregate and render an Anthropic message object.
	agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
	err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
		agg.Add(ev)
		return nil
	})
	if err != nil {
		protocol.WriteAnthropicJSONError(w, http.StatusBadGateway, "api_error", err.Error())
		s.recordUsage(reqID, req.Model, "anthropic", channel, session, http.StatusBadGateway, start, agg.Usage)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := renderAnthropicMessage(w, req, agg); err != nil {
		return
	}
	s.recordUsage(reqID, req.Model, "anthropic", channel, session, http.StatusOK, start, agg.Usage)
}

// renderAnthropicMessage renders the aggregate as an Anthropic message.
func renderAnthropicMessage(w http.ResponseWriter, req *protocol.UnifiedRequest, agg *protocol.AggregateChatResponse) error {
	var content []map[string]any
	if agg.Reason.Len() > 0 {
		content = append(content, map[string]any{
			"type":     "thinking",
			"thinking": agg.Reason.String(),
		})
	}
	if agg.Content.Len() > 0 || len(agg.ToolCalls) == 0 {
		content = append(content, map[string]any{
			"type": "text",
			"text": agg.Content.String(),
		})
	}
	for i, tc := range agg.ToolCalls {
		args := tc.Arguments.String()
		var input any
		if err := json.Unmarshal([]byte(args), &input); err != nil {
			input = map[string]any{}
		}
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("toolu_gw_%d", i)
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  tc.Name,
			"input": input,
		})
	}
	stopReason := "end_turn"
	if len(agg.ToolCalls) > 0 {
		stopReason = "tool_use"
	} else if agg.Finish == "length" {
		stopReason = "max_tokens"
	}
	msg := map[string]any{
		"id":            orDef(agg.ID, "msg_gw"),
		"type":          "message",
		"role":          "assistant",
		"model":         req.Model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":                agg.Usage.InputTokens,
			"output_tokens":               agg.Usage.OutputTokens,
			"cache_creation_input_tokens": agg.Usage.CacheWriteTokens,
			"cache_read_input_tokens":     agg.Usage.CacheReadTokens,
		},
	}
	return json.NewEncoder(w).Encode(msg)
}

func (s *Server) writeAnthropicUpstreamError(w http.ResponseWriter, err error) {
	protocol.WriteAnthropicJSONError(w, upstreamErrorStatus(err), "api_error", upstreamErrorMessage(err))
}
