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
	ID          int64
	Path        string
	Description string
	Content     string
	UpdatedAt   time.Time
}

func NewScriptsModule() *ScriptsModule {
	return &ScriptsModule{}
}

func (m *ScriptsModule) Name() string { return "scripts" }

func (m *ScriptsModule) Init(db *sql.DB) error {
	m.db = db

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scripts (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			path        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		// Migration: add description column to existing table
		db.Exec("ALTER TABLE scripts ADD COLUMN description TEXT NOT NULL DEFAULT ''")
		err = nil
	}
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

	m.listScripts = prep("SELECT id, path, description, content, updated_at FROM scripts ORDER BY path")
	m.getScript = prep("SELECT id, path, description, content, updated_at FROM scripts WHERE path = ?")
	m.upsertScript = prep("INSERT INTO scripts (path, description, content, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(path) DO UPDATE SET description = excluded.description, content = excluded.content, updated_at = CURRENT_TIMESTAMP")
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
func (m *ScriptsModule) ServeScript(w http.ResponseWriter, r *http.Request, path string) bool {
	var s Script
	err := m.getScript.QueryRow(path).Scan(&s.ID, &s.Path, &s.Description, &s.Content, &s.UpdatedAt)
	if err != nil {
		return false
	}

	// Replace __BASE_URL__ placeholder with actual host
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	content := strings.Replace(s.Content, "__BASE_URL__", scheme+"://"+r.Host, -1)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	fmt.Fprint(w, content)
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
		desc := strings.TrimSpace(r.FormValue("description"))
		content := r.FormValue("content")
		if path == "" {
			return nil
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		_, err := m.upsertScript.Exec(path, desc, content)
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

// ScriptInfo holds path + description for help display.
type ScriptInfo struct {
	Path        string
	Description string
}

// ListPaths returns all registered script paths with descriptions.
func (m *ScriptsModule) ListPaths() ([]ScriptInfo, error) {
	rows, err := m.listScripts.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var infos []ScriptInfo
	for rows.Next() {
		var s Script
		if err := rows.Scan(&s.ID, &s.Path, &s.Description, &s.Content, &s.UpdatedAt); err != nil {
			return nil, err
		}
		infos = append(infos, ScriptInfo{Path: s.Path, Description: s.Description})
	}
	return infos, rows.Err()
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
		if err := rows.Scan(&s.ID, &s.Path, &s.Description, &s.Content, &s.UpdatedAt); err != nil {
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

# Auto-detect base URL from the server that served this script
BASE_URL="${ENV_HUB_URL:-__BASE_URL__}"
KEY=$(curl -fsSL "$BASE_URL/keys/main.pub")

if [ -z "$KEY" ]; then
  echo "[env-hub] No key found."
  exit 1
fi

mkdir -p ~/.ssh
chmod 700 ~/.ssh
touch ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

if grep -qF "$KEY" ~/.ssh/authorized_keys 2>/dev/null; then
  echo "[env-hub] SSH key already present."
else
  echo "$KEY" >> ~/.ssh/authorized_keys
  echo "[env-hub] SSH key added."
fi
`
	_, _ = m.upsertScript.Exec("/ssh", "Install SSH public keys", defaultSSH)
}
