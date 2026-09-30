// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// opencode-gateway proxies OpenCode Zen as an OpenAI / Anthropic / Codex
// compatible API.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"opencode-gateway/config"
	"opencode-gateway/internal/catalog"
	"opencode-gateway/internal/server"
	"opencode-gateway/internal/telemetry"
	"opencode-gateway/internal/tray"
	"opencode-gateway/internal/upstream"
	"opencode-gateway/internal/usage"
)

func main() {
	var configPath string
	var listen string
	var noTray bool
	var hideWindow bool
	flag.StringVar(&configPath, "config", config.DefaultConfigPath(), "path to config.json")
	flag.StringVar(&listen, "listen", "", "override listen address")
	flag.BoolVar(&noTray, "no-tray", false, "不使用任务栏托盘图标（仅 Windows 有意义）")
	flag.BoolVar(&hideWindow, "hide-window", false, "启动后隐藏控制台窗口，只保留托盘图标（仅 Windows 有意义）")
	flag.Parse()

	// First run: bootstrap a config with generated secrets so the admin
	// console and remote access work out of the box.
	if configPath != "" && !fileExists(configPath) && os.Getenv("OPENCODE_GATEWAY_NO_INIT") == "" {
		if err := config.Bootstrap(configPath, listen); err != nil {
			log.Printf("warning: could not create %s: %v", configPath, err)
		} else {
			log.Printf("created default config at %s", configPath)
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fatalf("无法读取配置文件 %s：%v", configPath, err)
	}
	if abs, err := filepath.Abs(configPath); err == nil {
		log.Printf("config: %s", abs)
	}
	if listen != "" {
		cfg.Listen = listen
	}
	if cfg.AdminPass == "" {
		log.Printf("warning: admin password not set — /admin disabled (set admin_password in config)")
	}

	// Log files: daily JSONL under ./logs, keeping 7 days. The standard log
	// package is redirected here too, so every line (requests, LLM calls,
	// application events) lands in the same place. Logs sit next to
	// config.json rather than inside data_dir: data_dir holds the usage
	// database (machine state), while logs are meant to be tailed and
	// grepped by hand from the project root.
	logDir := "logs"
	if abs, err := filepath.Abs(logDir); err == nil {
		logDir = abs // report a path that is unambiguous in the banner
	}
	logs := telemetry.New(logDir, 7)
	defer logs.Close()
	log.SetOutput(logs)
	log.Printf("gateway starting: listen=%s upstream=%s logs=%s", cfg.Listen, cfg.Upstream, logs.Dir())

	// Fail fast, and legibly, when the port is already taken. Doing this
	// before anything else starts keeps the exit clean: no half-open
	// database, no tray icon that appears and immediately disappears.
	if hint := occupied(cfg.Listen); hint != "" {
		fatalf("无法启动：\n  %s", hint)
	}

	timeout := time.Duration(cfg.Timeout.RequestSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	zen := upstream.NewClient(cfg.Upstream, cfg.ZenKeys, timeout)
	cat := catalog.New(cfg.Upstream, func(ctx context.Context, url string) ([]byte, error) {
		return zen.FetchModelsJSON(ctx)
	})
	store, err := usage.Open(cfg.DataDir)
	if err != nil {
		fatalf("无法打开用量数据库（目录 %s）：%v", cfg.DataDir, err)
	}
	defer store.Close()

	srv := server.New(cfg, configPath, zen, cat, store, logs)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.StartRefreshLoop(ctx)

	// Tray icon: on Windows the gateway lives in the notification area so it
	// can keep serving with its console minimized (or hidden with
	// -hide-window). Everywhere else this is a no-op.
	trayEnabled := false
	if !noTray && tray.Available() {
		if err := tray.Start(tray.Options{
			ConsoleURL: consoleURL(cfg.Listen),
			Status:     trayStatus(store, cat),
			OnQuit:     stop,
			HideWindow: hideWindow,
		}); err != nil {
			log.Printf("warning: tray icon unavailable: %v", err)
		} else {
			trayEnabled = true
		}
	}

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	printBanner(cfg, configPath, logs.Dir(), trayEnabled)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatalf("网关监听 %s 失败：%v", cfg.Listen, err)
	}
}

// fatalf reports an unrecoverable start-up error and exits. A console
// process started by double-clicking loses its window the moment it exits,
// so printing to stderr alone would mean the reason disappears before it can
// be read — hence the pause, which gives the operator a chance to see why.
func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	fmt.Fprintf(os.Stderr, "\n✗ %s\n", msg)
	pause()
	os.Exit(1)
}

// pause waits for Enter so an error stays readable on screen. Without a
// console attached the read returns immediately, and the timeout keeps a
// headless launch (scheduled task, service wrapper) from hanging forever.
func pause() {
	fmt.Fprint(os.Stderr, "\n按回车键关闭窗口…")
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
	}
}

