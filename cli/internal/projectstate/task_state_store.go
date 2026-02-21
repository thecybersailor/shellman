package projectstate

import (
	"database/sql"
	"errors"
	"time"
)

func (s *Store) ListTasksByProject(projectID string) ([]TaskRecordRow, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	rows, err := db.Query(`
SELECT task_id, project_id, parent_task_id, title, current_command, status, sidecar_mode, description, flag, flag_desc, flag_readed, checked, archived, created_at, last_modified
FROM tasks
WHERE repo_root = ? AND project_id = ? AND archived = false
ORDER BY created_at ASC, task_id ASC
`, s.repoRoot, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]TaskRecordRow, 0)
	for rows.Next() {
		var row TaskRecordRow
		if err := rows.Scan(
			&row.TaskID,
			&row.ProjectID,
			&row.ParentTaskID,
			&row.Title,
			&row.CurrentCommand,
			&row.Status,
			&row.SidecarMode,
			&row.Description,
			&row.Flag,
			&row.FlagDesc,
			&row.FlagReaded,
			&row.Checked,
			&row.Archived,
			&row.CreatedAt,
			&row.LastModified,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) UpsertTaskMeta(input TaskMetaUpsert) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	if input.TaskID == "" {
		return errors.New("task id is required")
	}
	if input.ProjectID == "" {
		return errors.New("project id is required")
	}

	now := input.LastModified
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}

	hasParent := input.ParentTaskID != nil
	hasTitle := input.Title != nil
	hasCurrentCommand := input.CurrentCommand != nil
	hasStatus := input.Status != nil
	hasSidecarMode := input.SidecarMode != nil
	hasDescription := input.Description != nil
	hasFlag := input.Flag != nil
	hasFlagDesc := input.FlagDesc != nil
	hasFlagReaded := input.FlagReaded != nil
	hasChecked := input.Checked != nil
	hasArchived := input.Archived != nil

	_, err = db.Exec(`
INSERT INTO tasks(task_id, repo_root, project_id, parent_task_id, title, current_command, status, sidecar_mode, description, flag, flag_desc, flag_readed, checked, archived, created_at, last_modified)
VALUES (?, ?, ?, COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, 'advisor'), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, ''), COALESCE(?, false), COALESCE(?, false), COALESCE(?, false), ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
  project_id = CASE WHEN excluded.project_id <> '' THEN excluded.project_id ELSE tasks.project_id END,
  parent_task_id = CASE WHEN ? THEN excluded.parent_task_id ELSE tasks.parent_task_id END,
  title = CASE WHEN ? THEN excluded.title ELSE tasks.title END,
  current_command = CASE WHEN ? THEN excluded.current_command ELSE tasks.current_command END,
  status = CASE WHEN ? THEN excluded.status ELSE tasks.status END,
  sidecar_mode = CASE WHEN ? THEN excluded.sidecar_mode ELSE tasks.sidecar_mode END,
  description = CASE WHEN ? THEN excluded.description ELSE tasks.description END,
  flag = CASE WHEN ? THEN excluded.flag ELSE tasks.flag END,
  flag_desc = CASE WHEN ? THEN excluded.flag_desc ELSE tasks.flag_desc END,
  flag_readed = CASE WHEN ? THEN excluded.flag_readed ELSE tasks.flag_readed END,
  checked = CASE WHEN ? THEN excluded.checked ELSE tasks.checked END,
  archived = CASE WHEN ? THEN excluded.archived ELSE tasks.archived END,
  last_modified = excluded.last_modified
`,
		input.TaskID,
		s.repoRoot,
		input.ProjectID,
		input.ParentTaskID,
		input.Title,
		input.CurrentCommand,
		input.Status,
		input.SidecarMode,
		input.Description,
		input.Flag,
		input.FlagDesc,
		input.FlagReaded,
		input.Checked,
		input.Archived,
		now,
		now,
		hasParent,
		hasTitle,
		hasCurrentCommand,
		hasStatus,
		hasSidecarMode,
		hasDescription,
		hasFlag,
		hasFlagDesc,
		hasFlagReaded,
		hasChecked,
		hasArchived,
	)
	return err
}

