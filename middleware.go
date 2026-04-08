package main

import (
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
	"time"
)

// --- Auth ---

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    token,
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   int(30 * 24 * time.Hour / time.Second),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *app) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token := r.URL.Query().Get("token"); token == a.adminToken {
			setAuthCookie(w, a.adminToken)
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}

		cookie, err := r.Cookie("admin_token")
		if err != nil || cookie.Value != a.adminToken {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}

		next(w, r)
	}
}

// --- Security headers ---

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// --- Gzip ---

var gzPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	sniffDone   bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	// Sniff Content-Type from uncompressed data before gzip writes
	if !w.sniffDone {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.sniffDone = true
	}
	return w.gz.Write(b)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzPool.Get().(*gzip.Writer)
		gz.Reset(w)

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)

		gz.Close()
		gzPool.Put(gz)
	})
}

