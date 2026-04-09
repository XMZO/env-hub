package main

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
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
	templates    []ScriptTemplate
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

	// Load script templates (embedded + optional external directory)
	extraDir := os.Getenv("SCRIPT_TEMPLATES_DIR")
	m.templates = loadScriptTemplates(extraDir)

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

	// Detect client IP and classify as v4 or v6 (only if valid, to prevent header injection)
	ip, ipv4, ipv6 := "", "", ""
	if parsed := net.ParseIP(realIP(r)); parsed != nil {
		ip = parsed.String()
		if parsed.To4() != nil {
			ipv4 = ip
		} else {
			ipv6 = ip
		}
	}

	// Strip CRLF — shell scripts must use LF line endings
	content := strings.ReplaceAll(s.Content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "")

	// Per-client placeholders disable CF caching
	perClient := strings.Contains(content, "__CLIENT_IP")

	content = strings.Replace(content, "__BASE_URL__", scheme+"://"+r.Host, -1)
	content = strings.Replace(content, "__CLIENT_IP__", ip, -1)
	content = strings.Replace(content, "__CLIENT_IPV4__", ipv4, -1)
	content = strings.Replace(content, "__CLIENT_IPV6__", ipv6, -1)

	// Auto-detect JSON content by first non-whitespace char
	trimmed := strings.TrimLeft(content, " \t\r\n")
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	if perClient {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60")
	}
	fmt.Fprint(w, content)
	return true
}

func (m *ScriptsModule) AdminTemplateName() string { return "mod_scripts.html" }

// ScriptsAdminData bundles script list and available templates for admin UI.
type ScriptsAdminData struct {
	Scripts   []Script
	Templates []ScriptTemplate
}

func (m *ScriptsModule) AdminData() (any, error) {
	scripts, err := m.list()
	if err != nil {
		return nil, err
	}
	return ScriptsAdminData{Scripts: scripts, Templates: m.templates}, nil
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
	IsData      bool // true if content is data (JSON/text), false if shell script
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
		// Detect data (JSON/array) vs shell script by first non-whitespace char
		trimmed := strings.TrimLeft(s.Content, " \t\r\n")
		isData := strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
		infos = append(infos, ScriptInfo{Path: s.Path, Description: s.Description, IsData: isData})
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

	// Seed the "ssh" template as /ssh if available
	for _, t := range m.templates {
		if t.ID == "ssh" {
			_, _ = m.upsertScript.Exec("/ssh", t.Description, t.Content)
			return
		}
	}
}
