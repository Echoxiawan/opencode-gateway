// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package webui embeds the admin console assets.
package webui

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed app.js
var AppJS []byte

//go:embed styles.css
var StylesCSS []byte
