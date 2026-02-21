package appserver

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func newWebUIHandler(cfg WebUIConfig) (http.Handler, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "dev"
	}
	if mode == "prod" {
		dist := cfg.DistDir
		if dist == "" {
			dist = filepath.Clean("../webui/dist")
		}
		return newSPAHandler(dist), nil
	}
	proxyURL := cfg.DevProxyURL
	if proxyURL == "" {
		proxyURL = "http://127.0.0.1:15173"
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, routeError("invalid dev proxy url: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "webui dev server unavailable (start vite on 127.0.0.1:15173)", http.StatusBadGateway)
	}
	return proxy, nil
}

type spaHandler struct {
	dist string
}

func newSPAHandler(dist string) http.Handler {
	return &spaHandler{dist: dist}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := filepath.Clean("/" + r.URL.Path)
	indexPath := filepath.Join(h.dist, "index.html")
	if clean == "/" {
		http.ServeFile(w, r, indexPath)
		return
	}
	candidate := filepath.Join(h.dist, strings.TrimPrefix(clean, "/"))
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}
	http.ServeFile(w, r, indexPath)
}
