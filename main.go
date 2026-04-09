package main

import (
	"bytes"
	"context"
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
	modules       []Module
	scripts       *ScriptsModule
	i18n          *I18n
	adminToken    string
	tmpl          *template.Template
	turnstileSite string // Cloudflare Turnstile site key (empty = disabled)
	turnstileKey  string // Cloudflare Turnstile secret key
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

	// Turnstile: "sitekey,secret" or "sitekey;secret", empty/none/null = disabled
	var turnstileSite, turnstileKey string
	if ts := os.Getenv("TURNSTILE_KEYS"); ts != "" {
		lower := strings.ToLower(strings.TrimSpace(ts))
		if lower != "none" && lower != "null" && lower != "" {
			sep := ","
			if strings.Contains(ts, ";") {
				sep = ";"
			}
			parts := strings.SplitN(ts, sep, 2)
			if len(parts) == 2 {
				turnstileSite = strings.TrimSpace(parts[0])
				turnstileKey = strings.TrimSpace(parts[1])
				log.Printf("Turnstile enabled")
			}
		}
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
		modules:       modules,
		scripts:       scriptsModule,
		i18n:          i18n,
		adminToken:    adminToken,
		tmpl:          tmpl,
		turnstileSite: turnstileSite,
		turnstileKey:  turnstileKey,
	}

	mux := http.NewServeMux()

	// Core routes
	mux.HandleFunc("GET /healthz", a.handleHealthz)

	// Lang switch
	mux.HandleFunc("GET /lang", a.handleLangSwitch)

	// Login rate limiter: 5 attempts per minute per IP
	loginRL := newRateLimiter(5, time.Minute)

	// Admin routes
	mux.HandleFunc("GET /admin/login", a.handleLoginPage)
	mux.HandleFunc("POST /admin/login", loginRL.handlerFunc(a.handleLogin))
	mux.HandleFunc("GET /admin/logout", a.handleLogout)
	mux.HandleFunc("GET /admin", a.requireAuth(a.handleAdmin))
	mux.HandleFunc("POST /admin", a.requireAuth(a.handleAdminPost))

	// Module routes
	for _, m := range modules {
		m.RegisterRoutes(mux)
	}

	// Rate limiter: 60 requests per minute per IP
	rl := newRateLimiter(60, time.Minute)

	// Middleware chain (outer → inner): rate limit → security → gzip → script fallback → mux
	handler := rl.middleware(securityHeaders(gzipMiddleware(a.scriptFallback(mux))))

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

func isCurl(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("User-Agent"), "curl/")
}

func (a *app) scriptFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !strings.Contains(r.URL.Path[1:], "/") {
			// Try serving as a script route (including "/" itself)
			if a.scripts.ServeScript(w, r, r.URL.Path) {
				return
			}
			// curl on root with no "/" script → show help
			if r.URL.Path == "/" && isCurl(r) {
				a.serveHelp(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) serveHelp(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	base := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	fmt.Fprintf(w, "  env-hub\n\n")
	fmt.Fprintf(w, "  Usage:  curl -fsSL %s/<route> | sh\n\n", base)
	fmt.Fprintf(w, "  Available routes:\n\n")

	scripts, err := a.scripts.ListPaths()
	if err != nil || len(scripts) == 0 {
		fmt.Fprintf(w, "    (none)\n")
	} else {
		// Compute max command width for alignment
		maxLen := 0
		for _, s := range scripts {
			cmd := fmt.Sprintf("curl -fsSL %s%s | sh", base, s.Path)
			if len(cmd) > maxLen {
				maxLen = len(cmd)
			}
		}
		for _, s := range scripts {
			cmd := fmt.Sprintf("curl -fsSL %s%s | sh", base, s.Path)
			if s.Description != "" {
				fmt.Fprintf(w, "    %-*s  # %s\n", maxLen, cmd, s.Description)
			} else {
				fmt.Fprintf(w, "    %s\n", cmd)
			}
		}
	}

	fmt.Fprintf(w, "\n  Other endpoints:\n\n")
	fmt.Fprintf(w, "    curl %s/keys/main.pub    # SSH public keys\n", base)
	fmt.Fprintf(w, "    curl %s/healthz          # Health check\n", base)
	fmt.Fprintf(w, "\n")
}
