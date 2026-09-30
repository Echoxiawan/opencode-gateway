// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

//go:build windows

package tray

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// Win32 constants. Kept local rather than pulled from a helper module so the
// package stays dependency-free: every other platform stubs this file out,
// and paying for a dependency (or a cgo requirement on Linux) for one file
// nobody else uses is not worth it.
const (
	// window messages
	wmNull       = 0x0000
	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmCommand    = 0x0111
	wmUser       = 0x0400
	wmLButtonUp  = 0x0202
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205

	// trayCallback is the private message the shell posts to our hidden
	// window for clicks on the icon.
	trayCallback = wmUser + 1

	// Shell_NotifyIcon operations and NOTIFYICONDATA members
	nimAdd     = 0x00000000
	nimModify  = 0x00000001
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	// menu flags
	mfString    = 0x00000000
	mfGrayed    = 0x00000001
	mfSeparator = 0x00000800

	// TrackPopupMenu positioning
	tpmRightAlign  = 0x0008
	tpmBottomAlign = 0x0020
	tpmRightButton = 0x0002

	// menu item ids
	menuOpenConsole  = 1001
	menuToggleWindow = 1002
	menuQuit         = 1003

	// ShowWindow commands
	swHide    = 0
	swShow    = 5
	swRestore = 9

	nullPen = 8 // GetStockObject stock index

	idiApplication = 32512 // fallback icon when drawing fails

	iconSize = 32

	// Colours packed as COLORREF (0x00BBGGRR), matching the console theme:
	// green = healthy upstream, red = unreachable, grey = still loading, and
	// the same dark background behind them.
	colorOK      uint32 = 0x8ec934 // #34c98e
	colorBad     uint32 = 0x4b53e5 // #e5534b
	colorLoading uint32 = 0xa3938b // #8b93a3
	colorBack    uint32 = 0x15110f // #0f1115
)

// Struct layouts mirror their Win32 counterparts. No explicit padding fields:
// Go already aligns Handle/uintptr members exactly the way the ABI does on
// both amd64 and 386, so the same source produces the right offsets for each.
type notifyIconData struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32 // union with uVersion; unused here
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         guid
	HBalloonIcon     syscall.Handle
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  uintptr
	LpszClassName uintptr
	HIconSm       syscall.Handle
}

type point struct {
	X int32
	Y int32
}

type msgTag struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  syscall.Handle
	HbmColor syscall.Handle
}

// The packages behind every API used below are always present on Windows, so
// LazyDLL's deferred lookup is only about avoiding work for builds that
// never touch the tray (e.g. -no-tray runs).
var (
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")

	pGetModuleHandleW = modKernel32.NewProc("GetModuleHandleW")
	pGetConsoleWindow = modKernel32.NewProc("GetConsoleWindow")
	pSetConsoleTitleW = modKernel32.NewProc("SetConsoleTitleW")

	pRegisterClassExW    = modUser32.NewProc("RegisterClassExW")
	pCreateWindowExW     = modUser32.NewProc("CreateWindowExW")
	pDefWindowProcW      = modUser32.NewProc("DefWindowProcW")
	pGetMessageW         = modUser32.NewProc("GetMessageW")
	pTranslateMessage    = modUser32.NewProc("TranslateMessage")
	pDispatchMessageW    = modUser32.NewProc("DispatchMessageW")
	pPostMessageW        = modUser32.NewProc("PostMessageW")
	pPostQuitMessage     = modUser32.NewProc("PostQuitMessage")
	pSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	pCreatePopupMenu     = modUser32.NewProc("CreatePopupMenu")
	pAppendMenuW         = modUser32.NewProc("AppendMenuW")
	pTrackPopupMenu      = modUser32.NewProc("TrackPopupMenu")
	pDestroyMenu         = modUser32.NewProc("DestroyMenu")
	pGetCursorPos        = modUser32.NewProc("GetCursorPos")
	pShowWindow          = modUser32.NewProc("ShowWindow")
	pIsIconic            = modUser32.NewProc("IsIconic")
	pIsWindowVisible     = modUser32.NewProc("IsWindowVisible")
	pEnumWindows         = modUser32.NewProc("EnumWindows")
	pGetWindowTextW      = modUser32.NewProc("GetWindowTextW")
	pLoadIconW           = modUser32.NewProc("LoadIconW")
	pDestroyIcon         = modUser32.NewProc("DestroyIcon")
	pFillRect            = modUser32.NewProc("FillRect")
	pCreateIconIndirect  = modUser32.NewProc("CreateIconIndirect")
	pGetDC               = modUser32.NewProc("GetDC")
	pReleaseDC           = modUser32.NewProc("ReleaseDC")

	pShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW    = modShell32.NewProc("ShellExecuteW")

	pCreateCompatibleDC     = modGdi32.NewProc("CreateCompatibleDC")
	pDeleteDC               = modGdi32.NewProc("DeleteDC")
	pCreateCompatibleBitmap = modGdi32.NewProc("CreateCompatibleBitmap")
	pCreateBitmap           = modGdi32.NewProc("CreateBitmap")
	pSelectObject           = modGdi32.NewProc("SelectObject")
	pCreateSolidBrush       = modGdi32.NewProc("CreateSolidBrush")
	pGetStockObject         = modGdi32.NewProc("GetStockObject")
	pEllipse                = modGdi32.NewProc("Ellipse")
	pDeleteObject           = modGdi32.NewProc("DeleteObject")
)

