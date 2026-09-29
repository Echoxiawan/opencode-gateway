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

// handleResponses serves POST /v1/responses (OpenAI Responses / Codex).
func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := s.nextRequestID("req")
	body, err := readBody(r)
	if err != nil {
		protocol.WriteResponsesJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	req, err := protocol.ParseResponsesRequest(body)
	if err != nil {
		protocol.WriteResponsesJSONError(w, http.StatusBadRequest, err.Error())
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
		protocol.WriteResponsesJSONError(w, http.StatusBadRequest, "model is required")
		s.recordRejected(reqID, req.Model, "responses", session, http.StatusBadRequest, start)
		return
	}
	model, ok := s.cat.Get(req.Model)
	if !ok {
		protocol.WriteResponsesJSONError(w, http.StatusNotFound, fmt.Sprintf("model %q not found", req.Model))
		s.recordRejected(reqID, req.Model, "responses", session, http.StatusNotFound, start)
		return
	}
	channel := "free"
	if !model.Free {
		if !s.zen.HasKeys() || !s.allowPaid() {
			protocol.WriteResponsesJSONError(w, http.StatusPaymentRequired,
				"paid model requires a configured zen API key (set zen_keys)")
			s.recordRejected(reqID, req.Model, "responses", session, http.StatusPaymentRequired, start)
			return
		}
		channel = "paid"
	}

	chatBody := protocol.BuildChatBody(req)
	if req.MaxTokens > 0 {
		chatBody["max_tokens"] = req.MaxTokens
	} else {
		chatBody["max_tokens"] = 32000
	}

	var upstreamResp *http.Response
	if channel == "paid" {
		payload, err := marshalJSON(chatBody)
		if err != nil {
			protocol.WriteResponsesJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		upstreamResp, err = s.zen.SendPaid(r.Context(), "chat", payload, session, false)
	} else {
		upstreamResp, err = s.zen.SendFreeChat(r.Context(), chatBody, session)
	}
	if err != nil {
		protocol.WriteResponsesJSONError(w, upstreamErrorStatus(err), upstreamErrorMessage(err))
		s.recordUsage(reqID, req.Model, "responses", channel, session, upstreamErrorStatus(err), start, protocol.Usage{})
		return
	}
	defer upstreamResp.Body.Close()

	if req.Stream {
		sw := protocol.NewResponsesStreamWriter(w, req.Model)
		agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
		err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
			agg.Add(ev)
			return sw.WriteEvent(ev)
		})
		if err != nil {
			sw.WriteError(err.Error())
			sw.Finish()
			s.recordFailed(reqID, req.Model, "responses", channel, session, upstreamErrorStatus(err), start, agg.Usage, err.Error())
			return
		}
		sw.Finish()
		s.recordUsage(reqID, req.Model, "responses", channel, session, http.StatusOK, start, agg.Usage)
		return
	}

	agg := &protocol.AggregateChatResponse{ID: reqID, Model: req.Model}
	err = protocol.ParseSSEStream(upstreamResp.Body, streamOpts, func(ev protocol.UnifiedStreamEvent) error {
		agg.Add(ev)
		return nil
	})
	if err != nil {
		protocol.WriteResponsesJSONError(w, http.StatusBadGateway, err.Error())
		s.recordUsage(reqID, req.Model, "responses", channel, session, http.StatusBadGateway, start, agg.Usage)
		return
	}
	if err := protocol.RenderResponsesMessage(w, req.Model, agg); err != nil {
		return
	}
	s.recordUsage(reqID, req.Model, "responses", channel, session, http.StatusOK, start, agg.Usage)
}
