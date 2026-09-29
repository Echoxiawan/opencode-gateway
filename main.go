// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// opencode-gateway proxies OpenCode Zen as an OpenAI / Anthropic / Codex
// compatible API.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"opencode-gateway/config"
	"opencode-gateway/internal/catalog"
	"opencode-gateway/internal/server"
	"opencode-gateway/internal/telemetry"
	"opencode-gateway/internal/upstream"
	"opencode-gateway/internal/usage"
)

func main() {
	var configPath string
	var listen string
	flag.StringVar(&configPath, "config", config.DefaultConfigPath(), "path to config.json")
	flag.StringVar(&listen, "listen", "", "override listen address")
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
		log.Fatalf("load config: %v", err)
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
		log.Fatalf("open usage store: %v", err)
	}
	defer store.Close()

	srv := server.New(cfg, configPath, zen, cat, store, logs)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.StartRefreshLoop(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	printBanner(cfg, configPath, logs.Dir())

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// printBanner reports the addresses and credentials the operator needs to
// reach the gateway. It is printed on every start (not just the first one):
// the console password and API key live in the config file, and having to
// open that file just to find them again on each restart is needless.
func printBanner(cfg *config.Config, configPath, logDir string) {
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
