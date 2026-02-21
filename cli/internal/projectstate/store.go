package projectstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"shellman/cli/internal/db"
)

var (
	globalDBMu   sync.Mutex
	globalDB     *sql.DB
	globalDBPath string
)

type Store struct {
	repoRoot string
}

func NewStore(repoRoot string) *Store {
	return &Store{repoRoot: repoRoot}
}

// InitGlobalDB sets the process-wide sqlite file used by project state storage.
func InitGlobalDB(dbPath string) error {
	globalDBMu.Lock()
	defer globalDBMu.Unlock()

	if dbPath == "" {
		return errors.New("db path is required")
	}
	if globalDB != nil && globalDBPath == dbPath {
		return nil
	}
	if globalDB != nil {
		_ = globalDB.Close()
		globalDB = nil
	}
	db, err := openDB(dbPath)
	if err != nil {
		return err
	}
	globalDB = db
	globalDBPath = dbPath
	return nil
}

// GlobalDB returns the process-wide DB. Caller must not close it.
func GlobalDB() (*sql.DB, error) {
	globalDBMu.Lock()
	db := globalDB
	globalDBMu.Unlock()
	if db == nil {
		return nil, errors.New("global DB not initialized: call InitGlobalDB first")
	}
	return db, nil
}

func (s *Store) SavePanes(index PanesIndex) error {
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return s.saveJSON("panes_json", raw)
}

func (s *Store) LoadPanes() (PanesIndex, error) {
	var index PanesIndex
	raw, err := s.loadJSON("panes_json")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return PanesIndex{}, nil
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if index == nil {
		index = PanesIndex{}
	}
	return index, nil
}

func (s *Store) SavePaneSnapshots(index PaneSnapshotsIndex) error {
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
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
INSERT INTO legacy_state(repo_root, state_key, state_json, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(repo_root, state_key) DO UPDATE SET
  state_json = excluded.state_json,
  updated_at = excluded.updated_at;
`, s.repoRoot, "pane_snapshots_json", string(raw), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LoadPaneSnapshots() (PaneSnapshotsIndex, error) {
	var index PaneSnapshotsIndex
	raw, err := s.loadJSON("pane_snapshots_json")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return PaneSnapshotsIndex{}, nil
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if index == nil {
		index = PaneSnapshotsIndex{}
	}
	return index, nil
}

func (s *Store) saveJSON(column string, raw []byte) error {
	db, release, err := s.db()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	switch column {
	case "panes_json", "pane_snapshots_json":
	default:
		return errors.New("unsupported column")
	}

	_, err = db.Exec(`
INSERT INTO legacy_state(repo_root, state_key, state_json, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(repo_root, state_key) DO UPDATE SET
  state_json = excluded.state_json,
  updated_at = excluded.updated_at;
`, s.repoRoot, column, string(raw), now)
	return err
}

func (s *Store) loadJSON(column string) ([]byte, error) {
	db, release, err := s.db()
	if err != nil {
		return nil, err
	}
	defer func() { _ = release() }()

	switch column {
	case "panes_json", "pane_snapshots_json":
	default:
		return nil, errors.New("unsupported column")
	}

	var raw string
	err = db.QueryRow(`SELECT state_json FROM legacy_state WHERE repo_root = ? AND state_key = ?`, s.repoRoot, column).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	return []byte(raw), nil
}

func (s *Store) db() (*sql.DB, func() error, error) {
	globalDBMu.Lock()
	db := globalDB
	globalDBMu.Unlock()
	if db == nil {
		return nil, nil, errors.New("global DB not initialized: call InitGlobalDB first")
	}
	return db, func() error { return nil }, nil
}

func openDB(path string) (*sql.DB, error) {
	return db.OpenSQLiteWithMigrations(path)
}