// Package-level UTF-16 buffers: keeping the slice alive keeps its backing
// array alive, so pointers handed to Win32 stay valid for the process
// lifetime. Taking pointers at call time avoids unsafe frozenGotchas with
// escaping unsafe.Pointer values.
var (
	className  = syscall.StringToUTF16("opencode-gateway-tray")
	windowName = syscall.StringToUTF16("opencode-gateway")
	wndProcCB  = syscall.NewCallback(wndProc)
)

var (
	classOnce sync.Once
	classAtom uintptr

	// current is the single tray instance. One gateway process only ever
	// installs one icon, and the window procedure needs to reach it without
	// threading a pointer through the C callback.
	current *shell
)

// shell owns the icon's Win32 handles and the state shown in the tooltip.
type shell struct {
	opts Options

	mu        sync.Mutex
	stopped   bool
	hWnd      syscall.Handle
	hIcon     syscall.Handle
	nid       notifyIconData
	status    Status
	lastState int
	lastTip   string

	// sawVisible is read and written only by the poll goroutine.
	sawVisible bool

	// winMu serializes every read-then-act transition on the console
	// windows. hideIfMinimized runs on the poll goroutine, toggleConsole on
	// the message pump (menu command); without the lock the poll can observe
	// "iconic" just before toggleConsole restores the window, then hide the
	// freshly restored window a heartbeat later — from the user's seat,
	// "show console only works every other click".
	winMu sync.Mutex

	// titleToken tags the console with a PID-unique title so the poll can
	// recognise the window that actually hosts the console. Set in loop
	// before the poll goroutine starts, read-only afterwards.
	titleToken string
}

func available() bool { return true }

// call invokes a Win32 procedure, turning a missing or misplaced one into a
// plain failure instead of a panic. Every procedure here is looked up lazily,
// so a name that lives in another DLL (FillRect belongs to user32, not gdi32)
// would otherwise surface as a panic at runtime — in this goroutine, which
// means taking the whole gateway down with it. Losing the icon is acceptable;
// losing the proxy is not.
//
// The returned error is the raw GetLastError, which Windows leaves set even
// on success; it is only meaningful when the return value itself says the
// call failed (most of these APIs answer a BOOL or a zero handle on error).
func call(p *syscall.LazyProc, args ...uintptr) (uintptr, uintptr, error) {
	if err := p.Find(); err != nil {
		return 0, 0, err
	}
	r1, r2, err := p.Call(args...)
	return r1, r2, err
}

func run(opts Options) error {
	if current != nil {
		return fmt.Errorf("tray: already started")
	}
	current = &shell{opts: opts}
	// Last line of defence: whatever the icon does on this thread, it must
	// never be fatal to the gateway. A panic here would otherwise kill the
	// whole process — which from an Explorer double-click looks exactly like
	// "the window flashed and nothing happened".
	go func() {
		defer func() {
			if r := recover(); r != nil {
				current.markStopped()
				fmt.Fprintf(os.Stderr, "\n托盘图标已停用（网关继续运行）：%v\n", r)
			}
		}()
		current.loop()
	}()
	return nil
}

