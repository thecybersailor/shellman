package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func OpenSQLiteWithMigrations(path string) (*sql.DB, error) {
	gdb, err := OpenSQLiteGORMWithMigrations(path)
	if err != nil {
		return nil, err
	}
	return gdb.DB()
}

func OpenSQLiteGORMWithMigrations(path string) (*gorm.DB, error) {
	return OpenSQLiteGORMWithMigrationsFromDSN(path)
}

func OpenSQLiteGORMWithMigrationsFromDSN(dsn string) (*gorm.DB, error) {
	gdb, err := openSQLiteDSN(dsn)
	if err != nil {
		return nil, err
	}
	if err := MigrateUp(gdb); err != nil {
		sqlDB, dbErr := gdb.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return gdb, nil
}

func openSQLite(path string) (*gorm.DB, error) {
	return openSQLiteDSN(path)
}

func openSQLiteDSN(dsn string) (*gorm.DB, error) {
	if shouldEnsureSQLiteParentDir(dsn) {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, err
		}
	}
	gdb, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        dsn,
	}, &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := gdb.Exec(`PRAGMA journal_mode=WAL;`).Error; err != nil {
		return nil, err
	}
	if err := gdb.Exec(`PRAGMA busy_timeout=5000;`).Error; err != nil {
		return nil, err
	}
	return gdb, nil
}

func shouldEnsureSQLiteParentDir(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return false
	}
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "mode=memory") || strings.HasPrefix(lower, "file:") {
		return false
	}
	return true
}
