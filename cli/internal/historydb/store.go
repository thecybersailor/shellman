package historydb

import (
	"errors"
	"strings"
	"time"

	dbmodel "shellman/cli/internal/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Entry struct {
	Path          string
	FirstAccessed time.Time
	LastAccessed  time.Time
	AccessCount   int
}

type Store struct {
	db *gorm.DB
}

// NewStore uses the shared global DB. Caller must not close the db.
func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	return &Store{db: db}, nil
}

func (s *Store) Upsert(path string) error {
	if s == nil || s.db == nil {
		return errors.New("history store is not initialized")
	}
	p := strings.TrimSpace(path)
	if p == "" {
		return errors.New("path is required")
	}
	now := time.Now().UTC().Unix()
	row := dbmodel.DirHistory{
		Path:            p,
		FirstAccessedAt: now,
		LastAccessedAt:  now,
		AccessCount:     1,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_accessed_at": now,
			"access_count":     gorm.Expr("dir_history.access_count + 1"),
		}),
	}).Create(&row).Error
}

func (s *Store) List(limit int) ([]Entry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("history store is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}
	rows := make([]dbmodel.DirHistory, 0, limit)
	if err := s.db.Order("last_accessed_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, limit)
	for _, row := range rows {
		entries = append(entries, Entry{
			Path:          row.Path,
			FirstAccessed: time.Unix(row.FirstAccessedAt, 0).UTC(),
			LastAccessed:  time.Unix(row.LastAccessedAt, 0).UTC(),
			AccessCount:   row.AccessCount,
		})
	}
	return entries, nil
}

func (s *Store) Clear() error {
	if s == nil || s.db == nil {
		return errors.New("history store is not initialized")
	}
	return s.db.Where("1 = 1").Delete(&dbmodel.DirHistory{}).Error
}

// Close is a no-op; DB is process-wide and must not be closed by the store.
func (s *Store) Close() error {
	return nil
}
