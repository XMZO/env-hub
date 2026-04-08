package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type KeysModule struct {
	db         *sql.DB
	listKeys   *sql.Stmt
	addKey     *sql.Stmt
	deleteKey  *sql.Stmt
	allText    *sql.Stmt
}

type Key struct {
	ID        int64
	Name      string
	Key       string
	CreatedAt time.Time
}

func NewKeysModule() *KeysModule {
	return &KeysModule{}
}

func (m *KeysModule) Name() string { return "keys" }

func (m *KeysModule) Init(db *sql.DB) error {
	m.db = db

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS keys (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			key        TEXT NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
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

	m.listKeys = prep("SELECT id, name, key, created_at FROM keys ORDER BY id")
	m.addKey = prep("INSERT INTO keys (name, key) VALUES (?, ?)")
	m.deleteKey = prep("DELETE FROM keys WHERE id = ?")
	m.allText = prep("SELECT key FROM keys ORDER BY id")

	return prepErr
}

func (m *KeysModule) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /keys/main.pub", m.handleKeys)
}

func (m *KeysModule) handleKeys(w http.ResponseWriter, r *http.Request) {
	text, err := m.getAllKeysText()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if text == "" {
		http.Error(w, "no keys configured", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, text)
}

func (m *KeysModule) AdminTemplateName() string { return "mod_keys.html" }

func (m *KeysModule) AdminData() (any, error) {
	return m.list()
}

func (m *KeysModule) AdminAction(action string, r *http.Request) error {
	switch action {
	case "add":
		name := strings.TrimSpace(r.FormValue("name"))
		key := strings.TrimSpace(r.FormValue("key"))
		if name == "" || key == "" {
			return nil
		}
		_, err := m.addKey.Exec(name, key)
		return err

	case "delete":
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if id > 0 {
			_, err := m.deleteKey.Exec(id)
			return err
		}
	}
	return nil
}

func (m *KeysModule) list() ([]Key, error) {
	rows, err := m.listKeys.Query()
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

func (m *KeysModule) getAllKeysText() (string, error) {
	rows, err := m.allText.Query()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	first := true
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return "", err
		}
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		first = false
	}
	return b.String(), rows.Err()
}
