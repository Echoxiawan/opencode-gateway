// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package usage records per-request token usage in SQLite and serves
// aggregates.
package usage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the usage database.
type Store struct {
	mu sync.Mutex
	db *sql.DB
}

// RequestRecord is one completed request.
type RequestRecord struct {
	ID              string
	Time            time.Time
	Model           string
	Protocol        string // client protocol: chat / anthropic / responses
	Channel         string // free / paid / rejected
	Session         string // session hint sent upstream (or the request id)
	Status          int
	DurationMS      int64
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	CacheReadTokens int
	CostUSD         float64
}

// ModelUsage aggregates usage for one model.
type ModelUsage struct {
	Model         string  `json:"model"`
	Requests      int64   `json:"requests"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	TodayRequests int64   `json:"today_requests"`
	TodayInput    int64   `json:"today_input_tokens"`
	TodayOutput   int64   `json:"today_output_tokens"`
	CostUSD       float64 `json:"cost_usd"`
	LastUsed      string  `json:"last_used"`
}

// Open initializes the store under dataDir.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "usage.db")
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open usage db: %w", err)
	}
	schema := `
CREATE TABLE IF NOT EXISTS requests (
    id TEXT PRIMARY KEY,
    time INTEGER NOT NULL,
    model TEXT NOT NULL,
    protocol TEXT NOT NULL,
    channel TEXT NOT NULL,
    session TEXT NOT NULL DEFAULT '',
    status INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    input_tokens INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    reasoning_tokens INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_requests_time ON requests(time);
CREATE INDEX IF NOT EXISTS idx_requests_model ON requests(model);
`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("init usage db: %w", err)
	}
	// Databases written before the session column existed are upgraded in
	// place; the duplicate-column error on a current schema is expected.
	if _, err := db.Exec(`ALTER TABLE requests ADD COLUMN session TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		return nil, fmt.Errorf("migrate usage db: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Record inserts one request record (best-effort, errors are logged upstream).
func (s *Store) Record(rec RequestRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO requests (id, time, model, protocol, channel, session, status, duration_ms, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cost_usd)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.Time.Unix(), rec.Model, rec.Protocol, rec.Channel, rec.Session, rec.Status,
		rec.DurationMS, rec.InputTokens, rec.OutputTokens, rec.ReasoningTokens, rec.CacheReadTokens, rec.CostUSD,
	)
	return err
}

// dayStartUnix returns the Unix second at which the local day containing now
// began, for a caller whose zone sits offsetMin minutes east of UTC.
//
// The offset comes from the browser (see /api/*?tz=) because day boundaries
// must follow the viewer's calendar, not the gateway host's: a UTC server
// would otherwise roll "today" over at 08:00 Beijing time.
func dayStartUnix(now time.Time, offsetMin int) int64 {
	shifted := now.UTC().Add(time.Duration(offsetMin) * time.Minute)
	y, m, d := shifted.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return midnight.Add(-time.Duration(offsetMin) * time.Minute).Unix()
}

// localDate renders a local-midnight stamp (Unix seconds) as YYYY-MM-DD.
func localDate(stamp int64, offsetMin int) string {
	return time.Unix(stamp+int64(offsetMin)*60, 0).UTC().Format("2006-01-02")
}

// ModelAggregates returns per-model usage (all time + today). today is
// resolved against the caller's timezone offset so "today" means the viewer's
// calendar day.
func (s *Store) ModelAggregates(offsetMin int) (map[string]*ModelUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := dayStartUnix(time.Now(), offsetMin)
	rows, err := s.db.Query(`
		SELECT model,
		       COUNT(*), SUM(input_tokens), SUM(output_tokens), SUM(cost_usd),
		       MAX(time),
		       SUM(CASE WHEN time >= ? THEN 1 ELSE 0 END),
		       SUM(CASE WHEN time >= ? THEN input_tokens ELSE 0 END),
		       SUM(CASE WHEN time >= ? THEN output_tokens ELSE 0 END)
		FROM requests GROUP BY model`, today, today, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*ModelUsage{}
	for rows.Next() {
		var m ModelUsage
		var lastTime int64
		var cost sql.NullFloat64
		if err := rows.Scan(&m.Model, &m.Requests, &m.InputTokens, &m.OutputTokens, &cost, &lastTime,
			&m.TodayRequests, &m.TodayInput, &m.TodayOutput); err != nil {
			return nil, err
		}
		if cost.Valid {
			m.CostUSD = cost.Float64
		}
		if lastTime > 0 {
			m.LastUsed = time.Unix(lastTime, 0).Format(time.RFC3339)
		}
		out[m.Model] = &m
	}
	return out, rows.Err()
}

// DailySeries returns daily token totals for the last N days, days that saw
// no traffic included as zeroes so the chart plots a continuous calendar
// rather than only the days that happen to have rows.
func (s *Store) DailySeries(days, offsetMin int) ([]map[string]any, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	offSec := int64(offsetMin) * 60
	today := dayStartUnix(time.Now(), offsetMin)
	since := today - int64(days-1)*86400
	rows, err := s.db.Query(`
		SELECT (time + ?) / 86400 AS day, SUM(input_tokens), SUM(output_tokens), COUNT(*)
		FROM requests WHERE time >= ? GROUP BY day ORDER BY day`, offSec, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[int64]map[string]any{}
	for rows.Next() {
		var day int64
		var in, outT, cnt int64
		if err := rows.Scan(&day, &in, &outT, &cnt); err != nil {
			return nil, err
		}
		byDay[day] = map[string]any{
			"input_tokens":  in,
			"output_tokens": outT,
			"requests":      cnt,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, days)
	for i := 0; i < days; i++ {
		stamp := since + int64(i)*86400
		entry := map[string]any{
			"date":          localDate(stamp, offsetMin),
			"input_tokens":  int64(0),
			"output_tokens": int64(0),
			"requests":      int64(0),
		}
		if hits, ok := byDay[(stamp+offSec)/86400]; ok {
			for k, v := range hits {
				entry[k] = v
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// RecentRequests returns the latest N request records, optionally bounded by
// a [sinceMS, untilMS) window in Unix milliseconds (0 means unbounded).
func (s *Store) RecentRequests(limit int, sinceMS, untilMS int64) ([]map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	query := `
		SELECT id, time, model, protocol, channel, session, status, duration_ms,
		       input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cost_usd
		FROM requests WHERE 1 = 1`
	var args []any
	if sinceMS > 0 {
		query += ` AND time >= ?`
		args = append(args, sinceMS/1000)
	}
	if untilMS > 0 {
		query += ` AND time < ?`
		args = append(args, untilMS/1000)
	}
	query += ` ORDER BY time DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, model, proto, channel, session string
		var t int64
		var status, dur, in, outT, reason, cacheRead int64
		var cost sql.NullFloat64
		if err := rows.Scan(&id, &t, &model, &proto, &channel, &session, &status, &dur,
			&in, &outT, &reason, &cacheRead, &cost); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":                id,
			"time":              time.Unix(t, 0).Format(time.RFC3339),
			"time_ms":           t * 1000,
			"model":             model,
			"protocol":          proto,
			"channel":           channel,
			"session":           session,
			"status":            status,
			"duration_ms":       dur,
			"input_tokens":      in,
			"output_tokens":     outT,
			"reasoning_tokens":  reason,
			"cache_read_tokens": cacheRead,
			"cost_usd":          cost.Float64,
		})
	}
	return out, rows.Err()
}
