// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"opencode-gateway/config"
)

// handleLogin exchanges the admin password for a session token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	pass := s.adminPass()
	if pass == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "admin password not configured"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(pass)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "wrong password"})
		return
	}
	tok, _ := s.adminTok.Load().(string)
	writeJSON(w, http.StatusOK, map[string]any{"token": tok})
}

// adminAuth guards admin APIs with the session token.
func (s *Server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminPass() == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin password not configured"})
			return
		}
		auth := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(auth, "Bearer ")
		want, _ := s.adminTok.Load().(string)
		if want == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// handleOverview returns models + usage summary. ?tz=<minutes east of UTC>
// makes the "today" figures follow the console viewer's calendar day.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	models := s.cat.List()
	usageByModel, _ := s.usage.ModelAggregates(tzOffset(r))
	out := []map[string]any{}
	for _, m := range models {
		if m.Free || s.zen.HasKeys() {
			entry := map[string]any{
				"id":                m.ID,
				"free":              m.Free,
				"cost_input_per_m":  m.CostInput,
				"cost_output_per_m": m.CostOutput,
			}
			if u, ok := usageByModel[m.ID]; ok {
				entry["usage"] = u
			}
			out = append(out, entry)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models":     out,
		"fetched_at": s.cat.FetchedAt(),
		"loading":    s.cat.FetchedAt().IsZero(),
		"upstream":   s.zen.Upstream(),
		"has_keys":   s.zen.HasKeys(),
	})
}

// handleUsage returns the daily usage series. ?days=N (default 30) and
// ?tz=<minutes east of UTC> control the window and its day boundaries.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	days := queryInt(r, "days", 30)
	series, err := s.usage.DailySeries(days, tzOffset(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": series, "days": days})
}

// handleRecentRequests returns recent request records. The caller may pass
// ?limit=N (default 200, max 1000) and an optional time window in Unix
// milliseconds (?since_ms= / ?until_ms=).
func (s *Server) handleRecentRequests(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n := atoi(v); n > 0 && n <= 1000 {
			limit = n
		}
	}
	recs, err := s.usage.RecentRequests(limit, queryInt64(r, "since_ms"), queryInt64(r, "until_ms"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": recs})
}

// handleGetConfig returns the config with upstream keys redacted. The
// gateway API key is returned in full so the console can display it for
// client setup (it is already protected by the admin session).
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.cfgMu.RLock()
	cfgCopy := *s.cfg
	cfgCopy.ZenKeys = append([]string(nil), s.cfg.ZenKeys...)
	cfgCopy.ModelRules.ModelBlacklist = append([]string(nil), s.cfg.ModelRules.ModelBlacklist...)
	s.cfgMu.RUnlock()

	if len(cfgCopy.ZenKeys) > 0 {
		cfgCopy.ZenKeys = redactKeys(cfgCopy.ZenKeys)
	}
	cfgCopy.AdminPass = ""
	writeJSON(w, http.StatusOK, cfgCopy)
}

// handleSetConfig updates gateway credentials, zen keys and model rules, and
// persists them to the config file.
func (s *Server) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APIKey    *string   `json:"api_key"`
		AdminPass *string   `json:"admin_password"`
		ZenKeys   *[]string `json:"zen_keys"`
		AllowPaid *bool     `json:"allow_paid"`
		Blacklist *[]string `json:"model_blacklist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}

	s.cfgMu.Lock()
	if body.APIKey != nil {
		key := strings.TrimSpace(*body.APIKey)
		// An empty value means "allow localhost only".
		s.cfg.APIKey = key
	}
	passChanged := false
	if body.AdminPass != nil {
		if pass := strings.TrimSpace(*body.AdminPass); pass != "" {
			s.cfg.AdminPass = pass
			passChanged = true
		}
	}
	if body.ZenKeys != nil {
		// Skip redacted placeholder values (meaning "unchanged").
		var keys []string
		for _, k := range *body.ZenKeys {
			k = strings.TrimSpace(k)
			if k == "" || strings.HasPrefix(k, "sk-***") {
				continue
			}
			keys = append(keys, k)
		}
		s.cfg.ZenKeys = keys
	}
	if body.AllowPaid != nil {
		s.cfg.ModelRules.AllowPaid = *body.AllowPaid
	}
	if body.Blacklist != nil {
		s.cfg.ModelRules.ModelBlacklist = *body.Blacklist
	}
	// Snapshot for persisting and for the upstream client.
	snapshot := *s.cfg
	snapshot.ZenKeys = append([]string(nil), s.cfg.ZenKeys...)
	snapshot.ModelRules.ModelBlacklist = append([]string(nil), s.cfg.ModelRules.ModelBlacklist...)
	s.cfgMu.Unlock()

	s.zen.Keys = snapshot.ZenKeys
	if passChanged {
		// Invalidate every existing console session on password change.
		s.adminTok.Store(issueToken(snapshot.AdminPass))
	}

	if s.configPath != "" {
		if err := config.Save(s.configPath, &snapshot); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"saved_to":        s.configPath,
		"api_key_changed": body.APIKey != nil,
	})
}

// handleVerifyKey tests a zen paid key.
func (s *Server) handleVerifyKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	if body.Key == "" || strings.HasPrefix(body.Key, "sk-***") {
		// verify existing configured keys
		if len(s.zen.Keys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no key given and none configured"})
			return
		}
		body.Key = s.zen.Keys[0]
	}
	status, resp := s.zen.VerifyKey(r.Context(), body.Key)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"valid":  status == 200,
		"detail": resp,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func redactKeys(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = redactKey(k)
	}
	return out
}

func redactKey(k string) string {
	if len(k) <= 8 {
		return "***"
	}
	return k[:5] + "***" + k[len(k)-4:]
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func queryInt(r *http.Request, name string, def int) int {
	if v := r.URL.Query().Get(name); v != "" {
		if n := atoi(v); n != 0 {
			return n
		}
	}
	return def
}

func queryInt64(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

// tzOffset reads the console viewer's UTC offset in minutes (JS convention:
// -new Date().getTimezoneOffset()). Values outside ±14h are treated as
// missing, so a bad query string cannot slide the day boundary arbitrarily.
func tzOffset(r *http.Request) int {
	n := queryInt(r, "tz", 0)
	if n < -840 || n > 840 {
		return 0
	}
	return n
}
