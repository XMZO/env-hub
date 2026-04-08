package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func openDB(dataDir string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s/env-hub.db?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-2000", dataDir)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}
