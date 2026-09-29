// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package telemetry records structured log entries (HTTP requests and
// application events) to daily log files, keeping a bounded number of days
// on disk. It also acts as an io.Writer so the standard log package lands in
// the same files.
package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry is one structured log record.
type Entry struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"` // info | warn | error
	Kind  string    `json:"kind"`  // request | event

	// HTTP request fields.
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     int    `json:"status,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Remote     string `json:"remote,omitempty"`

	// LLM request fields.
	Model     string `json:"model,omitempty"`
	Channel   string `json:"channel,omitempty"` // free | paid
	Protocol  string `json:"protocol,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`

	// Application event message.
	Message string `json:"message,omitempty"`
}

// Logger writes JSONL log entries into daily files under dir and prunes
// files older than retention days.
type Logger struct {
	mu        sync.Mutex
	dir       string
	retention int
	file      *os.File
	fileDay   string
}

// New creates a logger writing to dir/gateway-YYYY-MM-DD.log, retaining
// retention days of history. A dir of "" disables file output (entries are
// dropped), which keeps the gateway usable in read-only environments.
func New(dir string, retention int) *Logger {
	if retention <= 0 {
		retention = 7
	}
	l := &Logger{dir: dir, retention: retention}
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		l.prune()
	}
	return l
}

// Dir returns the log directory (empty when file logging is disabled).
func (l *Logger) Dir() string { return l.dir }

// Add appends an entry to today's log file.
func (l *Logger) Add(e Entry) {
	if l.dir == "" {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	if e.Level == "" {
		e.Level = "info"
	}
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')

	day := e.Time.Format("2006-01-02")
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureFile(day); err != nil {
		return
	}
	_, _ = l.file.Write(line)
}

// Log records an application event.
func (l *Logger) Log(level, msg string) {
	l.Add(Entry{Level: level, Kind: "event", Message: msg})
}

// Infof / Warnf / Errorf are convenience helpers.
func (l *Logger) Infof(format string, args ...any)  { l.Log("info", sprintf(format, args...)) }
func (l *Logger) Warnf(format string, args ...any)  { l.Log("warn", sprintf(format, args...)) }
func (l *Logger) Errorf(format string, args ...any) { l.Log("error", sprintf(format, args...)) }

// Write implements io.Writer so the standard log package can emit into the
// daily log file. The level is inferred from the message text.
func (l *Logger) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	level := "info"
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "error"), strings.Contains(lower, "fail"):
		level = "error"
	case strings.Contains(lower, "warn"):
		level = "warn"
	}
	l.Log(level, msg)
	return len(p), nil
}

// Close flushes and closes the current log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// ensureFile opens the file for day if it is not already open. Caller holds mu.
func (l *Logger) ensureFile(day string) error {
	if l.file != nil && l.fileDay == day {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	f, err := os.OpenFile(l.path(day), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	l.fileDay = day

	// A new day started: drop anything past the retention window.
	l.pruneLocked()
	return nil
}

func (l *Logger) path(day string) string {
	return filepath.Join(l.dir, "gateway-"+day+".log")
}

// prune removes log files older than the retention window.
func (l *Logger) prune() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked()
}

// pruneLocked drops log files whose date is outside the retention window.
// Caller holds mu.
func (l *Logger) pruneLocked() {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -(l.retention - 1)).Format("2006-01-02")
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "gateway-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, "gateway-"), ".log")
		// Keep files with the current day or later, and anything within
		// the retention window.
		if day >= cutoff {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_ = os.Remove(filepath.Join(l.dir, name))
	}
}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
