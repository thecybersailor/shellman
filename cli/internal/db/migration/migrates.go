package migration

import (
	"fmt"

	"gorm.io/gorm"
)

type step struct {
	name string
	run  func(*Migration) error
}

var steps []step

// Migration is passed to each migration step. DB is set by RunAll.
type Migration struct {
	DB   *gorm.DB
	logs []string
}

func (m *Migration) Log(v ...interface{}) {
	m.logs = append(m.logs, fmt.Sprint(v...))
}

// RunAll runs all registered migrations in order. Used for data/behavior one-shots; schema is synced via db.SyncSchema.
func RunAll(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	ctx := &Migration{DB: db}
	for _, s := range steps {
		ctx.logs = nil
		if err := s.run(ctx); err != nil {
			return fmt.Errorf("migration %s failed: %w", s.name, err)
		}
	}
	return nil
}