// loop runs the icon's message pump. It owns an OS thread for its lifetime:
// the window it creates must be pumped by the same thread that made it, and
// the window procedure runs on that thread.
func (s *shell) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := call(pGetModuleHandleW, 0)
	if hInstance == 0 {
		return
	}
	hwnd := s.createWindow(hInstance)
	if hwnd == 0 {
		return
	}

	st := s.snapshot()
	s.mu.Lock()
	s.hWnd = hwnd
	ok := s.addIconLocked(st)
	s.mu.Unlock()
	if !ok {
		call(pDestroyIcon, uintptr(s.hIcon))
		return
	}

	// Two background helpers: one refreshes the numbers, the other turns a
	// click on the window's minimize button into "hide to tray".
	s.titleToken = fmt.Sprintf("opencode-gateway #%d", os.Getpid())
	tok := syscall.StringToUTF16(s.titleToken)
	call(pSetConsoleTitleW, uintptr(unsafe.Pointer(&tok[0])))
	go s.poll()

	var m msgTag
	for {
		ret, _, _ := call(pGetMessageW, uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 { // WM_QUIT
			break
		}
		if int32(uint32(ret)) == -1 { // GetMessage failed; stop churning
			break
		}
		call(pTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(pDispatchMessageW, uintptr(unsafe.Pointer(&m)))
	}

	s.removeIcon()
}

// createWindow makes the message-only window every tray icon needs: the shell
// posts its click notifications to it, so there is always exactly one owner
// for the finished messages to be processed by a single defined owner.
func (s *shell) createWindow(hInstance uintptr) syscall.Handle {
	classOnce.Do(func() {
		wcx := wndClassEx{
			Style:         0,
			LpfnWndProc:   wndProcCB,
			HInstance:     syscall.Handle(hInstance),
			LpszClassName: uintptr(unsafe.Pointer(&className[0])),
		}
		wcx.CbSize = uint32(unsafe.Sizeof(wcx))
		r, _, _ := call(pRegisterClassExW, uintptr(unsafe.Pointer(&wcx)))
		classAtom = r
	})
	if classAtom == 0 {
		return 0
	}
	// HWND_MESSAGE (-3) as the parent makes it a message-only window: it
	// never appears on screen but still receives posted messages.
	const hwndMessage = ^uintptr(2)
	hwnd, _, _ := call(pCreateWindowExW,
		0,
		uintptr(unsafe.Pointer(&className[0])),
		uintptr(unsafe.Pointer(&windowName[0])),
		0,
		0, 0, 0, 0,
		hwndMessage,
		0,
		hInstance,
		0,
	)
	return syscall.Handle(hwnd)
}

// addIconLocked puts the icon in the notification area. Caller holds s.mu.
func (s *shell) addIconLocked(st Status) bool {
	state := iconState(st)
	hIcon := makeIcon(iconColor(state))
	if hIcon == 0 {
		// Drawing failed (no screen DC?); fall back to the generic
		// application icon so the tray entry is never invisible.
		h, _, _ := call(pLoadIconW, 0, uintptr(idiApplication))
		hIcon = syscall.Handle(h)
		if hIcon == 0 {
			return false
		}
	}
	s.hIcon = hIcon
	s.lastState = state
	s.status = st

	nid := notifyIconData{}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = s.hWnd
	nid.UID = 1
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = trayCallback
	nid.HIcon = hIcon
	s.nid = nid
	tip := tooltip(st, s.opts.ConsoleURL)
	setTip(&s.nid, tip)
	s.lastTip = tip

	r, _, _ := call(pShellNotifyIconW, nimAdd, uintptr(unsafe.Pointer(&s.nid)))
	return r != 0
}

// removeIcon takes the icon out of the notification area and frees it.
func (s *shell) removeIcon() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.hWnd != 0 {
		call(pShellNotifyIconW, nimDelete, uintptr(unsafe.Pointer(&s.nid)))
	}
	if s.hIcon != 0 {
		call(pDestroyIcon, uintptr(s.hIcon))
		s.hIcon = 0
	}
}

