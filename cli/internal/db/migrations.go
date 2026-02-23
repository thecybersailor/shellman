package db

import (
	"errors"

	"shellman/cli/internal/db/migration"

	"gorm.io/gorm"
)

// SyncSchema creates/updates tables and indexes from models. Table structure changes do not use versioned migrations.
func SyncSchema(db *gorm.DB) error {
	if db == nil {
		return errors.New("db is required")
	}
	if err := db.AutoMigrate(
		&Task{},
		&TaskRun{},
		&RunBinding{},
		&RunEvent{},
		&CompletionInbox{},
		&Note{},
		&TaskMessage{},
		&PMSession{},
		&PMMessage{},
		&ActionOutbox{},
		&TmuxServer{},
		&LegacyState{},
		&DirHistory{},
		&PaneRuntime{},
		&TaskRuntime{},
		&Config{},
	); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_task_runs_task_id_started_at ON task_runs(task_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_status_retry ON action_outbox(status, next_retry_at);`,
		`CREATE INDEX IF NOT EXISTS idx_notes_task_created_at ON notes(task_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_task_messages_task_created_at ON task_messages(task_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_pm_sessions_repo_project_updated ON pm_sessions(repo_root, project_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_pm_messages_session_created_at ON pm_messages(session_id, created_at DESC);`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

// MigrateUp syncs schema then runs data migrations (botworks-style). Kept for compatibility with OpenSQLiteWithMigrations.
func MigrateUp(db *gorm.DB) error {
	if err := SyncSchema(db); err != nil {
		return err
	}
	migration.Init()
	return migration.RunAll(db)
}
