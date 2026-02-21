package db

import (
	"database/sql"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func OpenSQLiteWithMigrations(path string) (*sql.DB, error) {
	gdb, err := openSQLite(path)
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
	return sqlDB, nil
}

func openSQLite(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(sqlite.Dialector{
		DriverName: "sqlite",
		DSN:        path,
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
