// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package config loads gateway configuration from a JSON file and
// environment variables.
package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
)

// Config is the full gateway configuration.
type Config struct {
	Listen     string        `json:"listen"`
	APIKey     string        `json:"api_key"`
	ZenKeys    []string      `json:"zen_keys"`
	Upstream   string        `json:"upstream"`
	DataDir    string        `json:"data_dir"`
	AdminPass  string        `json:"admin_password"`
	Timeout    TimeoutConfig `json:"timeout"`
	ModelRules ModelRules    `json:"model_rules"`
}

// TimeoutConfig controls request timeouts.
type TimeoutConfig struct {
	RequestSeconds int `json:"request_seconds"`
	RefreshSeconds int `json:"refresh_seconds"`
}

// ModelRules controls how models are exposed and routed.
type ModelRules struct {
	// AllowPaid toggles exposing paid models (requires zen keys).
	AllowPaid bool `json:"allow_paid"`
	// ModelBlacklist hides specific model IDs from /v1/models and rejects them.
	ModelBlacklist []string `json:"model_blacklist"`
}

// Defaults returns the built-in configuration.
func Defaults() *Config {
	return &Config{
		Listen:    "127.0.0.1:8787",
		APIKey:    "",
		ZenKeys:   []string{},
		Upstream:  "https://opencode.ai/zen",
		DataDir:   "data",
		AdminPass: "",
		Timeout: TimeoutConfig{
			RequestSeconds: 300,
			RefreshSeconds: 300,
		},
		ModelRules: ModelRules{
			AllowPaid:      true,
			ModelBlacklist: []string{},
		},
	}
}

// Load reads the config file (if present) and applies environment overrides.
func Load(path string) (*Config, error) {
	cfg := Defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// Missing config is fine: run on defaults + environment overrides.
		case err != nil:
			return nil, fmt.Errorf("read config %s: %w", path, err)
		default:
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config %s: %w", path, err)
			}
		}
	}
	// Environment overrides. Real environment variables (docker run -e,
	// CI secrets, systemd Environment=) win over the config file; keys that
	// are not set leave the file value untouched.
	if v := os.Getenv("OPENCODE_GATEWAY_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("OPENCODE_GATEWAY_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("OPENCODE_GATEWAY_ADMIN_PASSWORD"); v != "" {
		cfg.AdminPass = v
	}
	if v := os.Getenv("OPENCODE_GATEWAY_UPSTREAM"); v != "" {
		cfg.Upstream = v
	}
	if v := os.Getenv("OPENCODE_GATEWAY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("OPENCODE_ZEN_KEYS"); v != "" {
		keys := []string{}
		for _, k := range splitComma(v) {
			if k != "" {
				keys = append(keys, k)
			}
		}
		cfg.ZenKeys = keys
	}
	if cfg.Listen == "" {
		cfg.Listen = "127.0.0.1:8787"
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "https://opencode.ai/zen"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	return cfg, nil
}

// Save writes the configuration back to the JSON file, creating parent
// directories as needed.
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o600)
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' || r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// DefaultConfigPath returns the default config location: config.json in the
// current working directory, so the file sits next to the project and is easy
// to edit by hand. Use -config to point elsewhere.
func DefaultConfigPath() string {
	return "config.json"
}

// Bootstrap writes a fresh config with generated secrets. The generated
// credentials are not printed here: the caller prints them with the rest of
// the startup banner, on this and every later run.
func Bootstrap(path, listen string) error {
	cfg := Defaults()
	if listen != "" {
		cfg.Listen = listen
	}
	cfg.AdminPass = randomSecret(12)
	cfg.APIKey = "sk-gw-" + randomSecret(18)
	return Save(path, cfg)
}

func randomSecret(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			out[i] = alphabet[i%len(alphabet)]
			continue
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out)
}
