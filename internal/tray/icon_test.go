// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

//go:build windows

package tray

import "testing"

// TestMakeIcon draws a real icon. This is the exact code path that once
// panicked the whole gateway (a GDI call attributed to the wrong DLL), so it
// is worth exercising rather than trusting: it proves the bitmap, the AND
// mask and CreateIconIndirect all succeed on a real desktop.
func TestMakeIcon(t *testing.T) {
	for name, color := range map[string]uint32{
		"ok":      colorOK,
		"bad":     colorBad,
		"loading": colorLoading,
	} {
		h := makeIcon(color)
		if h == 0 {
			t.Errorf("makeIcon(%s) returned no icon handle", name)
			continue
		}
		// DestroyIcon answers BOOL: 0 means failure. The error coming back
		// from Call is just GetLastError, which is set (to "success") even
		// when everything worked, so the return value is what decides.
		if ok, _, _ := call(pDestroyIcon, uintptr(h)); ok == 0 {
			t.Errorf("destroy icon (%s) failed", name)
		}
	}
}
