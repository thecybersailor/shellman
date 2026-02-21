package projectstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s *Store) InsertTask(task TaskRecord) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	status := task.Status
	if status == "" {
		status = StatusPending
	}
	_, err = db.Exec(`
INSERT INTO tasks(task_id, repo_root, project_id, parent_task_id, title, status, created_at, last_modified)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, task.TaskID, s.repoRoot, task.ProjectID, task.ParentTaskID, task.Title, status, now, now)
	return err
}

func (s *Store) InsertRun(run RunRecord) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	status := run.RunStatus
	if status == "" {
		status = RunStatusRunning
	}
	startedAt := run.StartedAt
	if startedAt == 0 {
		startedAt = now
	}
	_, err = db.Exec(`
INSERT INTO task_runs(run_id, task_id, run_status, started_at, completed_at, updated_at, last_error)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, run.RunID, run.TaskID, status, startedAt, run.CompletedAt, now, run.LastError)
	return err
}

func (s *Store) GetRun(runID string) (RunRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return RunRecord{}, err
	}
	defer func() { _ = release() }()

	var run RunRecord
	err = db.QueryRow(`
SELECT run_id, task_id, run_status, started_at, completed_at, updated_at, last_error
FROM task_runs
WHERE run_id = ?
`, runID).Scan(&run.RunID, &run.TaskID, &run.RunStatus, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt, &run.LastError)
	if err != nil {
		return RunRecord{}, err
	}
	return run, nil
}

func (s *Store) UpsertRunBinding(binding RunBinding) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	status := binding.BindingStatus
	if status == "" {
		status = BindingStatusLive
	}
	_, err = db.Exec(`
INSERT INTO run_bindings(run_id, server_instance_id, pane_id, pane_target, binding_status, stale_reason, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
  server_instance_id = excluded.server_instance_id,
  pane_id = excluded.pane_id,
  pane_target = excluded.pane_target,
  binding_status = excluded.binding_status,
  stale_reason = excluded.stale_reason,
  updated_at = excluded.updated_at
`, binding.RunID, binding.ServerInstanceID, binding.PaneID, binding.PaneTarget, status, binding.StaleReason, now)
	return err
}

func (s *Store) MarkBindingsStaleByServer(serverInstanceID, reason string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
UPDATE run_bindings
SET binding_status = ?, stale_reason = ?, updated_at = ?
WHERE server_instance_id = ? AND binding_status = ?
`, BindingStatusStale, reason, now, serverInstanceID, BindingStatusLive); err != nil {
		return err
	}

	if _, err := tx.Exec(`
UPDATE task_runs
SET run_status = ?, updated_at = ?
WHERE run_id IN (
  SELECT run_id FROM run_bindings WHERE server_instance_id = ? AND binding_status = ?
)
`, RunStatusNeedsRebind, now, serverInstanceID, BindingStatusStale); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) AppendRunEvent(runID, eventType string, payload map[string]any) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
INSERT INTO run_events(run_id, event_type, payload_json, created_at)
VALUES (?, ?, ?, ?)
`, runID, eventType, string(raw), time.Now().UTC().Unix())
	return err
}

func (s *Store) InsertCompletionInbox(runID, requestID, summary, source string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	_, err = db.Exec(`
INSERT INTO completion_inbox(run_id, request_id, summary, source, created_at)
VALUES (?, ?, ?, ?, ?)
`, runID, requestID, summary, source, time.Now().UTC().Unix())
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrDuplicateInboxRequest
		}
		return err
	}
	return nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func (s *Store) GetLiveBindingByRunID(runID string) (RunBinding, bool, error) {
	db, release, err := s.db()
	if err != nil {
		return RunBinding{}, false, err
	}
	defer func() { _ = release() }()

	var binding RunBinding
	err = db.QueryRow(`
SELECT run_id, server_instance_id, pane_id, pane_target, binding_status, stale_reason
FROM run_bindings
WHERE run_id = ? AND binding_status = ?
`, runID, BindingStatusLive).Scan(
		&binding.RunID,
		&binding.ServerInstanceID,
		&binding.PaneID,
		&binding.PaneTarget,
		&binding.BindingStatus,
		&binding.StaleReason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RunBinding{}, false, nil
	}
	if err != nil {
		return RunBinding{}, false, err
	}
	return binding, true, nil
}

