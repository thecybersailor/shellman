package projectstate

type TaskNode struct {
	TaskID               string   `json:"task_id"`
	ParentTaskID         string   `json:"parent_task_id,omitempty"`
	Title                string   `json:"title"`
	CurrentCommand       string   `json:"current_command,omitempty"`
	Description          string   `json:"description,omitempty"`
	Flag                 string   `json:"flag,omitempty"`
	FlagDesc             string   `json:"flag_desc,omitempty"`
	FlagReaded           bool     `json:"flag_readed,omitempty"`
	Checked              bool     `json:"checked,omitempty"`
	Archived             bool     `json:"archived,omitempty"`
	Status               string   `json:"status,omitempty"`
	Children             []string `json:"children,omitempty"`
	PendingChildrenCount int      `json:"pending_children_count,omitempty"`
	LastModified         int64    `json:"last_modified,omitempty"`
}

const (
	StatusPending         = "pending"
	StatusRunning         = "running"
	StatusWaitingUser     = "waiting_user"
	StatusWaitingChildren = "waiting_children"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusCanceled        = "canceled"
)

const (
	SidecarModeAdvisor   = "advisor"
	SidecarModeObserver  = "observer"
	SidecarModeAutopilot = "autopilot"
)

type TaskTree struct {
	ProjectID string     `json:"project_id"`
	Nodes     []TaskNode `json:"nodes"`
}

type TaskIndexEntry struct {
	TaskID       string `json:"task_id"`
	ProjectID    string `json:"project_id"`
	ParentTaskID string `json:"parent_task_id,omitempty"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Flag         string `json:"flag,omitempty"`
	FlagDesc     string `json:"flag_desc,omitempty"`
	FlagReaded   bool   `json:"flag_readed,omitempty"`
	Archived     bool   `json:"archived,omitempty"`
	Status       string `json:"status"`
	SidecarMode  string `json:"sidecar_mode,omitempty"`
	LastModified int64  `json:"last_modified"`
	CurrentRunID string `json:"current_run_id,omitempty"`
}

type TaskIndex map[string]TaskIndexEntry

type PaneBinding struct {
	TaskID             string `json:"task_id"`
	PaneUUID           string `json:"pane_uuid,omitempty"`
	PaneID             string `json:"pane_id"`
	PaneTarget         string `json:"pane_target"`
	ShellReadyRequired bool   `json:"shell_ready_required,omitempty"`
	ShellReadyAcked    bool   `json:"shell_ready_acked,omitempty"`
}

type PanesIndex map[string]PaneBinding

type PaneSnapshot struct {
	Output    string `json:"output"`
	FrameMode string `json:"frame_mode,omitempty"`
	FrameData string `json:"frame_data,omitempty"`
	CursorX   int    `json:"cursor_x,omitempty"`
	CursorY   int    `json:"cursor_y,omitempty"`
	HasCursor bool   `json:"has_cursor,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type PaneSnapshotsIndex map[string]PaneSnapshot

type TaskNoteRecord struct {
	TaskID    string `json:"task_id"`
	CreatedAt int64  `json:"created_at"`
	Flag      string `json:"flag,omitempty"`
	Notes     string `json:"notes"`
}

type TaskMessageRecord struct {
	ID        int64  `json:"id"`
	TaskID    string `json:"task_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	ErrorText string `json:"error_text,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
