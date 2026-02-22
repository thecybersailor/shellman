package db

type Task struct {
	TaskID             string `gorm:"column:task_id;primaryKey"`
	RepoRoot           string `gorm:"column:repo_root;not null;default:''"`
	ProjectID          string `gorm:"column:project_id;not null;default:''"`
	ParentTaskID       string `gorm:"column:parent_task_id;not null;default:''"`
	Title              string `gorm:"column:title;not null;default:''"`
	CurrentCommand     string `gorm:"column:current_command;not null;default:''"`
	Status             string `gorm:"column:status;not null;default:''"`
	SidecarMode        string `gorm:"column:sidecar_mode;not null;default:'advisor'"`
	TaskRole           string `gorm:"column:task_role;not null;default:'full'"`
	Description        string `gorm:"column:description;not null;default:''"`
	Flag               string `gorm:"column:flag;not null;default:''"`
	FlagDesc           string `gorm:"column:flag_desc;not null;default:''"`
	FlagReaded         bool   `gorm:"column:flag_readed;not null;default:false"`
	Checked            bool   `gorm:"column:checked;not null;default:false"`
	Archived           bool   `gorm:"column:archived;not null;default:false"`
	CreatedAt          int64  `gorm:"column:created_at;not null;default:0"`
	LastModified       int64  `gorm:"column:last_modified;not null;default:0"`
	CompletedAt        int64  `gorm:"column:completed_at;not null;default:0"`
	LastAutoProgressAt int64  `gorm:"column:last_auto_progress_at;not null;default:0"`
}

func (Task) TableName() string { return "tasks" }

type TaskRun struct {
	RunID       string `gorm:"column:run_id;primaryKey"`
	TaskID      string `gorm:"column:task_id;not null"`
	RunStatus   string `gorm:"column:run_status;not null;default:'running'"`
	StartedAt   int64  `gorm:"column:started_at;not null;default:0"`
	CompletedAt int64  `gorm:"column:completed_at;not null;default:0"`
	UpdatedAt   int64  `gorm:"column:updated_at;not null;default:0"`
	LastError   string `gorm:"column:last_error;not null;default:''"`
}

func (TaskRun) TableName() string { return "task_runs" }

type RunBinding struct {
	RunID            string `gorm:"column:run_id;primaryKey"`
	ServerInstanceID string `gorm:"column:server_instance_id;not null;default:''"`
	PaneID           string `gorm:"column:pane_id;not null;default:''"`
	PaneTarget       string `gorm:"column:pane_target;not null;default:''"`
	BindingStatus    string `gorm:"column:binding_status;not null;default:'live'"`
	StaleReason      string `gorm:"column:stale_reason;not null;default:''"`
	UpdatedAt        int64  `gorm:"column:updated_at;not null;default:0"`
}

func (RunBinding) TableName() string { return "run_bindings" }

type RunEvent struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	RunID       string `gorm:"column:run_id;not null"`
	EventType   string `gorm:"column:event_type;not null"`
	PayloadJSON string `gorm:"column:payload_json;not null;default:''"`
	CreatedAt   int64  `gorm:"column:created_at;not null;default:0"`
}

func (RunEvent) TableName() string { return "run_events" }

type CompletionInbox struct {
	RunID     string `gorm:"column:run_id;primaryKey"`
	RequestID string `gorm:"column:request_id;primaryKey"`
	Summary   string `gorm:"column:summary;not null;default:''"`
	Source    string `gorm:"column:source;not null;default:''"`
	CreatedAt int64  `gorm:"column:created_at;not null;default:0"`
}

func (CompletionInbox) TableName() string { return "completion_inbox" }

type Note struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	TaskID    string `gorm:"column:task_id;not null"`
	CreatedAt int64  `gorm:"column:created_at;not null;default:0"`
	Flag      string `gorm:"column:flag;not null;default:''"`
	Notes     string `gorm:"column:notes;not null;default:''"`
}

func (Note) TableName() string { return "notes" }

