package main

import (
	"strings"
	"testing"
)

// Regression test for the DSN format: modernc.org/sqlite only applies
// `_pragma=name(value)` parameters, so make sure ours actually take effect.
func TestOpenDBAppliesPragmas(t *testing.T) {
	db, err := openDB(t.TempDir())
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}

	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", timeout)
	}
}
