// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package catalog maintains the model directory: which models exist, which
// are free, and which native protocol each paid model uses upstream.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Protocol is the upstream native wire protocol for a model.
type Protocol string

const (
	ProtocolChat      Protocol = "chat"
	ProtocolResponses Protocol = "responses"
	ProtocolAnthropic Protocol = "anthropic"
)

// Model describes one upstream model.
type Model struct {
	ID       string   `json:"id"`
	Free     bool     `json:"free"`
	Protocol Protocol `json:"protocol"` // native protocol (paid direct forwarding)
	// Cost is per-million-token pricing when known (USD).
	CostInput  float64 `json:"cost_input_per_m"`
	CostOutput float64 `json:"cost_output_per_m"`
}

// Catalog is the refreshable model directory.
type Catalog struct {
	mu        sync.RWMutex
	models    map[string]*Model
	fetchedAt time.Time
	upstream  string
	httpGet   func(ctx context.Context, url string) ([]byte, error)
}

// New creates a catalog that fetches from the given zen base URL.
func New(upstream string, httpGet func(ctx context.Context, url string) ([]byte, error)) *Catalog {
	return &Catalog{
		models:   map[string]*Model{},
		upstream: strings.TrimRight(upstream, "/"),
		httpGet:  httpGet,
	}
}

// zenModel is the /v1/models wire shape.
type zenModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
}

type zenModelsResp struct {
	Object string     `json:"object"`
	Data   []zenModel `json:"data"`
}

// Refresh fetches the live model list and rebuilds the catalog.
func (c *Catalog) Refresh(ctx context.Context) error {
	body, err := c.httpGet(ctx, c.upstream+"/v1/models")
	if err != nil {
		return fmt.Errorf("fetch models: %w", err)
	}
	var resp zenModelsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse models: %w", err)
	}
	if len(resp.Data) == 0 {
		return fmt.Errorf("models endpoint returned an empty list")
	}

	models := map[string]*Model{}
	for _, m := range resp.Data {
		if m.ID == "" {
			continue
		}
		models[m.ID] = &Model{
			ID:       m.ID,
			Free:     isFreeModel(m.ID),
			Protocol: NativeProtocol(m.ID),
		}
	}
	// Enrich cost + free status from models.dev when available.
	c.enrichFromModelsDev(ctx, models)

	c.mu.Lock()
	c.models = models
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}

// FetchedAt reports the last successful refresh time.
func (c *Catalog) FetchedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fetchedAt
}

// Get returns a model by ID.
func (c *Catalog) Get(id string) (*Model, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.models[id]
	return m, ok
}

// List returns all models.
// List returns every model, sorted by ID. The catalog is stored as a map, so
// without an explicit sort every caller would see a different order on each
// call — which shuffled the console's model grid on every refresh and made
// /v1/models answers unstable.
func (c *Catalog) List() []*Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*Model, 0, len(c.models))
	for _, m := range c.models {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// isFreeModel decides free eligibility: "-free" suffix (the convention used
// by the zen free tier) — costs are also cross-checked with models.dev.
func isFreeModel(id string) bool {
	return strings.Contains(strings.ToLower(id), "free")
}

// NativeProtocol maps a model ID to its native zen endpoint protocol,
// following the official zen.mdx table.
func NativeProtocol(id string) Protocol {
	switch {
	case strings.HasPrefix(id, "gpt-"), strings.HasPrefix(id, "o1"), strings.HasPrefix(id, "o3"),
		strings.HasPrefix(id, "codex"):
		// Most GPT-family and codex models are served via Responses.
		// (jev uses systemone upstream, but we route it through chat.)
		return ProtocolChat
	case strings.HasPrefix(id, "claude-"):
		return ProtocolAnthropic
	default:
		return ProtocolChat
	}
}

// modelsDevProvider mirrors the subset of models.dev/api.json we need.
var modelsDevCostRegex = regexp.MustCompile(`^\d+(\.\d+)?$`)

func (c *Catalog) enrichFromModelsDev(ctx context.Context, models map[string]*Model) {
	if c.httpGet == nil {
		return
	}
	body, err := c.httpGet(ctx, "https://models.dev/api.json")
	if err != nil {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return
	}
	var provider struct {
		Models map[string]struct {
			Cost struct {
				Input  json.Number `json:"input"`
				Output json.Number `json:"output"`
			} `json:"cost"`
			Status string `json:"status"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw["opencode"], &provider); err != nil {
		return
	}
	for id, pm := range provider.Models {
		m, ok := models[id]
		if !ok {
			continue
		}
		in, err1 := pm.Cost.Input.Float64()
		out, err2 := pm.Cost.Output.Float64()
		if err1 == nil && err2 == nil && modelsDevCostRegex.MatchString(pm.Cost.Input.String()) {
			m.CostInput = in
			m.CostOutput = out
		}
		// models.dev zero-cost models that are not deprecated also qualify.
		if pm.Status == "" || pm.Status == "active" {
			if m.CostInput == 0 && m.CostOutput == 0 {
				m.Free = true
			}
		}
	}
}

// createdUnix returns a stable created timestamp for the models list.
func (m *Model) createdUnix() int64 {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
}
