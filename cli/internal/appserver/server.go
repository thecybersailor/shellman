package appserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	pprofhttp "net/http/pprof"
	"strings"

	"shellman/cli/internal/localapi"
)

type WebUIConfig struct {
	Mode        string
	DevProxyURL string
	DistDir     string
}

type Deps struct {
	LocalAPI       localapi.Deps
	LocalAPIHandle http.Handler
	WebUI          WebUIConfig
	EnablePprof    bool
}

type Server struct {
	local http.Handler
	webui http.Handler
	edge  *EdgeWSHub
	pprof http.Handler
}

func NewServer(deps Deps) (*Server, error) {
	webui, err := newWebUIHandler(deps.WebUI)
	if err != nil {
		return nil, err
	}
	local := deps.LocalAPIHandle
	if local == nil {
		local = localapi.NewServer(deps.LocalAPI).Handler()
	}
	var pprofHandler http.Handler
	if deps.EnablePprof {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", pprofhttp.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprofhttp.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprofhttp.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprofhttp.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprofhttp.Trace)
		pprofHandler = mux
	}
	return &Server{
		local: local,
		webui: webui,
		edge:  NewEdgeWSHub(),
		pprof: pprofHandler,
	}, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) PublishClientEvent(turnID, topic, projectID, taskID string, payload map[string]any) {
	if s == nil || s.edge == nil {
		return
	}
	s.edge.PublishClientEvent(turnID, topic, projectID, taskID, payload)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/ws/agent/") || strings.HasPrefix(p, "/ws/client/"):
		s.edge.ServeHTTP(w, r)
		return
	case p == "/debug/pprof" || strings.HasPrefix(p, "/debug/pprof/"):
		if s.pprof == nil {
			http.NotFound(w, r)
			return
		}
		s.pprof.ServeHTTP(w, r)
		return
	case p == "/ws" || p == "/healthz" || strings.HasPrefix(p, "/api/v1/"):
		s.local.ServeHTTP(w, r)
		return
	case strings.HasPrefix(p, "/mcp"):
		handleMCP(w, r)
		return
	default:
		s.webui.ServeHTTP(w, r)
	}
}

func handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp/health" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"ok":    false,
			"error": map[string]any{"code": "MCP_NOT_FOUND", "message": "mcp route not found"},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": map[string]any{"service": "shellman-mcp", "status": "ok"}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func routeError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
