package localapi

import (
	"context"
	"encoding/json"
	"net/http"

	"shellman/cli/internal/fsbrowser"
	"shellman/cli/internal/global"
	"shellman/cli/internal/helperconfig"
	"shellman/cli/internal/historydb"
)

type ConfigStore interface {
	LoadOrInit() (global.GlobalConfig, error)
	Save(cfg global.GlobalConfig) error
}

type AppProgramsStore interface {
	LoadOrInit() (global.AppProgramsConfig, error)
	Save(cfg global.AppProgramsConfig) error
}

type HelperConfigStore interface {
	LoadOpenAI() (helperconfig.OpenAIConfig, error)
	SaveOpenAI(cfg helperconfig.OpenAIConfig) error
}

type ProjectsStore interface {
	ListProjects() ([]global.ActiveProject, error)
	AddProject(project global.ActiveProject) error
	RemoveProject(projectID string) error
}

type PaneService interface {
	CreateSiblingPane(targetTaskID string) (string, error)
	CreateChildPane(targetTaskID string) (string, error)
	CreateRootPane() (string, error)
}

type FSBrowser interface {
	Roots() ([]string, error)
	List(path string) (fsbrowser.ListResult, error)
	Resolve(path string) (string, error)
	Search(base, q string, limit int) ([]fsbrowser.Item, error)
}

type DirHistory interface {
	Upsert(path string) error
	List(limit int) ([]historydb.Entry, error)
	Clear() error
}

type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type TaskPromptSender interface {
	SendInput(target, text string) error
}

type AgentLoopRunner interface {
	Run(ctx context.Context, userPrompt string) (string, error)
}

type Deps struct {
	ConfigStore         ConfigStore
	AppProgramsStore    AppProgramsStore
	HelperConfigStore   HelperConfigStore
	ProjectsStore       ProjectsStore
	PaneService         PaneService
	TaskPromptSender    TaskPromptSender
	ExecuteCommand      CommandRunner
	PickDirectory       func() (string, error)
	FSBrowser           FSBrowser
	DirHistory          DirHistory
	AgentLoopRunner     AgentLoopRunner
	AgentOpenAIEndpoint string
	AgentOpenAIModel    string
}

type Server struct {
	deps Deps
	mux  *http.ServeMux
	hub  *WSHub

	taskAgentSupervisor *taskAgentLoopSupervisor
	externalEventSink   func(topic, projectID, taskID string, payload map[string]any)
}

func NewServer(deps Deps) *Server {
	s := &Server{deps: deps, mux: http.NewServeMux(), hub: NewWSHub()}
	s.taskAgentSupervisor = newTaskAgentLoopSupervisor(nil, s.handleTaskAgentLoopEvent)
	s.registerConfigRoutes()
	s.registerProjectsRoutes()
	s.registerSystemRoutes()
	s.registerFSRoutes()
	s.registerTaskRoutes()
	s.registerRunRoutes()
	s.registerPaneRoutes()
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/ws", s.hub.HandleWS)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondOK(w, map[string]any{"status": "ok"})
}

func respondOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func respondError(w http.ResponseWriter, code int, errCode string, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": map[string]any{"code": errCode, "message": msg}})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) SetExternalEventSink(sink func(topic, projectID, taskID string, payload map[string]any)) {
	if s == nil {
		return
	}
	s.externalEventSink = sink
}

func (s *Server) publishEvent(topic, projectID, taskID string, payload map[string]any) {
	if s.hub == nil {
		return
	}
	s.hub.Publish(topic, projectID, taskID, payload)
	if s.externalEventSink != nil {
		s.externalEventSink(topic, projectID, taskID, cloneMap(payload))
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
