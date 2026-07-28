package main

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func openDB(dataDir string) (*sql.DB, error) {
	// modernc.org/sqlite only understands `_pragma=name(value)` query
	// parameters; mattn-style `_journal_mode=...` params are silently ignored.
	dsn := fmt.Sprintf("file:%s/env-hub.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-2000)", filepath.ToSlash(dataDir))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}
