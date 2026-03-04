package projectstate

import (
	"database/sql"
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
		TaskID:        task.TaskID,
		RepoRoot:      s.repoRoot,
		ProjectID:     task.ProjectID,
		ParentTaskID:  task.ParentTaskID,
		Title:         task.Title,
		ActiveAdapter: strings.TrimSpace(task.ActiveAdapter),
		Status:        status,
		SidecarMode:   sidecarMode,
		TaskRole:      taskRole,
		CreatedAt:     now,
		LastModified:  now,
	}
	return gdb.Create(&row).Error
}

func (s *Store) TryMarkTaskAutoProgressObserved(task TaskRecord, observedAt int64) (bool, error) {
	if observedAt <= 0 {
		return false, nil
	}
	gdb, release, err := s.dbGORM()
	if err != nil {
		return false, err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	sidecarMode := strings.TrimSpace(task.SidecarMode)
	if sidecarMode == "" {
		sidecarMode = SidecarModeAdvisor
	}
	taskRole := strings.TrimSpace(task.TaskRole)
	if taskRole == "" {
		taskRole = TaskRoleFull
	}
	status := strings.TrimSpace(task.Status)
	if status == "" {
		status = StatusPending
	}
	row := dbmodel.Task{
		TaskID:             strings.TrimSpace(task.TaskID),
		RepoRoot:           s.repoRoot,
		ProjectID:          strings.TrimSpace(task.ProjectID),
		ParentTaskID:       strings.TrimSpace(task.ParentTaskID),
		Title:              task.Title,
		Status:             status,
		SidecarMode:        sidecarMode,
		TaskRole:           taskRole,
		CreatedAt:          now,
		LastModified:       now,
		LastAutoProgressAt: observedAt,
	}
	result := gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"project_id":            gorm.Expr("excluded.project_id"),
			"parent_task_id":        gorm.Expr("excluded.parent_task_id"),
			"title":                 gorm.Expr("excluded.title"),
			"status":                gorm.Expr("excluded.status"),
			"sidecar_mode":          gorm.Expr("excluded.sidecar_mode"),
			"task_role":             gorm.Expr("excluded.task_role"),
			"last_modified":         gorm.Expr("excluded.last_modified"),
			"last_auto_progress_at": gorm.Expr("excluded.last_auto_progress_at"),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "tasks.last_auto_progress_at < excluded.last_auto_progress_at"},
		}},
	}).Create(&row)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

func (s *Store) InsertTaskNote(taskID, notes, flag string) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	row := dbmodel.Note{
		TaskID:    taskID,
		CreatedAt: now,
		Flag:      strings.TrimSpace(flag),
		Notes:     strings.TrimSpace(notes),
	}
	return gdb.Create(&row).Error
}

func (s *Store) ListTaskNotes(taskID string, limit int) ([]TaskNoteRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`
SELECT task_id, created_at, flag, notes
FROM notes
WHERE task_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]TaskNoteRecord, 0, limit)
	for rows.Next() {
		var rec TaskNoteRecord
		if err := rows.Scan(&rec.TaskID, &rec.CreatedAt, &rec.Flag, &rec.Notes); err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) InsertTaskMessage(taskID, role, content, status, errText string) (int64, error) {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	statusText := strings.TrimSpace(status)
	if statusText == "" {
		statusText = StatusCompleted
	}
	row := dbmodel.TaskMessage{
		TaskID:    strings.TrimSpace(taskID),
		Role:      strings.TrimSpace(role),
		Content:   content,
		Status:    statusText,
		ErrorText: strings.TrimSpace(errText),
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

	updates := map[string]any{
		"updated_at": time.Now().UTC().Unix(),
	}
	if content != "" {
		updates["content"] = content
	}
	if strings.TrimSpace(status) != "" {
		updates["status"] = strings.TrimSpace(status)
	}
	if errText != "" || strings.TrimSpace(status) == StatusFailed {
		updates["error_text"] = strings.TrimSpace(errText)
	}

	return gdb.Model(&dbmodel.TaskMessage{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (s *Store) ListTaskMessages(taskID string, limit int) ([]TaskMessageRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`
SELECT id, task_id, role, content, status, error_text, created_at, updated_at
FROM (
	SELECT id, task_id, role, content, status, error_text, created_at, updated_at
	FROM task_messages
	WHERE task_id = ?
	ORDER BY created_at DESC, id DESC
	LIMIT ?
)
ORDER BY created_at ASC, id ASC
`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]TaskMessageRecord, 0, limit)
	for rows.Next() {
		var rec TaskMessageRecord
		if err := rows.Scan(&rec.ID, &rec.TaskID, &rec.Role, &rec.Content, &rec.Status, &rec.ErrorText, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanTaskRecordRows(rows *sql.Rows) ([]TaskRecordRow, error) {
	items := make([]TaskRecordRow, 0, 32)
	for rows.Next() {
		var row TaskRecordRow
		if err := rows.Scan(
			&row.TaskID,
			&row.ProjectID,
			&row.ParentTaskID,
			&row.Title,
			&row.CurrentCommand,
			&row.ActiveAdapter,
			&row.Status,
			&row.SidecarMode,
			&row.TaskRole,
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
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