// poll refreshes the tooltip every ~10s, re-colours the icon when the
// upstream state changes, and checks for a minimized console every 300ms.
func (s *shell) poll() {
	// The short tick is what makes minimize-to-tray feel instant: the check
	// has to land between the user clicking minimize and them noticing the
	// taskbar button is still there. A 10s poll read as "it doesn't work".
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	s.refresh()
	s.hideIfMinimized()
	for tick := 1; ; tick++ {
		<-ticker.C
		if s.stoppedNow() {
			return
		}
		if tick%33 == 0 { // ~10s: enough for the numbers and status colour
			s.refresh()
		}
		s.hideIfMinimized()
	}
}

func (s *shell) stoppedNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// markStopped records that the icon is gone so the polling goroutine stops
// touching its handles. It exists for the recover() path, where the icon's
// own teardown never ran.
func (s *shell) markStopped() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
}

func (s *shell) snapshot() Status {
	return s.opts.Status()
}

func (s *shell) refresh() {
	st := s.snapshot()
	tip := tooltip(st, s.opts.ConsoleURL)
	state := iconState(st)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.status = st
	if tip != s.lastTip {
		s.lastTip = tip
		setTip(&s.nid, tip)
		call(pShellNotifyIconW, nimModify, uintptr(unsafe.Pointer(&s.nid)))
	}
	if state != s.lastState {
		s.lastState = state
		if h := makeIcon(iconColor(state)); h != 0 {
			s.nid.HIcon = h
			call(pShellNotifyIconW, nimModify, uintptr(unsafe.Pointer(&s.nid)))
			if s.hIcon != 0 {
				call(pDestroyIcon, uintptr(s.hIcon))
			}
			s.hIcon = h
		}
	}
}

// enum state for consoleWindows' EnumWindows sweep. The callback is created
// once (syscall callbacks cannot be freed, so making one per poll tick would
// leak); the token it matches against is swapped under the mutex before each
// sweep. All access is from the single poll goroutine in practice, but the
// mutex keeps it honest.
var (
	enumMu    sync.Mutex
	enumToken string
	enumHits  []syscall.Handle
	enumCB    = syscall.NewCallback(enumMatch)
)

func enumMatch(hwnd syscall.Handle, _ uintptr) uintptr {
	buf := make([]uint16, 128)
	n, _, _ := call(pGetWindowTextW, uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n > 0 && strings.Contains(syscall.UTF16ToString(buf[:n]), enumToken) {
		enumHits = append(enumHits, hwnd)
	}
	return 1 // continue enumeration
}

// consoleWindows lists the top-level windows that host this process's
// console. GetConsoleWindow alone is not enough: when the session is hosted
// by Windows Terminal (the default on current Windows 11), it returns an
// invisible pseudo window that never reports IsIconic — the window the user
// actually minimizes belongs to WindowsTerminal.exe and is only recognisable
// by the PID-unique title set at startup.
func consoleWindows(token string) []syscall.Handle {
	var out []syscall.Handle
	if h := consoleWindow(); h != 0 {
		out = append(out, h)
	}
	if token != "" {
		enumMu.Lock()
		enumToken = token
		enumHits = enumHits[:0]
		call(pEnumWindows, enumCB, 0)
		// GetConsoleWindow's window carries the token title too, so the
		// sweep finds it again — keep each hwnd once.
		for _, h := range enumHits {
			dup := false
			for _, seen := range out {
				if seen == h {
					dup = true
					break
				}
			}
			if !dup {
				out = append(out, h)
			}
		}
		enumMu.Unlock()
	}
	return out
}

// hideIfMinimized implements "minimize to tray": as soon as a console window
// goes iconic, take it off the taskbar entirely and leave only the icon
// behind. Polling is deliberate — the console window belongs to conhost or
// Windows Terminal, not to this process, so it cannot be subclassed.
func (s *shell) hideIfMinimized() {
	s.winMu.Lock()
	defer s.winMu.Unlock()
	for _, hwnd := range consoleWindows(s.titleToken) {
		iconic, _, _ := call(pIsIconic, uintptr(hwnd))
		if iconic == 0 {
			if visible, _, _ := call(pIsWindowVisible, uintptr(hwnd)); visible != 0 {
				s.sawVisible = true
			}
			continue
		}
		// Only hide a window we have actually seen on screen. A process can
		// start with its console already iconic (launch quirks happen), and
		// hiding that takes the gateway off screen before anyone has seen it
		// — the opposite of what "minimize to tray" is for.
		if s.sawVisible {
			call(pShowWindow, uintptr(hwnd), swHide)
		}
	}
}

func (s *shell) toggleConsole() {
	s.winMu.Lock()
	defer s.winMu.Unlock()
	windows := consoleWindows(s.titleToken)
	if len(windows) == 0 {
		return
	}
	anyVisible := false
	for _, hwnd := range windows {
		if visible, _, _ := call(pIsWindowVisible, uintptr(hwnd)); visible != 0 {
			anyVisible = true
			break
		}
	}
	for _, hwnd := range windows {
		if anyVisible {
			call(pShowWindow, uintptr(hwnd), swHide)
			continue
		}
		if iconic, _, _ := call(pIsIconic, uintptr(hwnd)); iconic != 0 {
			call(pShowWindow, uintptr(hwnd), swRestore)
		} else {
			call(pShowWindow, uintptr(hwnd), swShow)
		}
		call(pSetForegroundWindow, uintptr(hwnd))
	}
}

func (s *shell) openConsole() {
	if s.opts.ConsoleURL == "" {
		return
	}
	verb := syscall.StringToUTF16("open")
	url := syscall.StringToUTF16(s.opts.ConsoleURL)
	call(pShellExecuteW, 0,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&url[0])),
		0, 0, swShow)
}

