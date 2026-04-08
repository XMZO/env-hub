package main

import (
	"bytes"
	"context"
	"embed"
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
	modules    []Module
	scripts    *ScriptsModule
	i18n       *I18n
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
		listenAddr = ":9800"
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

	i18n, err := newI18n("en")
	if err != nil {
		log.Fatalf("Failed to load i18n: %v", err)
	}

	// --- Modules: add or remove here ---
	scriptsModule := NewScriptsModule()
	modules := []Module{
		NewKeysModule(),
		scriptsModule,
	}

	for _, m := range modules {
		if err := m.Init(db); err != nil {
			log.Fatalf("Failed to init module %s: %v", m.Name(), err)
		}
	}

	var tmpl *template.Template
	tmpl = template.Must(
		template.New("").Funcs(template.FuncMap{
			"t": func(translations map[string]string, key string) string {
				if v, ok := translations[key]; ok {
					return v
				}
				return key
			},
			"truncate": func(s string, n int) string {
				if len(s) <= n {
					return s
				}
				return s[:n] + "..."
			},
			"renderModule": func(mv ModuleView) template.HTML {
				var buf bytes.Buffer
				if err := tmpl.ExecuteTemplate(&buf, mv.Template, mv); err != nil {
					log.Printf("renderModule %s error: %v", mv.Template, err)
					return ""
				}
				return template.HTML(buf.String())
			},
		}).ParseFS(templateFS, "templates/*.html"),
	)

	a := &app{
		modules:    modules,
		scripts:    scriptsModule,
		i18n:       i18n,
		adminToken: adminToken,
		tmpl:       tmpl,
	}

	mux := http.NewServeMux()

	// Core routes
	mux.HandleFunc("GET /", a.handleIndex)
	mux.HandleFunc("GET /healthz", a.handleHealthz)

	// Lang switch
	mux.HandleFunc("GET /lang", a.handleLangSwitch)

	// Admin routes
	mux.HandleFunc("GET /admin/login", a.handleLoginPage)
	mux.HandleFunc("POST /admin/login", a.handleLogin)
	mux.HandleFunc("GET /admin", a.requireAuth(a.handleAdmin))
	mux.HandleFunc("POST /admin", a.requireAuth(a.handleAdminPost))

	// Module routes
	for _, m := range modules {
		m.RegisterRoutes(mux)
	}

	// Middleware chain (outer → inner): security → gzip → script fallback → mux
	// ETag removed: conflicts with gzip (hash mismatch), and for this app
	// gzip alone is sufficient — responses are small and mostly dynamic.
	handler := securityHeaders(gzipMiddleware(a.scriptFallback(mux)))

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
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
		path := strings.TrimPrefix(r.URL.Path, "/")
		if r.Method == http.MethodGet && path != "" && !strings.Contains(path, "/") {
			if a.scripts.ServeScript(w, "/"+path) {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