func (s *Store) GetBindingByRunID(runID string) (RunBinding, bool, error) {
	db, release, err := s.db()
	if err != nil {
		return RunBinding{}, false, err
	}
	defer func() { _ = release() }()

	var binding RunBinding
	err = db.QueryRow(`
SELECT run_id, server_instance_id, pane_id, pane_target, binding_status, stale_reason
FROM run_bindings
WHERE run_id = ?
`, runID).Scan(
		&binding.RunID,
		&binding.ServerInstanceID,
		&binding.PaneID,
		&binding.PaneTarget,
		&binding.BindingStatus,
		&binding.StaleReason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RunBinding{}, false, nil
	}
	if err != nil {
		return RunBinding{}, false, err
	}
	return binding, true, nil
}

func (s *Store) MarkRunCompleted(runID string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	_, err = db.Exec(`
UPDATE task_runs
SET run_status = ?, completed_at = ?, updated_at = ?
WHERE run_id = ?
`, RunStatusCompleted, now, now, runID)
	return err
}

func (s *Store) SetRunStatus(runID, status string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	_, err = db.Exec(`
UPDATE task_runs
SET run_status = ?, updated_at = ?
WHERE run_id = ?
`, status, time.Now().UTC().Unix(), runID)
	return err
}

func (s *Store) FindLiveRunningRunByPaneTarget(paneTarget string) (RunRecord, bool, error) {
	db, release, err := s.db()
	if err != nil {
		return RunRecord{}, false, err
	}
	defer func() { _ = release() }()

	var run RunRecord
	err = db.QueryRow(`
SELECT tr.run_id, tr.task_id, tr.run_status, tr.started_at, tr.completed_at, tr.updated_at, tr.last_error
FROM task_runs tr
JOIN run_bindings rb ON rb.run_id = tr.run_id
WHERE rb.binding_status = ? AND tr.run_status = ? AND (rb.pane_target = ? OR rb.pane_id = ?)
ORDER BY tr.updated_at DESC
LIMIT 1
`, BindingStatusLive, RunStatusRunning, paneTarget, paneTarget).Scan(
		&run.RunID,
		&run.TaskID,
		&run.RunStatus,
		&run.StartedAt,
		&run.CompletedAt,
		&run.UpdatedAt,
		&run.LastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RunRecord{}, false, nil
	}
	if err != nil {
		return RunRecord{}, false, err
	}
	return run, true, nil
}