type TaskMessage struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	TaskID    string `gorm:"column:task_id;not null;index"`
	Role      string `gorm:"column:role;not null;default:''"`
	Content   string `gorm:"column:content;not null;default:''"`
	Status    string `gorm:"column:status;not null;default:'completed'"`
	ErrorText string `gorm:"column:error_text;not null;default:''"`
	CreatedAt int64  `gorm:"column:created_at;not null;default:0"`
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0"`
}

func (TaskMessage) TableName() string { return "task_messages" }

type ActionOutbox struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	RunID       string `gorm:"column:run_id;not null"`
	ActionType  string `gorm:"column:action_type;not null"`
	PayloadJSON string `gorm:"column:payload_json;not null;default:''"`
	Status      string `gorm:"column:status;not null;default:'pending'"`
	RetryCount  int    `gorm:"column:retry_count;not null;default:0"`
	NextRetryAt int64  `gorm:"column:next_retry_at;not null;default:0"`
	CreatedAt   int64  `gorm:"column:created_at;not null;default:0"`
	UpdatedAt   int64  `gorm:"column:updated_at;not null;default:0"`
}

func (ActionOutbox) TableName() string { return "action_outbox" }

type TmuxServer struct {
	ServerInstanceID string `gorm:"column:server_instance_id;primaryKey"`
	SocketPath       string `gorm:"column:socket_path;not null;default:''"`
	PID              string `gorm:"column:pid;not null;default:''"`
	LastSeenAt       int64  `gorm:"column:last_seen_at;not null;default:0"`
}

func (TmuxServer) TableName() string { return "tmux_servers" }

type LegacyState struct {
	RepoRoot  string `gorm:"column:repo_root;primaryKey"`
	StateKey  string `gorm:"column:state_key;primaryKey"`
	StateJSON string `gorm:"column:state_json;not null;default:''"`
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0"`
}

func (LegacyState) TableName() string { return "legacy_state" }

type DirHistory struct {
	Path            string `gorm:"column:path;primaryKey"`
	FirstAccessedAt int64  `gorm:"column:first_accessed_at;not null"`
	LastAccessedAt  int64  `gorm:"column:last_accessed_at;not null"`
	AccessCount     int    `gorm:"column:access_count;not null"`
}

func (DirHistory) TableName() string { return "dir_history" }

type PaneRuntime struct {
	PaneID         string `gorm:"column:pane_id;primaryKey"`
	PaneTarget     string `gorm:"column:pane_target;not null;default:''"`
	CurrentCommand string `gorm:"column:current_command;not null;default:''"`
	RuntimeStatus  string `gorm:"column:runtime_status;not null;default:''"`
	Snapshot       string `gorm:"column:snapshot;not null;default:''"`
	SnapshotHash   string `gorm:"column:snapshot_hash;not null;default:''"`
	CursorX        int    `gorm:"column:cursor_x;not null;default:0"`
	CursorY        int    `gorm:"column:cursor_y;not null;default:0"`
	HasCursor      bool   `gorm:"column:has_cursor;not null;default:false"`
	UpdatedAt      int64  `gorm:"column:updated_at;not null;default:0"`
}

func (PaneRuntime) TableName() string { return "pane_runtime" }

type TaskRuntime struct {
	TaskID         string `gorm:"column:task_id;primaryKey"`
	SourcePaneID   string `gorm:"column:source_pane_id;not null;default:''"`
	CurrentCommand string `gorm:"column:current_command;not null;default:''"`
	RuntimeStatus  string `gorm:"column:runtime_status;not null;default:''"`
	SnapshotHash   string `gorm:"column:snapshot_hash;not null;default:''"`
	UpdatedAt      int64  `gorm:"column:updated_at;not null;default:0"`
}

func (TaskRuntime) TableName() string { return "task_runtime" }

type Config struct {
	Key       string `gorm:"column:key;primaryKey"`
	Value     string `gorm:"column:value;not null;default:''"`
	UpdatedAt int64  `gorm:"column:updated_at;not null;default:0"`
}

func (Config) TableName() string { return "config" }
