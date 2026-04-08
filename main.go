package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed templates/*
var templateFS embed.FS

type app struct {
	db         *sql.DB
	adminToken string
	tmpl       *template.Template
}

func main() {
	adminToken := os.Getenv("ADMIN_TOKEN")
	if adminToken == "" {
		log.Fatal("ADMIN_TOKEN environment variable is required")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	db, err := openDB(dataDir)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	a := &app{
		db:         db,
		adminToken: adminToken,
		tmpl:       tmpl,
	}

	seedDefaultScript(db)

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /", a.handleIndex)
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	mux.HandleFunc("GET /keys/main.pub", a.handleKeys)

	// Admin routes
	mux.HandleFunc("GET /admin/login", a.handleLoginPage)
	mux.HandleFunc("POST /admin/login", a.handleLogin)
	mux.HandleFunc("GET /admin", a.requireAuth(a.handleAdmin))
	mux.HandleFunc("POST /admin", a.requireAuth(a.handleAdminPost))

	// Dynamic script routes — wrap the mux to catch unmatched paths
	handler := a.scriptFallback(mux)

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: handler,
	}

	go func() {
		log.Printf("env-hub listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func (a *app) scriptFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the mux try first by checking if it has an exact match
		// For paths like /ssh, /init, /dev — check scripts table
		path := strings.TrimPrefix(r.URL.Path, "/")
		if r.Method == http.MethodGet && path != "" && !strings.Contains(path, "/") {
			s, err := getScript(a.db, "/"+path)
			if err == nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				fmt.Fprint(w, s.Content)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func seedDefaultScript(db *sql.DB) {
	_, err := getScript(db, "/ssh")
	if err != nil {
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
		upsertScript(db, "/ssh", defaultSSH)
	}
}
