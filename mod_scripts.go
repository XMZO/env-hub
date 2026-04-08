package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ScriptsModule struct {
	db           *sql.DB
	listScripts  *sql.Stmt
	getScript    *sql.Stmt
	upsertScript *sql.Stmt
	deleteScript *sql.Stmt
}

type Script struct {
	ID        int64
	Path      string
	Content   string
	UpdatedAt time.Time
}

func NewScriptsModule() *ScriptsModule {
	return &ScriptsModule{}
}

func (m *ScriptsModule) Name() string { return "scripts" }

func (m *ScriptsModule) Init(db *sql.DB) error {
	m.db = db

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scripts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			path       TEXT NOT NULL UNIQUE,
			content    TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	var prepErr error
	prep := func(q string) *sql.Stmt {
		if prepErr != nil {
			return nil
		}
		s, e := db.Prepare(q)
		if e != nil {
			prepErr = e
		}
		return s
	}

	m.listScripts = prep("SELECT id, path, content, updated_at FROM scripts ORDER BY path")
	m.getScript = prep("SELECT id, path, content, updated_at FROM scripts WHERE path = ?")
	m.upsertScript = prep("INSERT INTO scripts (path, content, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(path) DO UPDATE SET content = excluded.content, updated_at = CURRENT_TIMESTAMP")
	m.deleteScript = prep("DELETE FROM scripts WHERE id = ?")

	if prepErr != nil {
		return prepErr
	}

	// Seed default /ssh script if not exists
	m.seedDefaults()
	return nil
}

func (m *ScriptsModule) RegisterRoutes(_ *http.ServeMux) {
	// Scripts are served via the scriptFallback handler in main.go,
	// not through explicit route registration.
}

// ServeScript handles a dynamic script request. Returns true if found.
func (m *ScriptsModule) ServeScript(w http.ResponseWriter, path string) bool {
	var s Script
	err := m.getScript.QueryRow(path).Scan(&s.ID, &s.Path, &s.Content, &s.UpdatedAt)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, s.Content)
	return true
}

func (m *ScriptsModule) AdminTemplateName() string { return "mod_scripts.html" }

func (m *ScriptsModule) AdminData() (any, error) {
	return m.list()
}

func (m *ScriptsModule) AdminAction(action string, r *http.Request) error {
	switch action {
	case "save", "new":
		path := strings.TrimSpace(r.FormValue("path"))
		content := r.FormValue("content")
		if path == "" {
			return nil
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		_, err := m.upsertScript.Exec(path, content)
		return err

	case "delete":
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if id > 0 {
			_, err := m.deleteScript.Exec(id)
			return err
		}
	}
	return nil
}

func (m *ScriptsModule) list() ([]Script, error) {
	rows, err := m.listScripts.Query()
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

func (m *ScriptsModule) seedDefaults() {
	var count int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM scripts").Scan(&count); err != nil {
		return
	}
	if count > 0 {
		return
	}

	defaultSSH := `#!/bin/sh
set -eu

KEY_URL="${ENV_HUB_URL:-https://env.moe}/keys/main.pub"
KEY=$(curl -fsSL "$KEY_URL")

if [ -z "$KEY" ]; then
  echo "[env.moe] No key found."
  exit 1
fi

mkdir -p ~/.ssh
chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

if grep -qF "$KEY" ~/.ssh/authorized_keys 2>/dev/null; then
  echo "[env.moe] SSH key already present."
else
  echo "$KEY" >> ~/.ssh/authorized_keys
  echo "[env.moe] SSH key added."
fi
`
	_, _ = m.upsertScript.Exec("/ssh", defaultSSH)
}
