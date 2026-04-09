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
		return err
	}

	// Migration: add description column if missing (ignore "duplicate column" error)
	db.Exec("ALTER TABLE scripts ADD COLUMN description TEXT NOT NULL DEFAULT ''")

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
	// Strip CRLF — shell scripts must use LF line endings
	content := strings.ReplaceAll(s.Content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "")
	content = strings.Replace(content, "__BASE_URL__", scheme+"://"+r.Host, -1)

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
# env-hub: install SSH public keys
set -eu

# Colors (disabled if not a tty or NO_COLOR is set)
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_GREEN='\033[32m'; C_YELLOW='\033[33m'; C_RED='\033[31m'; C_DIM='\033[2m'; C_OFF='\033[0m'
else
  C_GREEN=''; C_YELLOW=''; C_RED=''; C_DIM=''; C_OFF=''
fi

log()  { printf '%b[env-hub]%b %s\n' "$C_DIM" "$C_OFF" "$1"; }
ok()   { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_GREEN" "$1" "$C_OFF"; }
warn() { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_YELLOW" "$1" "$C_OFF"; }
err()  { printf '%b[env-hub]%b %b%s%b\n' "$C_DIM" "$C_OFF" "$C_RED" "$1" "$C_OFF" >&2; }

# Check dependencies
command -v curl >/dev/null 2>&1 || { err "curl is required but not installed."; exit 1; }

# Determine target SSH directory (respect SUDO_USER)
if [ -n "${SUDO_USER:-}" ] && [ "$(id -u)" = "0" ]; then
  SSH_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)
  OWNER="$SUDO_USER"
else
  SSH_HOME="$HOME"
  OWNER=""
fi
SSH_DIR="$SSH_HOME/.ssh"
AUTH="$SSH_DIR/authorized_keys"

# Fetch keys
BASE_URL="${ENV_HUB_URL:-__BASE_URL__}"
log "Fetching keys from $BASE_URL/keys/main.pub"
KEYS=$(curl -fsSL "$BASE_URL/keys/main.pub") || { err "Failed to fetch keys."; exit 1; }
[ -z "$KEYS" ] && { err "No keys found at remote."; exit 1; }

# Prepare directory
mkdir -p "$SSH_DIR"
chmod 700 "$SSH_DIR"
touch "$AUTH"
chmod 600 "$AUTH"

# Install keys line by line
ADDED=0
SKIPPED=0
TOTAL=0
OLDIFS=$IFS
IFS='
'
for KEY in $KEYS; do
  [ -z "$KEY" ] && continue
  TOTAL=$((TOTAL + 1))
  if grep -qxF "$KEY" "$AUTH" 2>/dev/null; then
    SKIPPED=$((SKIPPED + 1))
  else
    printf '%s\n' "$KEY" >> "$AUTH"
    ADDED=$((ADDED + 1))
  fi
done
IFS=$OLDIFS

# Fix ownership if running via sudo
if [ -n "$OWNER" ]; then
  chown -R "$OWNER:$OWNER" "$SSH_DIR"
fi

# Summary
if [ "$ADDED" -gt 0 ]; then
  ok "Added $ADDED key(s), skipped $SKIPPED (already present). Total: $TOTAL."
else
  warn "All $TOTAL key(s) already present, nothing to do."
fi
`
	_, _ = m.upsertScript.Exec("/ssh", "Install SSH public keys", defaultSSH)
}
