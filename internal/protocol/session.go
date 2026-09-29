// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

package protocol

import (
	"net/http"
	"strings"
)

// sessionMaxLen bounds what is stored per request. Session ids come from the
// client and end up in the usage database and the console table, so a client
// sending a novel-sized header should not be able to bloat either.
const sessionMaxLen = 64

// sessionHeaders are the header names clients use to announce a session.
// OpenCode sends its own; Codex CLI sends session_id; the rest cover common
// conventions. Only used as a fallback when the body carries nothing.
var sessionHeaders = []string{
	"x-opencode-session",
	"session_id",
	"x-session-id",
	"x-claude-code-session-id",
}

// SessionFromHeaders returns the client's session identifier, or "" when it
// sent none.
func SessionFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	for _, name := range sessionHeaders {
		if v := compactSession(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

// SessionFromUserID pulls a session id out of an Anthropic metadata.user_id.
// Claude Code sends "user_<hash>_account_<hash>_session_<uuid>", where only the
// trailing part changes per conversation: keeping the whole string would make
// every session look like the same client, and keeping nothing loses the only
// session marker that protocol carries. Other clients put an arbitrary end-user
// id there, which is still the most useful grouping available.
func SessionFromUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if i := strings.Index(userID, "session_"); i >= 0 {
		return compactSession(userID[i:])
	}
	return compactSession(userID)
}

func compactSession(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > sessionMaxLen {
		s = s[:sessionMaxLen]
	}
	return s
}
