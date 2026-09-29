// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"net/http"
)

// handleModels serves GET /v1/models with free/paid markers and usage stats.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := []map[string]any{}
	for _, m := range s.cat.List() {
		// Paid models need keys; hide them when unconfigured.
		if !m.Free && !s.zen.HasKeys() {
			continue
		}
		if s.blacklisted(m.ID) {
			continue
		}
		data = append(data, map[string]any{
			"id":       m.ID,
			"object":   "model",
			"created":  1790587287,
			"owned_by": "opencode",
			"free":     m.Free,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}