func (s *Store) ListRunCandidatesByPaneTarget(paneTarget string, limit int) ([]RunLookupCandidate, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	paneTarget = strings.TrimSpace(paneTarget)
	if paneTarget == "" {
		return []RunLookupCandidate{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := db.Query(`
SELECT tr.run_id, tr.task_id, tr.run_status, tr.updated_at, rb.pane_id, rb.pane_target, rb.binding_status, rb.stale_reason, rb.server_instance_id, rb.updated_at
FROM run_bindings rb
JOIN task_runs tr ON tr.run_id = rb.run_id
WHERE rb.pane_target = ? OR rb.pane_id = ?
ORDER BY tr.updated_at DESC, rb.updated_at DESC
LIMIT ?
`, paneTarget, paneTarget, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]RunLookupCandidate, 0, limit)
	for rows.Next() {
		var item RunLookupCandidate
		if err := rows.Scan(
			&item.RunID,
			&item.TaskID,
			&item.RunStatus,
			&item.RunUpdatedAt,
			&item.PaneID,
			&item.PaneTarget,
			&item.BindingStatus,
			&item.StaleReason,
			&item.ServerInstanceID,
			&item.BindingUpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) EnqueueRunAction(runID, actionType string, payload map[string]any) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	_, err = db.Exec(`
INSERT INTO action_outbox(run_id, action_type, payload_json, status, retry_count, next_retry_at, created_at, updated_at)
VALUES (?, ?, ?, 'pending', 0, 0, ?, ?)
`, runID, actionType, string(raw), now, now)
	return err
}

func (s *Store) CountOutboxByRunID(runID string) (int, error) {
	db, release, err := s.db()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM action_outbox WHERE run_id = ?`, runID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountRunEventsByType(runID, eventType string) (int, error) {
	db, release, err := s.db()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM run_events WHERE run_id = ? AND event_type = ?`, runID, eventType).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) TryMarkTaskAutoProgressObserved(task TaskRecord, observedAt int64) (bool, error) {
	db, release, err := s.db()
	if err != nil {
		return false, err
	}
	defer func() { _ = release() }()

	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" || observedAt <= 0 {
		return false, nil
	}
	status := strings.TrimSpace(task.Status)
	if status == "" {
		status = StatusPending
	}
	now := time.Now().UTC().Unix()
	res, err := db.Exec(`
INSERT INTO tasks(task_id, repo_root, project_id, parent_task_id, title, status, created_at, last_modified, last_auto_progress_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
  last_auto_progress_at = excluded.last_auto_progress_at,
  last_modified = excluded.last_modified
WHERE tasks.last_auto_progress_at < excluded.last_auto_progress_at
`, taskID, s.repoRoot, strings.TrimSpace(task.ProjectID), strings.TrimSpace(task.ParentTaskID), strings.TrimSpace(task.Title), status, now, now, observedAt)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) InsertTaskNote(taskID, notes, flag string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	taskID = strings.TrimSpace(taskID)
	notes = strings.TrimSpace(notes)
	flag = strings.TrimSpace(flag)
	if taskID == "" || notes == "" {
		return nil
	}
	_, err = db.Exec(`
INSERT INTO notes(task_id, created_at, flag, notes)
VALUES (?, ?, ?, ?)
`, taskID, time.Now().UTC().Unix(), flag, notes)
	return err
}

func (s *Store) ListTaskNotes(taskID string, limit int) ([]TaskNoteRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return []TaskNoteRecord{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`
SELECT task_id, created_at, flag, notes
FROM notes
WHERE task_id = ?
ORDER BY created_at DESC
LIMIT ?
`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]TaskNoteRecord, 0, limit)
	for rows.Next() {
		var item TaskNoteRecord
		if err := rows.Scan(&item.TaskID, &item.CreatedAt, &item.Flag, &item.Notes); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) InsertTaskMessage(taskID, role, content, status, errText string) (int64, error) {
	db, release, err := s.db()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	taskID = strings.TrimSpace(taskID)
	role = strings.TrimSpace(role)
	content = strings.TrimSpace(content)
	status = strings.TrimSpace(status)
	errText = strings.TrimSpace(errText)
	if taskID == "" || role == "" {
		return 0, nil
	}
	if status == "" {
		status = StatusCompleted
	}
	now := time.Now().UTC().Unix()
	res, err := db.Exec(`
INSERT INTO task_messages(task_id, role, content, status, error_text, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, taskID, role, content, status, errText, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateTaskMessage(id int64, content, status, errText string) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	if id <= 0 {
		return nil
	}
	content = strings.TrimSpace(content)
	status = strings.TrimSpace(status)
	errText = strings.TrimSpace(errText)
	if status == "" {
		status = StatusCompleted
	}
	_, err = db.Exec(`
UPDATE task_messages
SET content = ?, status = ?, error_text = ?, updated_at = ?
WHERE id = ?
`, content, status, errText, time.Now().UTC().Unix(), id)
	return err
}

func (s *Store) ListTaskMessages(taskID string, limit int) ([]TaskMessageRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return []TaskMessageRecord{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`
SELECT id, task_id, role, content, status, error_text, created_at, updated_at
FROM task_messages
WHERE task_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?
`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]TaskMessageRecord, 0, limit)
	for rows.Next() {
		var item TaskMessageRecord
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Role, &item.Content, &item.Status, &item.ErrorText, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