// occupied returns a human explanation when listenAddr cannot be bound, or
// "" when it is free. The usual cause by far is a second instance already
// owning the port, and that is worth saying plainly instead of letting a
// bare "bind: address already in use" flash past.
func occupied(listenAddr string) string {
	if ln, err := net.Listen("tcp", listenAddr); err == nil {
		_ = ln.Close()
		return ""
	}
	alt := alternativePort(listenAddr)
	// /healthz needs no authentication, so answering it proves another
	// gateway — rather than some unrelated program — owns the port.
	client := &http.Client{Timeout: 2 * time.Second}
	if resp, err := client.Get("http://" + listenAddr + "/healthz"); err == nil {
		_ = resp.Body.Close()
		return fmt.Sprintf(
			"%s 上已经有一个 opencode-gateway 在跑（它可能已经缩进右下角托盘了）。\n"+
				"  · 直接用它就行：%s\n"+
				"  · 想用这个新版本：先退出那一个，或换个端口启动\n"+
				"      opencode-gateway -listen 127.0.0.1:%s",
			listenAddr, consoleURL(listenAddr), alt)
	}
	return fmt.Sprintf(
		"端口 %s 被其他程序占用了。\n"+
			"  · 换一个端口：opencode-gateway -listen 127.0.0.1:%s\n"+
			"  · 或找出占用者：netstat -ano | findstr :%s",
		listenAddr, alt, portOf(listenAddr))
}

// alternativePort suggests the next port past the busy one; portOf extracts
// just the number for hints that address netstat users.
func alternativePort(listenAddr string) string {
	return strconv.Itoa(portNumber(listenAddr) + 1)
}

func portOf(listenAddr string) string {
	return strconv.Itoa(portNumber(listenAddr))
}

func portNumber(listenAddr string) int {
	if _, port, err := net.SplitHostPort(listenAddr); err == nil {
		if n, err := strconv.Atoi(port); err == nil {
			return n
		}
	}
	return 0
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// trayStatus builds the snapshot behind the tray icon: today's request count
// plus whether the upstream model directory loaded. The day boundary follows
// the host's own timezone here — unlike /api/overview there is no browser
// around to tell us which calendar the operator is looking at.
func trayStatus(store *usage.Store, cat *catalog.Catalog) func() tray.Status {
	return func() tray.Status {
		var st tray.Status
		_, offsetSec := time.Now().Zone()
		if agg, err := store.ModelAggregates(offsetSec / 60); err == nil {
			for _, u := range agg {
				st.RequestsToday += u.TodayRequests
			}
		}
		st.Loading = cat.FetchedAt().IsZero()
		st.UpstreamOK = len(cat.List()) > 0
		return st
	}
}

// printBanner reports the addresses and credentials the operator needs to
// reach the gateway. It is printed on every start (not just the first one):
// the console password and API key live in the config file, and having to
// open that file just to find them again on each restart is needless.
func printBanner(cfg *config.Config, configPath, logDir string, trayEnabled bool) {
	fmt.Printf("opencode-gateway 已启动\n")
	fmt.Printf("  监听地址   : http://%s\n", cfg.Listen)
	fmt.Printf("  Web 控制台 : %s\n", consoleURL(cfg.Listen))
	if cfg.AdminPass != "" {
		fmt.Printf("  控制台密码 : %s\n", cfg.AdminPass)
	} else {
		fmt.Printf("  控制台密码 : （未设置，/admin 不可用；请在 config.json 设置 admin_password）\n")
	}
	if cfg.APIKey != "" {
		fmt.Printf("  网关 APIKey: %s\n", cfg.APIKey)
	} else {
		fmt.Printf("  网关 APIKey: （未设置，仅本机 127.0.0.1 可访问）\n")
	}
	fmt.Printf("  上游       : %s\n", cfg.Upstream)
	fmt.Printf("  配置文件   : %s\n", configPath)
	fmt.Printf("  数据目录   : %s（用量库 usage.db）\n", cfg.DataDir)
	fmt.Printf("  日志目录   : %s（JSONL，保留 7 天）\n", logDir)
	if trayEnabled {
		fmt.Printf("  托盘图标   : 已启用（最小化窗口即最小化到右下角，右键图标可打开控制台 / 退出）\n")
	} else {
		fmt.Printf("  托盘图标   : 未启用（可用 -hide-window 让它只在托盘运行）\n")
	}
	fmt.Printf("  接口       : /v1/models · /v1/chat/completions · /v1/messages · /v1/responses\n")
}

// consoleURL turns a listen address into a URL a browser can open. Wildcard
// hosts are rewritten to loopback, since 0.0.0.0 is not navigable.
func consoleURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://" + listen + "/admin"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/admin"
}
