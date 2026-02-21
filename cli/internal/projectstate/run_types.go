package projectstate

import "errors"

const (
	RunStatusRunning     = "running"
	RunStatusNeedsRebind = "needs_rebind"
	RunStatusCompleted   = "completed"
)

const (
	BindingStatusLive  = "live"
	BindingStatusStale = "stale"
)

var ErrDuplicateInboxRequest = errors.New("duplicate inbox request")

type TaskRecord struct {
	TaskID         string
	ProjectID      string
	ParentTaskID   string
	Title          string
	CurrentCommand string
	Status         string
	SidecarMode    string
	Description    string
	Flag           string
	FlagDesc       string
	FlagReaded     bool
	Checked        bool
	Archived       bool
	LastModified   int64
}

type TaskRecordRow struct {
	TaskID         string
	ProjectID      string
	ParentTaskID   string
	Title          string
	CurrentCommand string
	Status         string
	SidecarMode    string
	Description    string
	Flag           string
	FlagDesc       string
	FlagReaded     bool
	Checked        bool
	Archived       bool
	CreatedAt      int64
	LastModified   int64
}

type TaskMetaUpsert struct {
	TaskID         string
	ProjectID      string
	ParentTaskID   *string
	Title          *string
	CurrentCommand *string
	Status         *string
	SidecarMode    *string
	Description    *string
	Flag           *string
	FlagDesc       *string
	FlagReaded     *bool
	Checked        *bool
	Archived       *bool
	LastModified   int64
}

type PaneRuntimeRecord struct {
	PaneID         string `json:"pane_id"`
	PaneTarget     string `json:"pane_target"`
	CurrentCommand string `json:"current_command"`
	RuntimeStatus  string `json:"runtime_status"`
	Snapshot       string `json:"snapshot"`
	SnapshotHash   string `json:"snapshot_hash"`
	CursorX        int    `json:"cursor_x"`
	CursorY        int    `json:"cursor_y"`
	HasCursor      bool   `json:"has_cursor"`
	UpdatedAt      int64  `json:"updated_at"`
}

type TaskRuntimeRecord struct {
	TaskID         string `json:"task_id"`
	SourcePaneID   string `json:"source_pane_id"`
	CurrentCommand string `json:"current_command"`
	RuntimeStatus  string `json:"runtime_status"`
	SnapshotHash   string `json:"snapshot_hash"`
	UpdatedAt      int64  `json:"updated_at"`
}

type RuntimeBatchUpdate struct {
	Panes []PaneRuntimeRecord
	Tasks []TaskRuntimeRecord
}

type RunRecord struct {
	RunID       string
	TaskID      string
	RunStatus   string
	StartedAt   int64
	CompletedAt int64
	UpdatedAt   int64
	LastError   string
}

type RunBinding struct {
	RunID            string
	ServerInstanceID string
	PaneID           string
	PaneTarget       string
	BindingStatus    string
	StaleReason      string
}

type RunLookupCandidate struct {
	RunID            string
	TaskID           string
	RunStatus        string
	PaneID           string
	PaneTarget       string
	BindingStatus    string
	StaleReason      string
	ServerInstanceID string
	RunUpdatedAt     int64
	BindingUpdatedAt int64
}
