// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package server wires the HTTP routes and middleware.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opencode-gateway/config"
	"opencode-gateway/internal/catalog"
	"opencode-gateway/internal/protocol"
	"opencode-gateway/internal/telemetry"
	"opencode-gateway/internal/upstream"
	"opencode-gateway/internal/usage"
)

// Server holds all shared state.
type Server struct {
	cfgMu      sync.RWMutex // guards cfg: the admin console edits it at runtime
	cfg        *config.Config
	configPath string
	zen        *upstream.Client
	cat        *catalog.Catalog
	usage      *usage.Store
	logs       *telemetry.Logger
	reqSeq     atomic.Int64
	adminTok   atomic.Value // string
}

// New assembles the server. configPath is where config edits are persisted.
func New(cfg *config.Config, configPath string, zen *upstream.Client, cat *catalog.Catalog, u *usage.Store, logs *telemetry.Logger) *Server {
	s := &Server{cfg: cfg, configPath: configPath, zen: zen, cat: cat, usage: u, logs: logs}
	if cfg.AdminPass != "" {
		s.adminTok.Store(issueToken(cfg.AdminPass))
	}
	return s
}

// Handler builds the full route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Client-facing API
	mux.HandleFunc("GET /v1/models", s.auth(s.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", s.auth(s.handleChatCompletions))
	mux.HandleFunc("POST /v1/messages", s.auth(s.handleMessages))
	mux.HandleFunc("POST /v1/responses", s.auth(s.handleResponses))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Admin API + WebUI
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/overview", s.adminAuth(s.handleOverview))
	mux.HandleFunc("GET /api/usage", s.adminAuth(s.handleUsage))
	mux.HandleFunc("GET /api/requests", s.adminAuth(s.handleRecentRequests))
	mux.HandleFunc("GET /api/config", s.adminAuth(s.handleGetConfig))
	mux.HandleFunc("POST /api/config", s.adminAuth(s.handleSetConfig))
	mux.HandleFunc("POST /api/keys/verify", s.adminAuth(s.handleVerifyKey))
	mux.HandleFunc("GET /admin", s.handleAdminPage)
	mux.HandleFunc("GET /admin/", s.handleAdminPage)
	mux.HandleFunc("GET /api/admin.js", s.serveAdminJS)
	mux.HandleFunc("GET /api/admin.css", s.serveAdminCSS)

	return s.logRequests(mux)
}

// statusRecorder captures the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests wraps every request and records method/path/status/duration.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logs.Add(telemetry.Entry{
			Time:       start,
			Kind:       "request",
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     rec.status,
			DurationMS: time.Since(start).Milliseconds(),
			Remote:     extractHost(r.RemoteAddr),
		})
	})
}

// auth enforces the shared gateway API key. When no key is configured only
// loopback clients are allowed.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.cfgMu.RLock()
		apiKey := s.cfg.APIKey
		s.cfgMu.RUnlock()
		if apiKey == "" {
			host := extractHost(r.RemoteAddr)
			if host != "127.0.0.1" && host != "::1" && host != "[::1]" {
				protocol.WriteJSONError(w, http.StatusUnauthorized, "invalid_request_error",
					"API key not configured; remote access denied. Set api_key in config.")
				return
			}
			next(w, r)
			return
		}
		key := bearerKey(r)
		if key == "" {
			key = r.Header.Get("x-api-key")
		}
		if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
			protocol.WriteJSONError(w, http.StatusUnauthorized, "invalid_request_error",
				"Invalid API key.")
			return
		}
		next(w, r)
	}
}

func extractHost(remote string) string {
	if i := strings.LastIndex(remote, ":"); i > 0 {
		return strings.Trim(remote[:i], "[]")
	}
	return remote
}

func bearerKey(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// StartRefreshLoop refreshes the model catalog periodically.
func (s *Server) StartRefreshLoop(ctx context.Context) {
	interval := time.Duration(s.cfg.Timeout.RefreshSeconds) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	go func() {
		if err := s.cat.Refresh(ctx); err != nil {
			log.Printf("initial model refresh failed: %v", err)
		} else {
			log.Printf("model catalog loaded: %d models", len(s.cat.List()))
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.cat.Refresh(ctx); err != nil {
					log.Printf("model refresh failed: %v", err)
				}
			}
		}
	}()
}

// apiKey returns the current gateway API key.
func (s *Server) apiKey() string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.APIKey
}

// adminPass returns the current admin password.
func (s *Server) adminPass() string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.AdminPass
}

// allowPaid reports whether paid models are exposed.
func (s *Server) allowPaid() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.ModelRules.AllowPaid
}

// blacklisted reports whether a model is hidden.
func (s *Server) blacklisted(id string) bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	for _, b := range s.cfg.ModelRules.ModelBlacklist {
		if b == id {
			return true
		}
	}
	return false
}

// nextRequestID returns a unique request id.
func (s *Server) nextRequestID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), s.reqSeq.Add(1))
}

// issueToken derives the admin session token from the password. It is a
// derived value rather than a random one on purpose: the console keeps its
// token in localStorage, so a token minted fresh on every process start would
// leave every open console silently getting 401s after each gateway restart
// (which looks exactly like "the numbers stopped updating"). Rotating the
// password still invalidates sessions.
func issueToken(pass string) string {
	sum := sha256.Sum256([]byte("opencode-gateway-session:" + pass))
	return hex.EncodeToString(sum[:16])
}

// readBody reads a bounded request body.
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(io.LimitReader(r.Body, 32<<20))
}
