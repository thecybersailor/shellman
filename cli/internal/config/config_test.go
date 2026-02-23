package config

import (
	"testing"
	"time"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("SHELLMAN_WORKER_BASE_URL", "")
	t.Setenv("SHELLMAN_LOG_LEVEL", "")
	t.Setenv("SHELLMAN_TRACE_STREAM", "")
	t.Setenv("SHELLMAN_LOCAL_HOST", "")

	cfg := LoadConfig()
	if cfg.WorkerBaseURL != "http://127.0.0.1:8787" {
		t.Fatalf("unexpected WorkerBaseURL: %s", cfg.WorkerBaseURL)
	}
	if cfg.ListenLogLevel != "info" {
		t.Fatalf("unexpected ListenLogLevel: %s", cfg.ListenLogLevel)
	}
	if cfg.TraceStream {
		t.Fatal("trace stream should default to disabled")
	}
	if cfg.Mode != "local" {
		t.Fatalf("mode should default to local, got %s", cfg.Mode)
	}
	if cfg.TurnEnabled {
		t.Fatal("turn should default to disabled")
	}
	if cfg.LocalPort != 8000 {
		t.Fatalf("unexpected local port: %d", cfg.LocalPort)
	}
	if cfg.LocalHost != "127.0.0.1" {
		t.Fatalf("unexpected local host: %s", cfg.LocalHost)
	}
	if cfg.WebUIMode != "dev" {
		t.Fatalf("unexpected default web ui mode: %s", cfg.WebUIMode)
	}
	if cfg.WebUIDevProxyURL != "http://127.0.0.1:15173" {
		t.Fatalf("unexpected default web ui proxy: %s", cfg.WebUIDevProxyURL)
	}
	if cfg.WebUIDistDir != defaultWebUIDistDir() {
		t.Fatalf("unexpected default web ui dist: %s", cfg.WebUIDistDir)
	}
	if cfg.OpenAIEndpoint != "" || cfg.OpenAIModel != "" || cfg.OpenAIAPIKey != "" {
		t.Fatalf("openai env should default empty, got endpoint=%q model=%q key-set=%v", cfg.OpenAIEndpoint, cfg.OpenAIModel, cfg.OpenAIAPIKey != "")
	}
}

func TestLoadConfig_TurnEnabled(t *testing.T) {
	t.Setenv("SHELLMAN_TURN_ENABLED", "1")
	cfg := LoadConfig()
	if !cfg.TurnEnabled {
		t.Fatal("turn should be enabled when SHELLMAN_TURN_ENABLED=1")
	}
}

func TestLoadConfig_TraceStreamEnabled(t *testing.T) {
	t.Setenv("SHELLMAN_TRACE_STREAM", "1")
	cfg := LoadConfig()
	if !cfg.TraceStream {
		t.Fatal("trace stream should be enabled when SHELLMAN_TRACE_STREAM=1")
	}
}

func TestLoadConfig_ModeAndLocalPort(t *testing.T) {
	t.Setenv("SHELLMAN_MODE", "turn")
	t.Setenv("SHELLMAN_LOCAL_PORT", "4700")
	t.Setenv("SHELLMAN_LOCAL_HOST", "0.0.0.0")
	t.Setenv("SHELLMAN_WEBUI_MODE", "prod")
	t.Setenv("SHELLMAN_WEBUI_DEV_PROXY_URL", "http://127.0.0.1:25173")
	t.Setenv("SHELLMAN_WEBUI_DIST_DIR", "/tmp/webui-dist")
	t.Setenv("OPENAI_ENDPOINT", "https://api.example.com/v1")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	cfg := LoadConfig()
	if cfg.Mode != "turn" {
		t.Fatalf("unexpected mode: %s", cfg.Mode)
	}
	if cfg.LocalPort != 4700 {
		t.Fatalf("unexpected local port: %d", cfg.LocalPort)
	}
	if cfg.LocalHost != "0.0.0.0" {
		t.Fatalf("unexpected local host: %s", cfg.LocalHost)
	}
	if cfg.WebUIMode != "prod" {
		t.Fatalf("unexpected web ui mode: %s", cfg.WebUIMode)
	}
	if cfg.WebUIDevProxyURL != "http://127.0.0.1:25173" {
		t.Fatalf("unexpected web ui dev proxy: %s", cfg.WebUIDevProxyURL)
	}
	if cfg.WebUIDistDir != "/tmp/webui-dist" {
		t.Fatalf("unexpected web ui dist dir: %s", cfg.WebUIDistDir)
	}
	if cfg.OpenAIEndpoint != "https://api.example.com/v1" {
		t.Fatalf("unexpected openai endpoint: %s", cfg.OpenAIEndpoint)
	}
	if cfg.OpenAIModel != "gpt-5-mini" {
		t.Fatalf("unexpected openai model: %s", cfg.OpenAIModel)
	}
	if cfg.OpenAIAPIKey != "sk-test" {
		t.Fatalf("unexpected openai key")
	}
}

func TestLoadConfig_DefaultLocalPortFromBuildVariable(t *testing.T) {
	old := defaultLocalPort
	defaultLocalPort = "9001"
	t.Cleanup(func() {
		defaultLocalPort = old
	})
	t.Setenv("SHELLMAN_LOCAL_PORT", "")

	cfg := LoadConfig()
	if cfg.LocalPort != 9001 {
		t.Fatalf("unexpected local port from build variable: %d", cfg.LocalPort)
	}
}

func TestLoadConfig_HistoryLines(t *testing.T) {
	t.Setenv("SHELLMAN_HISTORY_LINES", "8000")
	cfg := LoadConfig()
	if cfg.HistoryLines != 8000 {
		t.Fatalf("unexpected history lines: %d", cfg.HistoryLines)
	}
}

func TestGetConfig_UsesCacheWithinTTL(t *testing.T) {
	resetConfigCacheForTest()
	t.Setenv("SHELLMAN_LOCAL_HOST", "127.0.0.1")
	_ = LoadConfig()

	t.Setenv("SHELLMAN_LOCAL_HOST", "0.0.0.0")
	got := GetConfig()
	if got == nil {
		t.Fatal("GetConfig should not return nil")
	}
	if got.LocalHost != "127.0.0.1" {
		t.Fatalf("expected cached host 127.0.0.1, got %s", got.LocalHost)
	}
}

func TestGetConfig_RefreshesAfterTTL(t *testing.T) {
	resetConfigCacheForTest()

	oldNow := nowFunc
	oldTTL := cacheTTL
	defer func() {
		nowFunc = oldNow
		cacheTTL = oldTTL
		resetConfigCacheForTest()
	}()

	base := time.Date(2026, time.February, 19, 0, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time { return base }
	cacheTTL = 10 * time.Second

	t.Setenv("SHELLMAN_LOCAL_HOST", "127.0.0.1")
	_ = LoadConfig()

	base = base.Add(11 * time.Second)
	t.Setenv("SHELLMAN_LOCAL_HOST", "0.0.0.0")

	got := GetConfig()
	if got == nil {
		t.Fatal("GetConfig should not return nil")
	}
	if got.LocalHost != "0.0.0.0" {
		t.Fatalf("expected refreshed host 0.0.0.0, got %s", got.LocalHost)
	}
}

func resetConfigCacheForTest() {
	cacheMu.Lock()
	cachedCfg = Config{}
	cachedAt = time.Time{}
	cacheValid = false
	cacheMu.Unlock()
}
