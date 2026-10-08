// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

//go:build windows

package tray

import (
	"syscall"
	"testing"
)

// TestProcsResolve guards against the mistake this file is most prone to: a
// procedure attributed to the wrong DLL. LazyProc resolves on first call, so
// nothing fails at build time — the icon would only panic once the gateway is
// actually launched, taking the whole process with it (which from Explorer
// looks like a window that flashes and disappears). Listing them here turns
// that into a test failure.
func TestProcsResolve(t *testing.T) {
	procs := map[string]*syscall.LazyProc{
		"kernel32.GetModuleHandleW":    pGetModuleHandleW,
		"kernel32.GetConsoleWindow":    pGetConsoleWindow,
		"kernel32.SetConsoleTitleW":    pSetConsoleTitleW,
		"user32.RegisterClassExW":      pRegisterClassExW,
		"user32.CreateWindowExW":       pCreateWindowExW,
		"user32.DefWindowProcW":        pDefWindowProcW,
		"user32.GetMessageW":           pGetMessageW,
		"user32.TranslateMessage":      pTranslateMessage,
		"user32.DispatchMessageW":      pDispatchMessageW,
		"user32.PostMessageW":          pPostMessageW,
		"user32.PostQuitMessage":       pPostQuitMessage,
		"user32.SetForegroundWindow":   pSetForegroundWindow,
		"user32.CreatePopupMenu":       pCreatePopupMenu,
		"user32.AppendMenuW":           pAppendMenuW,
		"user32.TrackPopupMenu":        pTrackPopupMenu,
		"user32.DestroyMenu":           pDestroyMenu,
		"user32.GetCursorPos":          pGetCursorPos,
		"user32.ShowWindow":            pShowWindow,
		"user32.IsIconic":              pIsIconic,
		"user32.IsWindowVisible":       pIsWindowVisible,
		"user32.EnumWindows":           pEnumWindows,
		"user32.GetWindowTextW":        pGetWindowTextW,
		"user32.LoadIconW":             pLoadIconW,
		"user32.DestroyIcon":           pDestroyIcon,
		"user32.FillRect":              pFillRect,
		"user32.CreateIconIndirect":    pCreateIconIndirect,
		"user32.GetDC":                 pGetDC,
		"user32.ReleaseDC":             pReleaseDC,
		"shell32.Shell_NotifyIconW":    pShellNotifyIconW,
		"shell32.ShellExecuteW":        pShellExecuteW,
		"gdi32.CreateCompatibleDC":     pCreateCompatibleDC,
		"gdi32.DeleteDC":               pDeleteDC,
		"gdi32.CreateCompatibleBitmap": pCreateCompatibleBitmap,
		"gdi32.CreateBitmap":           pCreateBitmap,
		"gdi32.SelectObject":           pSelectObject,
		"gdi32.CreateSolidBrush":       pCreateSolidBrush,
		"gdi32.GetStockObject":         pGetStockObject,
		"gdi32.Ellipse":                pEllipse,
		"gdi32.DeleteObject":           pDeleteObject,
	}
	for name, proc := range procs {
		if proc == nil {
			t.Errorf("%s: proc missing from the DLL table", name)
			continue
		}
		if err := proc.Find(); err != nil {
			t.Errorf("%s unresolved: %v (wrong DLL?)", name, err)
		}
	}
}
