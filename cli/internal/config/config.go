package config

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	WorkerBaseURL    string
	ListenLogLevel   string
	TmuxSocket       string
	TraceStream      bool
	HistoryLines     int
	Mode             string
	TurnEnabled      bool
	LocalHost        string
	LocalPort        int
	WebUIMode        string
	WebUIDevProxyURL string
	WebUIDistDir     string
	OpenAIEndpoint   string
	OpenAIModel      string
	OpenAIAPIKey     string
}

var (
	cacheTTL         = 10 * time.Second
	nowFunc          = time.Now
	cacheMu          sync.RWMutex
	cachedCfg        Config
	cachedAt         time.Time
	cacheValid       bool
	defaultWebUIMode = "dev"
)

func LoadConfig() Config {
	cfg := loadFromEnv()
	cacheMu.Lock()
	cachedCfg = cfg
	cachedAt = nowFunc()
	cacheValid = true
	cacheMu.Unlock()
	return cfg
}

func GetConfig() *Config {
	now := nowFunc()
	cacheMu.RLock()
	valid := cacheValid && now.Sub(cachedAt) < cacheTTL
	if valid {
		out := cachedCfg
		cacheMu.RUnlock()
		return &out
	}
	cacheMu.RUnlock()

	cfg := loadFromEnv()
	cacheMu.Lock()
	cachedCfg = cfg
	cachedAt = now
	cacheValid = true
	cacheMu.Unlock()

	out := cfg
	return &out
}

func loadFromEnv() Config {
	base := os.Getenv("SHELLMAN_WORKER_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8787"
	}

	level := os.Getenv("SHELLMAN_LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	socket := os.Getenv("SHELLMAN_TMUX_SOCKET")
	traceStream := os.Getenv("SHELLMAN_TRACE_STREAM") == "1"
	historyLines := atoiOrDefault(os.Getenv("SHELLMAN_HISTORY_LINES"), 2000)
	if historyLines < 1 {
		historyLines = 2000
	}
	mode := os.Getenv("SHELLMAN_MODE")
	if mode == "" {
		mode = "local"
	}
	turnEnabled := os.Getenv("SHELLMAN_TURN_ENABLED") == "1"
	localHost := os.Getenv("SHELLMAN_LOCAL_HOST")
	if localHost == "" {
		localHost = "127.0.0.1"
	}
	localPort := 4621
	if p := os.Getenv("SHELLMAN_LOCAL_PORT"); p != "" {
		if p == "0" {
			localPort = 4621
		} else {
			// Keep parsing strict but fallback to default on malformed values.
			if n := atoiOrDefault(p, 4621); n > 0 {
				localPort = n
			}
		}
	}
	webUIMode := os.Getenv("SHELLMAN_WEBUI_MODE")
	if webUIMode == "" {
		webUIMode = defaultWebUIMode
	}
	webUIDevProxyURL := os.Getenv("SHELLMAN_WEBUI_DEV_PROXY_URL")
	if webUIDevProxyURL == "" {
		webUIDevProxyURL = "http://127.0.0.1:15173"
	}
	webUIDistDir := os.Getenv("SHELLMAN_WEBUI_DIST_DIR")
	if webUIDistDir == "" {
		webUIDistDir = defaultWebUIDistDir()
	}
	openAIEndpoint := os.Getenv("OPENAI_ENDPOINT")
	openAIModel := os.Getenv("OPENAI_MODEL")
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	return Config{
		WorkerBaseURL:    base,
		ListenLogLevel:   level,
		TmuxSocket:       socket,
		TraceStream:      traceStream,
		HistoryLines:     historyLines,
		Mode:             mode,
		TurnEnabled:      turnEnabled,
		LocalHost:        localHost,
		LocalPort:        localPort,
		WebUIMode:        webUIMode,
		WebUIDevProxyURL: webUIDevProxyURL,
		WebUIDistDir:     webUIDistDir,
		OpenAIEndpoint:   openAIEndpoint,
		OpenAIModel:      openAIModel,
		OpenAIAPIKey:     openAIAPIKey,
	}
}

func defaultWebUIDistDir() string {
	execPath, err := os.Executable()
	if err != nil || execPath == "" {
		return filepath.Clean("../webui/dist")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(execPath), "..", "webui", "dist"))
}

func atoiOrDefault(v string, fallback int) int {
	n := 0
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return fallback
		}
		n = n*10 + int(v[i]-'0')
	}
	if n == 0 {
		return fallback
	}
	return n
}