func (s *shell) quit() {
	s.removeIcon()
	// Give the gateway its graceful shutdown; the process leaves once main
	// returns, and the icon is already gone by then.
	go s.opts.OnQuit()
	call(pPostQuitMessage, 0)
}

func (s *shell) popupMenu(hwnd syscall.Handle) {
	hMenu, _, _ := call(pCreatePopupMenu)
	if hMenu == 0 {
		return
	}
	defer call(pDestroyMenu, hMenu)

	s.mu.Lock()
	st := s.status
	s.mu.Unlock()

	appendItem(hMenu, mfString, menuOpenConsole, "打开控制台…")
	appendItem(hMenu, mfGrayed, 0, fmt.Sprintf("今日请求 %d · 上游 %s", st.RequestsToday, stateLabel(st)))
	appendItem(hMenu, mfSeparator, 0, "")
	appendItem(hMenu, mfString, menuToggleWindow, "显示 / 隐藏控制台窗口")
	appendItem(hMenu, mfString, menuQuit, "退出")

	var pt point
	call(pGetCursorPos, uintptr(unsafe.Pointer(&pt)))
	// TrackPopupMenu only dismisses reliably when its owner is foregrounded
	// first, and it leaves a NULL message behind to clear that state.
	call(pSetForegroundWindow, uintptr(hwnd))
	call(pTrackPopupMenu, hMenu,
		uintptr(tpmRightAlign|tpmBottomAlign|tpmRightButton),
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(hwnd), 0)
	call(pPostMessageW, uintptr(hwnd), wmNull, 0, 0)
}

func appendItem(hMenu uintptr, flags uint32, id uintptr, text string) {
	if int32(flags)&mfSeparator != 0 {
		call(pAppendMenuW, hMenu, uintptr(mfSeparator), 0, 0)
		return
	}
	u := syscall.StringToUTF16(text)
	call(pAppendMenuW, hMenu, uintptr(flags), id, uintptr(unsafe.Pointer(&u[0])))
}

// wndProc dispatches everything the shell sends to the hidden window.
func wndProc(hwnd syscall.Handle, message uint32, wparam, lparam uintptr) uintptr {
	s := current
	if s == nil {
		return 0
	}
	switch message {
	case trayCallback:
		switch lparam & 0xffff {
		case wmRButtonUp:
			s.popupMenu(hwnd)
			return 0
		case wmLButtonUp, wmLButtonDbl:
			s.openConsole()
			return 0
		}
	case wmCommand:
		switch int(wparam & 0xffff) {
		case menuOpenConsole:
			s.openConsole()
			return 0
		case menuToggleWindow:
			s.toggleConsole()
			return 0
		case menuQuit:
			s.quit()
			return 0
		}
	case wmClose, wmDestroy:
		return 0
	}
	r, _, _ := call(pDefWindowProcW, uintptr(hwnd), uintptr(message), wparam, lparam)
	return r
}

// ---- helpers ----

