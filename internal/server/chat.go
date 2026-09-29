// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"fmt"
	"net/http"
	"time"

	"opencode-gateway/internal/protocol"
)

// handleChatCompletions serves POST /v1/chat/completions (OpenAI protocol).
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := s.nextRequestID("req")
	body, err := readBody(r)
	if err != nil {
		protocol.WriteJSONError(w, http.StatusBadRequest, "invalid_request_error", "failed to read body")
		return
	}
	req, err := protocol.ParseChatRequest(body)
	if err != nil {
		protocol.WriteJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
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
		protocol.WriteJSONError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		s.recordRejected(reqID, req.Model, "chat", session, http.StatusBadRequest, start)
		return
	}
	model, ok := s.cat.Get(req.Model)
	if !ok {
		protocol.WriteJSONError(w, http.StatusNotFound, "invalid_request_error",
			fmt.Sprintf("model %q not found", req.Model))
		s.recordRejected(reqID, req.Model, "chat", session, http.StatusNotFound, start)
		return
	}
	if s.blacklisted(req.Model) {
		protocol.WriteJSONError(w, http.StatusNotFound, "invalid_request_error", "model not available")
		s.recordRejected(reqID, req.Model, "chat", session, http.StatusNotFound, start)
		return
	}
	channel := "free"
	if !model.Free {
		if !s.zen.HasKeys() || !s.allowPaid() {
			protocol.WriteJSONError(w, http.StatusPaymentRequired, "invalid_request_error",
				"paid model requires a configured zen API key (set zen_keys)")
			s.recordRejected(reqID, req.Model, "chat", session, http.StatusPaymentRequired, start)
			return
		}
		channel = "paid"
	}

	// Both channels flow through the unified event pipeline: the upstream is
	// always zen chat/completions (free tier requires it; paid models whose
	// native protocol differs still accept chat-completions form when the
	// client itself speaks chat).
	chatBody := protocol.BuildChatBody(req)
	if req.MaxTokens <= 0 {
		chatBody["max_tokens"] = 32000
	}

	var upstreamResp *http.Response
	if channel == "paid" {
		payload, err := marshalJSON(chatBody)
		if err != nil {
			protocol.WriteJSONError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		upstreamResp, err = s.zen.SendPaid(r.Context(), "chat", payload, session, false)
	} else {
		upstreamResp, err = s.zen.SendFreeChat(r.Context(), chatBody, session)
	}
	if err != nil {
		s.writeUpstreamError(w, err)
		s.recordUsage(reqID, req.Model, "chat", channel, session, upstreamErrorStatus(err), start, protocol.Usage{})
		return
	}
	defer upstreamResp.Body.Close()

	if req.Stream {
		writer := protocol.NewChatStreamWriter(w, reqID, req.Model)
		agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
		err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
			agg.Add(ev)
			return writer.WriteEvent(ev)
		})
		if err != nil {
			writer.WriteError(err.Error())
			_ = writer.WriteUsageAndDone(agg.Usage, agg.Finish)
			s.recordFailed(reqID, req.Model, "chat", channel, session, upstreamErrorStatus(err), start, agg.Usage, err.Error())
			return
		}
		_ = writer.WriteUsageAndDone(agg.Usage, agg.Finish)
		s.recordUsage(reqID, req.Model, "chat", channel, session, http.StatusOK, start, agg.Usage)
		return
	}

	// Non-streaming: aggregate the stream, render one JSON response.
	agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
	err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
		agg.Add(ev)
		return nil
	})
	if err != nil {
		protocol.WriteJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		s.recordUsage(reqID, req.Model, "chat", channel, session, http.StatusBadGateway, start, agg.Usage)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := agg.Render(w); err != nil {
		s.recordUsage(reqID, req.Model, "chat", channel, session, http.StatusOK, start, agg.Usage)
		return
	}
	s.recordUsage(reqID, req.Model, "chat", channel, session, http.StatusOK, start, agg.Usage)
}

func orDef(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
