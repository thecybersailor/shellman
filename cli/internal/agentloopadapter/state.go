package agentloopadapter

import "strings"

type StateMode string

const (
	ModeTask StateMode = "task"
	ModePM   StateMode = "pm"
)

type State struct {
	TaskID    string
	ProjectID string
	SessionID string
	Source    string
	Mode      StateMode
}

func normalizeState(in State) State {
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.SessionID = strings.TrimSpace(in.SessionID)
	in.Source = strings.TrimSpace(in.Source)
	if strings.TrimSpace(string(in.Mode)) == "" {
		in.Mode = ModeTask
	}
	return in
}
