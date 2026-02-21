package config

import (
	"os"
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
	cacheTTL   = 10 * time.Second
	nowFunc    = time.Now
	cacheMu    sync.RWMutex
	cachedCfg  Config
	cachedAt   time.Time
	cacheValid bool
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
	base := os.Getenv("TERMTEAM_WORKER_BASE_URL")
	if base == "" {
		base = "http://127.0.0.1:8787"
	}

	level := os.Getenv("TERMTEAM_LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	socket := os.Getenv("TERMTEAM_TMUX_SOCKET")
	traceStream := os.Getenv("TERMTEAM_TRACE_STREAM") == "1"
	historyLines := atoiOrDefault(os.Getenv("TERMTEAM_HISTORY_LINES"), 2000)
	if historyLines < 1 {
		historyLines = 2000
	}
	mode := os.Getenv("TERMTEAM_MODE")
	if mode == "" {
		mode = "local"
	}
	localHost := os.Getenv("TERMTEAM_LOCAL_HOST")
	if localHost == "" {
		localHost = "127.0.0.1"
	}
	localPort := 4621
	if p := os.Getenv("TERMTEAM_LOCAL_PORT"); p != "" {
		if p == "0" {
			localPort = 4621
		} else {
			// Keep parsing strict but fallback to default on malformed values.
			if n := atoiOrDefault(p, 4621); n > 0 {
				localPort = n
			}
		}
	}
	webUIMode := os.Getenv("TERMTEAM_WEBUI_MODE")
	if webUIMode == "" {
		webUIMode = "dev"
	}
	webUIDevProxyURL := os.Getenv("TERMTEAM_WEBUI_DEV_PROXY_URL")
	if webUIDevProxyURL == "" {
		webUIDevProxyURL = "http://127.0.0.1:15173"
	}
	webUIDistDir := os.Getenv("TERMTEAM_WEBUI_DIST_DIR")
	if webUIDistDir == "" {
		webUIDistDir = "../webui/dist"
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
