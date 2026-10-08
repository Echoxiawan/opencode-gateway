// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

//go:build !windows

package tray

import "errors"

// Non-Windows builds have no notification area to put an icon in, so every
// entry point is a stub. Start already refuses before reaching run(); the
// definitions exist so the platform-independent code in tray.go compiles.
func available() bool { return false }

func run(opts Options) error {
	return errors.New("tray: not supported on this platform")
}

func hideConsoleWindow() {}
