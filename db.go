package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Key struct {
	ID        int64
	Name      string
	Key       string
	CreatedAt time.Time
}

type Script struct {
	ID        int64
	Path      string
	Content   string
	UpdatedAt time.Time
}

func openDB(dataDir string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s/env-hub.db?_journal_mode=WAL", dataDir)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS keys (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			key        TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS scripts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL UNIQUE,
			content    TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

// --- Keys CRUD ---

func listKeys(db *sql.DB) ([]Key, error) {
	rows, err := db.Query("SELECT id, name, key, created_at FROM keys ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []Key
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.Key, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func addKey(db *sql.DB, name, key string) error {
	_, err := db.Exec("INSERT INTO keys (name, key) VALUES (?, ?)", name, key)
	return err
}

func deleteKey(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM keys WHERE id = ?", id)
	return err
}

func getAllKeysText(db *sql.DB) (string, error) {
	rows, err := db.Query("SELECT key FROM keys ORDER BY id")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var result string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return "", err
		}
		if result != "" {
			result += "\n"
		}
		result += k
	}
	return result, rows.Err()
}

// --- Scripts CRUD ---

func listScripts(db *sql.DB) ([]Script, error) {
	rows, err := db.Query("SELECT id, path, content, updated_at FROM scripts ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scripts []Script
	for rows.Next() {
		var s Script
		if err := rows.Scan(&s.ID, &s.Path, &s.Content, &s.UpdatedAt); err != nil {
			return nil, err
		}
		scripts = append(scripts, s)
	}
	return scripts, rows.Err()
}

func getScript(db *sql.DB, path string) (Script, error) {
	var s Script
	err := db.QueryRow("SELECT id, path, content, updated_at FROM scripts WHERE path = ?", path).
		Scan(&s.ID, &s.Path, &s.Content, &s.UpdatedAt)
	return s, err
}

func upsertScript(db *sql.DB, path, content string) error {
	_, err := db.Exec(`
		INSERT INTO scripts (path, content, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET content = excluded.content, updated_at = CURRENT_TIMESTAMP
	`, path, content)
	return err
}

func deleteScript(db *sql.DB, id int64) error {
	_, err := db.Exec("DELETE FROM scripts WHERE id = ?", id)
	return err
}
