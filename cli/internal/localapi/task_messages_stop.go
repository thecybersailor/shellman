package localapi

import (
	"context"
	"strings"
)

type taskMessageRunState struct {
	assistantMessageID int64
	cancel             context.CancelFunc
}

func (s *Server) setTaskMessageRun(taskID string, assistantMessageID int64, cancel context.CancelFunc) {
	if s == nil || cancel == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || assistantMessageID <= 0 {
		return
	}
	s.taskMessageRunMu.Lock()
	defer s.taskMessageRunMu.Unlock()
	s.taskMessageRunByTask[taskID] = taskMessageRunState{
		assistantMessageID: assistantMessageID,
		cancel:             cancel,
	}
}

func (s *Server) clearTaskMessageRun(taskID string) {
	if s == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	s.taskMessageRunMu.Lock()
	defer s.taskMessageRunMu.Unlock()
	delete(s.taskMessageRunByTask, taskID)
}

func (s *Server) stopTaskMessageRun(taskID string) (int64, bool) {
	if s == nil {
		return 0, false
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return 0, false
	}
	s.taskMessageRunMu.Lock()
	state, ok := s.taskMessageRunByTask[taskID]
	if ok {
		delete(s.taskMessageRunByTask, taskID)
	}
	s.taskMessageRunMu.Unlock()
	if !ok || state.cancel == nil {
		return 0, false
	}
	state.cancel()
	return state.assistantMessageID, true
}
