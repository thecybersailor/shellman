package projectstate

import (
	"database/sql"
	"errors"
	dbmodel "shellman/cli/internal/db"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	row := dbmodel.Task{
		TaskID:         input.TaskID,
		RepoRoot:       s.repoRoot,
		ProjectID:      input.ProjectID,
		ParentTaskID:   strPtrOrDefault(input.ParentTaskID, ""),
		Title:          strPtrOrDefault(input.Title, ""),
		CurrentCommand: strPtrOrDefault(input.CurrentCommand, ""),
		Status:         strPtrOrDefault(input.Status, ""),
		SidecarMode:    strPtrOrDefault(input.SidecarMode, SidecarModeAdvisor),
		Description:    strPtrOrDefault(input.Description, ""),
		Flag:           strPtrOrDefault(input.Flag, ""),
		FlagDesc:       strPtrOrDefault(input.FlagDesc, ""),
		FlagReaded:     boolPtrOrDefault(input.FlagReaded, false),
		Checked:        boolPtrOrDefault(input.Checked, false),
		Archived:       boolPtrOrDefault(input.Archived, false),
		CreatedAt:      now,
		LastModified:   now,
	}

	assignments := map[string]any{
		"project_id":    gorm.Expr("CASE WHEN excluded.project_id <> '' THEN excluded.project_id ELSE tasks.project_id END"),
		"last_modified": gorm.Expr("excluded.last_modified"),
	}
	if hasParent {
		assignments["parent_task_id"] = gorm.Expr("excluded.parent_task_id")
	}
	if hasTitle {
		assignments["title"] = gorm.Expr("excluded.title")
	}
	if hasCurrentCommand {
		assignments["current_command"] = gorm.Expr("excluded.current_command")
	}
	if hasStatus {
		assignments["status"] = gorm.Expr("excluded.status")
	}
	if hasSidecarMode {
		assignments["sidecar_mode"] = gorm.Expr("excluded.sidecar_mode")
	}
	if hasDescription {
		assignments["description"] = gorm.Expr("excluded.description")
	}
	if hasFlag {
		assignments["flag"] = gorm.Expr("excluded.flag")
	}
	if hasFlagDesc {
		assignments["flag_desc"] = gorm.Expr("excluded.flag_desc")
	}
	if hasFlagReaded {
		assignments["flag_readed"] = gorm.Expr("excluded.flag_readed")
	}
	if hasChecked {
		assignments["checked"] = gorm.Expr("excluded.checked")
	}
	if hasArchived {
		assignments["archived"] = gorm.Expr("excluded.archived")
	}

	return gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&row).Error
}

func (s *Store) ArchiveCheckedTasksByProject(projectID string) (int64, error) {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	tx := gdb.Model(&dbmodel.Task{}).
		Where("repo_root = ? AND project_id = ? AND checked = ? AND archived = ?", s.repoRoot, projectID, true, false).
		Updates(map[string]any{
			"archived":      true,
			"last_modified": now,
		})
	if tx.Error != nil {
		return 0, tx.Error
	}
	return tx.RowsAffected, nil
}

func (s *Store) BatchUpsertRuntime(input RuntimeBatchUpdate) error {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	return gdb.Transaction(func(tx *gorm.DB) error {
		for _, pane := range input.Panes {
			updatedAt := pane.UpdatedAt
			if updatedAt <= 0 {
				updatedAt = now
			}
			row := dbmodel.PaneRuntime{
				PaneID:         pane.PaneID,
				PaneTarget:     pane.PaneTarget,
				CurrentCommand: pane.CurrentCommand,
				RuntimeStatus:  pane.RuntimeStatus,
				Snapshot:       pane.Snapshot,
				SnapshotHash:   pane.SnapshotHash,
				CursorX:        pane.CursorX,
				CursorY:        pane.CursorY,
				HasCursor:      pane.HasCursor,
				UpdatedAt:      updatedAt,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "pane_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"pane_target":     gorm.Expr("excluded.pane_target"),
					"current_command": gorm.Expr("excluded.current_command"),
					"runtime_status":  gorm.Expr("excluded.runtime_status"),
					"snapshot":        gorm.Expr("excluded.snapshot"),
					"snapshot_hash":   gorm.Expr("excluded.snapshot_hash"),
					"cursor_x":        gorm.Expr("excluded.cursor_x"),
					"cursor_y":        gorm.Expr("excluded.cursor_y"),
					"has_cursor":      gorm.Expr("excluded.has_cursor"),
					"updated_at":      gorm.Expr("excluded.updated_at"),
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}

		for _, task := range input.Tasks {
			updatedAt := task.UpdatedAt
			if updatedAt <= 0 {
				updatedAt = now
			}
			row := dbmodel.TaskRuntime{
				TaskID:         task.TaskID,
				SourcePaneID:   task.SourcePaneID,
				CurrentCommand: task.CurrentCommand,
				RuntimeStatus:  task.RuntimeStatus,
				SnapshotHash:   task.SnapshotHash,
				UpdatedAt:      updatedAt,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "task_id"}},
				DoUpdates: clause.Assignments(map[string]any{
					"source_pane_id":  gorm.Expr("excluded.source_pane_id"),
					"current_command": gorm.Expr("excluded.current_command"),
					"runtime_status":  gorm.Expr("excluded.runtime_status"),
					"snapshot_hash":   gorm.Expr("excluded.snapshot_hash"),
					"updated_at":      gorm.Expr("excluded.updated_at"),
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Model(&dbmodel.Task{}).
				Where("repo_root = ? AND task_id = ?", s.repoRoot, task.TaskID).
				Update("current_command", task.CurrentCommand).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	return gdb.Where("repo_root = ? AND task_id = ?", s.repoRoot, taskID).Delete(&dbmodel.Task{}).Error
}

func strPtrOrDefault(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func boolPtrOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}
