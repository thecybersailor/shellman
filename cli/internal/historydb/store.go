package historydb

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Entry struct {
	Path          string
	FirstAccessed time.Time
	LastAccessed  time.Time
	AccessCount   int
}

type Store struct {
	db *sql.DB
}

// NewStore uses the shared global DB. Caller must not close the db.
func NewStore(db *sql.DB) (*Store, error) {
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
	_, err := s.db.Exec(`
INSERT INTO dir_history(path, first_accessed_at, last_accessed_at, access_count)
VALUES (?, ?, ?, 1)
ON CONFLICT(path) DO UPDATE SET
  last_accessed_at = excluded.last_accessed_at,
  access_count = dir_history.access_count + 1;
`, p, now, now)
	return err
}

func (s *Store) List(limit int) ([]Entry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("history store is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
SELECT path, first_accessed_at, last_accessed_at, access_count
FROM dir_history
ORDER BY last_accessed_at DESC
LIMIT ?;
`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries := make([]Entry, 0, limit)
	for rows.Next() {
		var (
			path  string
			first int64
			last  int64
			count int
		)
		if err := rows.Scan(&path, &first, &last, &count); err != nil {
			return nil, err
		}
		entries = append(entries, Entry{
			Path:          path,
			FirstAccessed: time.Unix(first, 0).UTC(),
			LastAccessed:  time.Unix(last, 0).UTC(),
			AccessCount:   count,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) Clear() error {
	if s == nil || s.db == nil {
		return errors.New("history store is not initialized")
	}
	_, err := s.db.Exec(`DELETE FROM dir_history`)
	return err
}

// Close is a no-op; DB is process-wide and must not be closed by the store.
func (s *Store) Close() error {
	return nil
}
