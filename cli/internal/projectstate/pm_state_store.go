package projectstate

import (
	"database/sql"
	dbmodel "shellman/cli/internal/db"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Store) CreatePMSession(projectID, title string) (string, error) {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return "", err
	}
	defer func() { _ = release() }()

	projectID = strings.TrimSpace(projectID)
	title = strings.TrimSpace(title)
	now := time.Now().UTC().UnixMilli()
	sessionID := uuid.NewString()

	row := dbmodel.PMSession{
		SessionID:     sessionID,
		RepoRoot:      s.repoRoot,
		ProjectID:     projectID,
		Title:         title,
		Archived:      false,
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := gdb.Create(&row).Error; err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *Store) ListPMSessionsByProject(projectID string, limit int) ([]PMSessionRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return []PMSessionRecord{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(`
SELECT session_id, project_id, title, archived, last_message_at, created_at, updated_at
FROM pm_sessions
WHERE repo_root = ? AND project_id = ?
ORDER BY last_message_at DESC, updated_at DESC, created_at DESC, session_id DESC
LIMIT ?
`, s.repoRoot, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]PMSessionRecord, 0, limit)
	for rows.Next() {
		var item PMSessionRecord
		if err := rows.Scan(&item.SessionID, &item.ProjectID, &item.Title, &item.Archived, &item.LastMessageAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) InsertPMMessage(sessionID, role, content, status, errText string) (int64, error) {
	gdb, release, err := s.dbGORM()
	if err != nil {
		return 0, err
	}
	defer func() { _ = release() }()

	sessionID = strings.TrimSpace(sessionID)
	role = strings.TrimSpace(role)
	content = strings.TrimSpace(content)
	status = strings.TrimSpace(status)
	errText = strings.TrimSpace(errText)
	if sessionID == "" || role == "" {
		return 0, nil
	}
	if status == "" {
		status = StatusCompleted
	}
	now := time.Now().UTC().UnixMilli()

	var insertedID int64
	err = gdb.Transaction(func(tx *gorm.DB) error {
		row := dbmodel.PMMessage{
			SessionID: sessionID,
			Role:      role,
			Content:   content,
			Status:    status,
			ErrorText: errText,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		insertedID = row.ID
		return tx.Model(&dbmodel.PMSession{}).
			Where("session_id = ? AND repo_root = ?", sessionID, s.repoRoot).
			Updates(map[string]any{"last_message_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return 0, err
	}
	return insertedID, nil
}

func (s *Store) UpdatePMMessage(id int64, content, status, errText string) error {
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
	return gdb.Model(&dbmodel.PMMessage{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"content":    content,
			"status":     status,
			"error_text": errText,
			"updated_at": time.Now().UTC().UnixMilli(),
		}).Error
}

func (s *Store) ListPMMessages(sessionID string, limit int) ([]PMMessageRecord, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return []PMMessageRecord{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`
SELECT id, session_id, role, content, status, error_text, created_at, updated_at
FROM pm_messages
WHERE session_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]PMMessageRecord, 0, limit)
	for rows.Next() {
		var item PMMessageRecord
		if err := rows.Scan(&item.ID, &item.SessionID, &item.Role, &item.Content, &item.Status, &item.ErrorText, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetPMSession(sessionID string) (PMSessionRecord, bool, error) {
	db, release, err := s.db()
	if err != nil {
		return PMSessionRecord{}, false, err
	}
	defer func() { _ = release() }()

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return PMSessionRecord{}, false, nil
	}
	var out PMSessionRecord
	err = db.QueryRow(`
SELECT session_id, project_id, title, archived, last_message_at, created_at, updated_at
FROM pm_sessions
WHERE repo_root = ? AND session_id = ?
`, s.repoRoot, sessionID).Scan(&out.SessionID, &out.ProjectID, &out.Title, &out.Archived, &out.LastMessageAt, &out.CreatedAt, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return PMSessionRecord{}, false, nil
	}
	if err != nil {
		return PMSessionRecord{}, false, err
	}
	return out, true, nil
}
