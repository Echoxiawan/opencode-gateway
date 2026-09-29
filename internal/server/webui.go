// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"net/http"

	"opencode-gateway/internal/webui"
)

// noStore marks console assets as uncacheable. Their URLs never change, so
// without this a browser happily reuses a stale app.js after the gateway is
// upgraded — a console frozen on old numbers, or on old buggy polling, and no
// way for the user to tell. The assets are a few KB served from memory, so
// re-fetching each load costs nothing worth caching for.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
}

// handleAdminPage serves the admin console HTML.
func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	if s.adminPass() == "" {
		http.Error(w, "admin password not configured", http.StatusForbidden)
		return
	}
	noStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(webui.IndexHTML)
}

// serveAdminJS serves the admin JS.
func (s *Server) serveAdminJS(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write(webui.AppJS)
}

// serveAdminCSS serves the admin CSS.
func (s *Server) serveAdminCSS(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(webui.StylesCSS)
}
