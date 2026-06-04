package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) TableExists(name string) bool {
	var count int
	_ = d.sql.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
		name,
	).Scan(&count)
	return count > 0
}

func (d *DB) migrate() error {
	if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename   TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var count int
		if err := d.sql.QueryRow(
			"SELECT COUNT(*) FROM schema_migrations WHERE filename = ?",
			entry.Name(),
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if count > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if _, err := d.sql.Exec(string(content)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}

		if _, err := d.sql.Exec(
			"INSERT INTO schema_migrations (filename, applied_at) VALUES (?, ?)",
			entry.Name(),
			timeToStr(time.Now()),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
	}

	return nil
}

func timeToStr(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func strToTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
