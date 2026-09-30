// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// Package tray hosts the notification-area ("system tray") icon that lets a
// Windows gateway keep running after its console window is minimized or
// hidden: the icon sits in the taskbar's bottom-right corner and is the only
// thing left on screen.
//
// Only Windows ships a real implementation (see tray_windows.go). Every other
// platform compiles the stub in tray_other.go, so Start becomes a no-op there
// and nothing about launching the gateway on Linux or macOS changes.
package tray

import (
	"errors"
	"runtime"
)

// Status is the snapshot the tray icon shows. It is small on purpose: the
// icon has room for one line of tooltip and a colour, and everything else
// lives in the /admin console the icon opens.
type Status struct {
	// RequestsToday is the number of requests recorded on the operator's
	// calendar day.
	RequestsToday int64
	// UpstreamOK reports whether the model directory was fetched; it is the
	// same signal the console's status dot uses.
	UpstreamOK bool
	// Loading is true until the first upstream fetch has answered, so a
	// freshly started gateway is not painted as broken.
	Loading bool
}

// Options configures the tray icon.
type Options struct {
	// ConsoleURL is opened by the "打开控制台" menu item and by clicking the
	// icon (e.g. http://127.0.0.1:8787/admin).
	ConsoleURL string
	// Status is polled periodically for the tooltip and icon state. Nil is
	// allowed and means "no numbers available".
	Status func() Status
	// OnQuit runs when the operator picks the quit menu item. It is called
	// on its own goroutine so it never blocks the icon's message loop.
	OnQuit func()
	// HideWindow hides the console window as soon as the icon is installed,
	// i.e. start already minimized to the tray.
	HideWindow bool
}

// Available reports whether this build can show a tray icon.
func Available() bool { return available() }

// Start installs the tray icon and returns immediately. The icon lives on
// its own goroutine; it stops when OnQuit runs, or when the process exits.
func Start(opts Options) error {
	if opts.Status == nil {
		opts.Status = func() Status { return Status{} }
	}
	if opts.OnQuit == nil {
		opts.OnQuit = func() {}
	}
	if !available() {
		return errors.New("tray: not available on " + runtime.GOOS)
	}
	if opts.HideWindow {
		hideConsoleWindow()
	}
	return run(opts)
}