func iconState(st Status) int {
	if st.Loading {
		return 3
	}
	if st.UpstreamOK {
		return 1
	}
	return 2
}

func iconColor(state int) uint32 {
	switch state {
	case 1:
		return colorOK
	case 2:
		return colorBad
	default:
		return colorLoading
	}
}

func stateLabel(st Status) string {
	switch iconState(st) {
	case 1:
		return "正常"
	case 2:
		return "异常"
	default:
		return "加载中"
	}
}

func tooltip(st Status, url string) string {
	head := "opencode-gateway"
	if url != "" {
		head += " · " + url
	}
	return fmt.Sprintf("%s\n今日请求 %d · 上游 %s", head, st.RequestsToday, stateLabel(st))
}

// setTip fills NOTIFYICONDATA.szTip. The array is 128 UTF-16 units including
// the terminator, so anything longer is clipped on a rune boundary.
func setTip(nid *notifyIconData, tip string) {
	runes := []rune(tip)
	if len(runes) > 127 {
		runes = runes[:127]
	}
	u := utf16.Encode(runes)
	for i := range nid.SzTip {
		nid.SzTip[i] = 0
	}
	copy(nid.SzTip[:], u)
}

func consoleWindow() syscall.Handle {
	h, _, _ := call(pGetConsoleWindow)
	return syscall.Handle(h)
}

func hideConsoleWindow() {
	if hwnd := consoleWindow(); hwnd != 0 {
		call(pShowWindow, uintptr(hwnd), swHide)
	}
}

// makeIcon draws the tray icon: a coloured dot on the console's dark
// background, so its colour alone tells you whether upstream is healthy. It
// is drawn rather than loaded from an .ico to avoid shipping a binary asset,
// and because the colour has to change at runtime.
func makeIcon(color uint32) syscall.Handle {
	hdcScreen, _, _ := call(pGetDC, 0)
	if hdcScreen == 0 {
		return 0
	}
	defer call(pReleaseDC, 0, hdcScreen)

	hdcMem, _, _ := call(pCreateCompatibleDC, hdcScreen)
	if hdcMem == 0 {
		return 0
	}
	defer call(pDeleteDC, hdcMem)

	bmp, _, _ := call(pCreateCompatibleBitmap, hdcScreen, iconSize, iconSize)
	if bmp == 0 {
		return 0
	}
	call(pSelectObject, hdcMem, bmp)

	// No outlines; remember enough to put the previous pen back.
	nullPenH, _, _ := call(pGetStockObject, nullPen)
	oldPen, _, _ := call(pSelectObject, hdcMem, nullPenH)

	if brush, _, _ := call(pCreateSolidBrush, uintptr(colorBack)); brush != 0 {
		all := rect{Right: iconSize, Bottom: iconSize}
		call(pFillRect, hdcMem, uintptr(unsafe.Pointer(&all)), brush)
		call(pDeleteObject, brush)
	}
	if brush, _, _ := call(pCreateSolidBrush, uintptr(color)); brush != 0 {
		oldBrush, _, _ := call(pSelectObject, hdcMem, brush)
		m := iconSize / 5
		call(pEllipse, hdcMem, uintptr(m), uintptr(m), uintptr(iconSize-m), uintptr(iconSize-m))
		call(pSelectObject, hdcMem, oldBrush)
		call(pDeleteObject, brush)
	}
	if oldPen != 0 {
		call(pSelectObject, hdcMem, oldPen)
	}

	// The AND mask is an all-zero monochrome bitmap: zero means "keep this
	// pixel", so every pixel of the colour bitmap shows through.
	rowBytes := iconSize / 8
	maskBits := make([]byte, rowBytes*iconSize)
	hMask, _, _ := call(pCreateBitmap, iconSize, iconSize, 1, 1,
		uintptr(unsafe.Pointer(&maskBits[0])))
	if hMask == 0 {
		return 0
	}

	info := iconInfo{
		FIcon:    1,
		HbmMask:  syscall.Handle(hMask),
		HbmColor: syscall.Handle(bmp),
	}
	hIcon, _, _ := call(pCreateIconIndirect, uintptr(unsafe.Pointer(&info)))
	// The two bitmaps are deliberately not freed: the icon owns their
	// contents from here on.
	return syscall.Handle(hIcon)
}
