package projectstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	dbmodel "shellman/cli/internal/db"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) InsertTask(task TaskRecord) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	status := task.Status
	if status == "" {
		status = StatusPending
	}
	sidecarMode := strings.TrimSpace(task.SidecarMode)
	if sidecarMode == "" {
		sidecarMode = SidecarModeAdvisor
	}
	taskRole := strings.TrimSpace(task.TaskRole)
	if taskRole == "" {
		taskRole = TaskRoleFull
	}
	row := dbmodel.Task{
		TaskID:       task.TaskID,
		RepoRoot:     s.repoRoot,
		ProjectID:    task.ProjectID,
		ParentTaskID: task.ParentTaskID,
		Title:        task.Title,
		Status:       status,
		SidecarMode:  sidecarMode,
		TaskRole:     taskRole,
		CreatedAt:    now,
		LastModified: now,
	}
	return gdb.Create(&row).Error
}

func (s *Store) InsertRun(run RunRecord) error {
	gdb, release, err := s.dbGORM()
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
	row := dbmodel.TaskRun{
		RunID:       run.RunID,
		TaskID:      run.TaskID,
		RunStatus:   status,
		StartedAt:   startedAt,
		CompletedAt: run.CompletedAt,
		UpdatedAt:   now,
		LastError:   run.LastError,
	}
	return gdb.Create(&row).Error
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
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	status := binding.BindingStatus
	if status == "" {
		status = BindingStatusLive
	}
	row := dbmodel.RunBinding{
		RunID:            binding.RunID,
		ServerInstanceID: binding.ServerInstanceID,
		PaneID:           binding.PaneID,
		PaneTarget:       binding.PaneTarget,
		BindingStatus:    status,
		StaleReason:      binding.StaleReason,
		UpdatedAt:        now,
	}
	return gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"server_instance_id": gorm.Expr("excluded.server_instance_id"),
			"pane_id":            gorm.Expr("excluded.pane_id"),
			"pane_target":        gorm.Expr("excluded.pane_target"),
			"binding_status":     gorm.Expr("excluded.binding_status"),
			"stale_reason":       gorm.Expr("excluded.stale_reason"),
			"updated_at":         gorm.Expr("excluded.updated_at"),
		}),
	}).Create(&row).Error
}

func (s *Store) MarkBindingsStaleByServer(serverInstanceID, reason string) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	return gdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dbmodel.RunBinding{}).
			Where("server_instance_id = ? AND binding_status = ?", serverInstanceID, BindingStatusLive).
			Updates(map[string]any{
				"binding_status": BindingStatusStale,
				"stale_reason":   reason,
				"updated_at":     now,
			}).Error; err != nil {
			return err
		}

		sub := tx.Model(&dbmodel.RunBinding{}).
			Select("run_id").
			Where("server_instance_id = ? AND binding_status = ?", serverInstanceID, BindingStatusStale)
		return tx.Model(&dbmodel.TaskRun{}).
			Where("run_id IN (?)", sub).
			Updates(map[string]any{
				"run_status": RunStatusNeedsRebind,
				"updated_at": now,
			}).Error
	})
}

func (s *Store) AppendRunEvent(runID, eventType string, payload map[string]any) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	row := dbmodel.RunEvent{
		RunID:       runID,
		EventType:   eventType,
		PayloadJSON: string(raw),
		CreatedAt:   time.Now().UTC().Unix(),
	}
	return gdb.Create(&row).Error
}

func (s *Store) InsertCompletionInbox(runID, requestID, summary, source string) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	row := dbmodel.CompletionInbox{
		RunID:     runID,
		RequestID: requestID,
		Summary:   summary,
		Source:    source,
		CreatedAt: time.Now().UTC().Unix(),
	}
	err = gdb.Create(&row).Error
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
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	return gdb.Model(&dbmodel.TaskRun{}).
		Where("run_id = ?", runID).
		Updates(map[string]any{
			"run_status":   RunStatusCompleted,
			"completed_at": now,
			"updated_at":   now,
		}).Error
}

func (s *Store) SetRunStatus(runID, status string) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	return gdb.Model(&dbmodel.TaskRun{}).
		Where("run_id = ?", runID).
		Updates(map[string]any{
			"run_status": status,
			"updated_at": time.Now().UTC().Unix(),
		}).Error
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
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Unix()
	row := dbmodel.ActionOutbox{
		RunID:       runID,
		ActionType:  actionType,
		PayloadJSON: string(raw),
		Status:      "pending",
		RetryCount:  0,
		NextRetryAt: 0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return gdb.Create(&row).Error
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
	gdb, release, err := s.dbGORM()
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
	row := dbmodel.Task{
		TaskID:             taskID,
		RepoRoot:           s.repoRoot,
		ProjectID:          strings.TrimSpace(task.ProjectID),
		ParentTaskID:       strings.TrimSpace(task.ParentTaskID),
		Title:              strings.TrimSpace(task.Title),
		Status:             status,
		CreatedAt:          now,
		LastModified:       now,
		LastAutoProgressAt: observedAt,
	}
	tx := gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_auto_progress_at": gorm.Expr("excluded.last_auto_progress_at"),
			"last_modified":         gorm.Expr("excluded.last_modified"),
		}),
		Where: clause.Where{
			Exprs: []clause.Expression{
				clause.Expr{SQL: "tasks.last_auto_progress_at < excluded.last_auto_progress_at"},
			},
		},
	}).Create(&row)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (s *Store) InsertTaskNote(taskID, notes, flag string) error {
	gdb, release, err := s.dbGORM()
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
	row := dbmodel.Note{
		TaskID:    taskID,
		CreatedAt: time.Now().UTC().Unix(),
		Flag:      flag,
		Notes:     notes,
	}
	return gdb.Create(&row).Error
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
	gdb, release, err := s.dbGORM()
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
	row := dbmodel.TaskMessage{
		TaskID:    taskID,
		Role:      role,
		Content:   content,
		Status:    status,
		ErrorText: errText,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := gdb.Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *Store) UpdateTaskMessage(id int64, content, status, errText string) error {
	gdb, release, err := s.dbGORM()
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
	return gdb.Model(&dbmodel.TaskMessage{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"content":    content,
			"status":     status,
			"error_text": errText,
			"updated_at": time.Now().UTC().Unix(),
		}).Error
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
