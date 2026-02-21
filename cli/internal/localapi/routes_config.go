package localapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"shellman/cli/internal/global"
)

type helperOpenAIResponse struct {
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	APIKeySet bool   `json:"api_key_set"`
}

type agentOpenAIResponse struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Enabled  bool   `json:"enabled"`
}

type configResponse struct {
	LocalPort                  int                          `json:"local_port"`
	Defaults                   any                          `json:"defaults"`
	TaskCompletionMode         string                       `json:"task_completion_mode"`
	TaskCompletionCommand      string                       `json:"task_completion_command"`
	TaskCompletionIdleDuration int                          `json:"task_completion_idle_duration_seconds"`
	TaskCompletion             taskCompletionConfigResponse `json:"task_completion"`
	HelperOpenAI               helperOpenAIResponse         `json:"helper_openai"`
	AgentOpenAI                agentOpenAIResponse          `json:"agent_openai"`
}

type taskCompletionConfigResponse struct {
	NotifyEnabled      bool   `json:"notify_enabled"`
	NotifyCommand      string `json:"notify_command"`
	NotifyIdleDuration int    `json:"notify_idle_duration_seconds"`
}

func buildConfigResponse(cfg global.GlobalConfig, helper helperOpenAIResponse, agent agentOpenAIResponse) (configResponse, error) {
	mode := "none"
	if cfg.TaskCompletion.NotifyEnabled && strings.TrimSpace(cfg.TaskCompletion.NotifyCommand) != "" {
		mode = "command"
	}
	return configResponse{
		LocalPort:                  cfg.LocalPort,
		Defaults:                   cfg.Defaults,
		TaskCompletionMode:         mode,
		TaskCompletionCommand:      cfg.TaskCompletion.NotifyCommand,
		TaskCompletionIdleDuration: cfg.TaskCompletion.NotifyIdleDuration,
		TaskCompletion: taskCompletionConfigResponse{
			NotifyEnabled:      cfg.TaskCompletion.NotifyEnabled,
			NotifyCommand:      cfg.TaskCompletion.NotifyCommand,
			NotifyIdleDuration: cfg.TaskCompletion.NotifyIdleDuration,
		},
		HelperOpenAI: helper,
		AgentOpenAI:  agent,
	}, nil
}

