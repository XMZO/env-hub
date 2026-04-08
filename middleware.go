package main

import (
	"net/http"
	"time"
)

func (a *app) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check query param token
		if token := r.URL.Query().Get("token"); token == a.adminToken {
			http.SetCookie(w, &http.Cookie{
				Name:     "admin_token",
				Value:    a.adminToken,
				Path:     "/admin",
				HttpOnly: true,
				MaxAge:   int(30 * 24 * time.Hour / time.Second),
				SameSite: http.SameSiteLaxMode,
			})
			// Redirect to strip token from URL
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}

		// Check cookie
		cookie, err := r.Cookie("admin_token")
		if err != nil || cookie.Value != a.adminToken {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}

		next(w, r)
	}
}