func (s *Store) ArchiveCheckedTasksByProject(projectID string) (int64, error) {
	db, release, err := s.db()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	res, err := db.Exec(`
UPDATE tasks
SET archived = true, last_modified = ?
WHERE repo_root = ? AND project_id = ? AND checked = true AND archived = false
`, now, s.repoRoot, projectID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) BatchUpsertRuntime(input RuntimeBatchUpdate) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Unix()
	for _, pane := range input.Panes {
		updatedAt := pane.UpdatedAt
		if updatedAt <= 0 {
			updatedAt = now
		}
		if _, err := tx.Exec(`
INSERT INTO pane_runtime(pane_id, pane_target, current_command, runtime_status, snapshot, snapshot_hash, cursor_x, cursor_y, has_cursor, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(pane_id) DO UPDATE SET
  pane_target = excluded.pane_target,
  current_command = excluded.current_command,
  runtime_status = excluded.runtime_status,
  snapshot = excluded.snapshot,
  snapshot_hash = excluded.snapshot_hash,
  cursor_x = excluded.cursor_x,
  cursor_y = excluded.cursor_y,
  has_cursor = excluded.has_cursor,
  updated_at = excluded.updated_at
`, pane.PaneID, pane.PaneTarget, pane.CurrentCommand, pane.RuntimeStatus, pane.Snapshot, pane.SnapshotHash, pane.CursorX, pane.CursorY, pane.HasCursor, updatedAt); err != nil {
			return err
		}
	}

	for _, task := range input.Tasks {
		updatedAt := task.UpdatedAt
		if updatedAt <= 0 {
			updatedAt = now
		}
		if _, err := tx.Exec(`
INSERT INTO task_runtime(task_id, source_pane_id, current_command, runtime_status, snapshot_hash, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
  source_pane_id = excluded.source_pane_id,
  current_command = excluded.current_command,
  runtime_status = excluded.runtime_status,
  snapshot_hash = excluded.snapshot_hash,
  updated_at = excluded.updated_at
`, task.TaskID, task.SourcePaneID, task.CurrentCommand, task.RuntimeStatus, task.SnapshotHash, updatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(`
UPDATE tasks
SET current_command = ?
WHERE repo_root = ? AND task_id = ?
`, task.CurrentCommand, s.repoRoot, task.TaskID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetPaneRuntimeByPaneID(paneID string) (PaneRuntimeRecord, bool, error) {
	db, release, err := s.db()
	if err != nil {
		return PaneRuntimeRecord{}, false, err
	}
	defer func() { _ = release() }()

	var row PaneRuntimeRecord
	err = db.QueryRow(`
SELECT pane_id, pane_target, current_command, runtime_status, snapshot, snapshot_hash, cursor_x, cursor_y, has_cursor, updated_at
FROM pane_runtime
WHERE pane_id = ?
`, paneID).Scan(
		&row.PaneID,
		&row.PaneTarget,
		&row.CurrentCommand,
		&row.RuntimeStatus,
		&row.Snapshot,
		&row.SnapshotHash,
		&row.CursorX,
		&row.CursorY,
		&row.HasCursor,
		&row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PaneRuntimeRecord{}, false, nil
	}
	if err != nil {
		return PaneRuntimeRecord{}, false, err
	}
	return row, true, nil
}

func (s *Store) GetProjectMaxTaskLastModified(projectID string) (int64, error) {
	db, release, err := s.db()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	var lastModified int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(last_modified), 0) FROM tasks WHERE repo_root = ? AND project_id = ?`, s.repoRoot, projectID).Scan(&lastModified); err != nil {
		return 0, err
	}
	return lastModified, nil
}

func (s *Store) DeleteTask(taskID string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	_, err = db.Exec(`DELETE FROM tasks WHERE repo_root = ? AND task_id = ?`, s.repoRoot, taskID)
	return err
}
