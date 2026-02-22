package projectstate

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"shellman/cli/internal/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	globalDBMu   sync.Mutex
	globalDB     *sql.DB
	globalDBGORM *gorm.DB
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
	return InitGlobalDBWithDSN(dbPath)
}

// InitGlobalDBWithDSN sets the process-wide sqlite dsn used by project state storage.
func InitGlobalDBWithDSN(dsn string) error {
	globalDBMu.Lock()
	defer globalDBMu.Unlock()

	if dsn == "" {
		return errors.New("db path is required")
	}
	if globalDBGORM != nil && globalDBPath == dsn {
		return nil
	}
	if globalDB != nil {
		_ = globalDB.Close()
		globalDB = nil
	}
	globalDBGORM = nil

	gdb, err := openDBGORMFromDSN(dsn)
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}

	globalDBGORM = gdb
	globalDB = sqlDB
	globalDBPath = dsn
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

// GlobalDBGORM returns the process-wide GORM DB. Caller must not close it.
func GlobalDBGORM() (*gorm.DB, error) {
	globalDBMu.Lock()
	gdb := globalDBGORM
	globalDBMu.Unlock()
	if gdb == nil {
		return nil, errors.New("global DB not initialized: call InitGlobalDB first")
	}
	return gdb, nil
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
	gdb, release, err := s.dbGORM()
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	now := time.Now().UTC().Unix()
	return gdb.Transaction(func(tx *gorm.DB) error {
		row := db.LegacyState{
			RepoRoot:  s.repoRoot,
			StateKey:  "pane_snapshots_json",
			StateJSON: string(raw),
			UpdatedAt: now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "repo_root"}, {Name: "state_key"}},
			DoUpdates: clause.Assignments(map[string]any{
				"state_json": row.StateJSON,
				"updated_at": row.UpdatedAt,
			}),
		}).Create(&row).Error
	})
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
	gdb, release, err := s.dbGORM()
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

	row := db.LegacyState{
		RepoRoot:  s.repoRoot,
		StateKey:  column,
		StateJSON: string(raw),
		UpdatedAt: now,
	}
	return gdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "repo_root"}, {Name: "state_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"state_json": row.StateJSON,
			"updated_at": row.UpdatedAt,
		}),
	}).Create(&row).Error
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

func (s *Store) dbGORM() (*gorm.DB, func() error, error) {
	globalDBMu.Lock()
	gdb := globalDBGORM
	globalDBMu.Unlock()
	if gdb == nil {
		return nil, nil, errors.New("global DB not initialized: call InitGlobalDB first")
	}
	return gdb, func() error { return nil }, nil
}

func openDBGORMFromDSN(dsn string) (*gorm.DB, error) {
	return db.OpenSQLiteGORMWithMigrationsFromDSN(dsn)
}