func (s *Server) registerConfigRoutes() {
	s.mux.HandleFunc("/api/v1/config", s.handleConfig)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.deps.ConfigStore.LoadOrInit()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED", err.Error())
			return
		}
		helperResp := helperOpenAIResponse{}
		if s.deps.HelperConfigStore != nil {
			helperCfg, err := s.deps.HelperConfigStore.LoadOpenAI()
			if err != nil {
				respondError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED", err.Error())
				return
			}
			helperResp.Endpoint = helperCfg.Endpoint
			helperResp.Model = helperCfg.Model
			helperResp.APIKeySet = helperCfg.APIKeySet
		}
		agentResp := agentOpenAIResponse{
			Endpoint: strings.TrimSpace(s.deps.AgentOpenAIEndpoint),
			Model:    strings.TrimSpace(s.deps.AgentOpenAIModel),
			Enabled:  s.deps.AgentLoopRunner != nil,
		}
		out, err := buildConfigResponse(cfg, helperResp, agentResp)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED", err.Error())
			return
		}
		respondOK(w, out)
	case http.MethodPatch:
		var req struct {
			LocalPort *int `json:"local_port"`
			Defaults  *struct {
				SessionProgram *string `json:"session_program"`
				HelperProgram  *string `json:"helper_program"`
			} `json:"defaults"`
			HelperOpenAI *struct {
				Endpoint *string `json:"endpoint"`
				Model    *string `json:"model"`
				APIKey   *string `json:"api_key"`
			} `json:"helper_openai"`
			TaskCompletionMode         *string `json:"task_completion_mode"`
			TaskCompletionCommand      *string `json:"task_completion_command"`
			TaskCompletionIdleDuration *int    `json:"task_completion_idle_duration_seconds"`
			TaskCompletion             *struct {
				NotifyEnabled      *bool   `json:"notify_enabled"`
				NotifyCommand      *string `json:"notify_command"`
				NotifyIdleDuration *int    `json:"notify_idle_duration_seconds"`
			} `json:"task_completion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		cfg, err := s.deps.ConfigStore.LoadOrInit()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "CONFIG_LOAD_FAILED", err.Error())
			return
		}
		if req.LocalPort != nil {
			cfg.LocalPort = *req.LocalPort
		}
		if req.Defaults != nil {
			if req.Defaults.SessionProgram != nil {
				cfg.Defaults.SessionProgram = strings.TrimSpace(*req.Defaults.SessionProgram)
			}
			if req.Defaults.HelperProgram != nil {
				cfg.Defaults.HelperProgram = strings.TrimSpace(*req.Defaults.HelperProgram)
			}
		}
		if req.TaskCompletionMode != nil {
			mode := strings.TrimSpace(*req.TaskCompletionMode)
			switch mode {
			case "command":
				cfg.TaskCompletion.NotifyEnabled = true
			case "none":
				cfg.TaskCompletion.NotifyEnabled = false
			default:
				respondError(w, http.StatusBadRequest, "INVALID_TASK_COMPLETION_MODE", "task_completion_mode must be command or none")
				return
			}
		}
		if req.TaskCompletionCommand != nil {
			cfg.TaskCompletion.NotifyCommand = *req.TaskCompletionCommand
		}
		if req.TaskCompletionIdleDuration != nil {
			cfg.TaskCompletion.NotifyIdleDuration = *req.TaskCompletionIdleDuration
		}
		if req.TaskCompletion != nil {
			if req.TaskCompletion.NotifyEnabled != nil {
				cfg.TaskCompletion.NotifyEnabled = *req.TaskCompletion.NotifyEnabled
			}
			if req.TaskCompletion.NotifyCommand != nil {
				cfg.TaskCompletion.NotifyCommand = *req.TaskCompletion.NotifyCommand
			}
			if req.TaskCompletion.NotifyIdleDuration != nil {
				cfg.TaskCompletion.NotifyIdleDuration = *req.TaskCompletion.NotifyIdleDuration
			}
		}
		if err := s.deps.ConfigStore.Save(cfg); err != nil {
			respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
			return
		}

		helperResp := helperOpenAIResponse{}
		if req.HelperOpenAI != nil {
			if s.deps.HelperConfigStore == nil {
				respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", "helper config store is unavailable")
				return
			}
			helperCfg, err := s.deps.HelperConfigStore.LoadOpenAI()
			if err != nil {
				respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
				return
			}
			if req.HelperOpenAI.Endpoint != nil {
				helperCfg.Endpoint = strings.TrimSpace(*req.HelperOpenAI.Endpoint)
			}
			if req.HelperOpenAI.Model != nil {
				helperCfg.Model = strings.TrimSpace(*req.HelperOpenAI.Model)
			}
			if req.HelperOpenAI.APIKey != nil {
				helperCfg.APIKey = strings.TrimSpace(*req.HelperOpenAI.APIKey)
				helperCfg.APIKeySet = helperCfg.APIKey != ""
			}
			if err := s.deps.HelperConfigStore.SaveOpenAI(helperCfg); err != nil {
				respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
				return
			}
			helperResp.Endpoint = helperCfg.Endpoint
			helperResp.Model = helperCfg.Model
			helperResp.APIKeySet = helperCfg.APIKeySet
		} else if s.deps.HelperConfigStore != nil {
			helperCfg, err := s.deps.HelperConfigStore.LoadOpenAI()
			if err != nil {
				respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
				return
			}
			helperResp.Endpoint = helperCfg.Endpoint
			helperResp.Model = helperCfg.Model
			helperResp.APIKeySet = helperCfg.APIKeySet
		}

		agentResp := agentOpenAIResponse{
			Endpoint: strings.TrimSpace(s.deps.AgentOpenAIEndpoint),
			Model:    strings.TrimSpace(s.deps.AgentOpenAIModel),
			Enabled:  s.deps.AgentLoopRunner != nil,
		}
		out, err := buildConfigResponse(cfg, helperResp, agentResp)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "CONFIG_SAVE_FAILED", err.Error())
			return
		}
		respondOK(w, out)
	default:
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
