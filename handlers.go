package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// --- Public handlers ---

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	a.tmpl.ExecuteTemplate(w, "index.html", nil)
}

func (a *app) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ok")
}

func (a *app) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := getAllKeysText(a.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("handleKeys error: %v", err)
		return
	}
	if keys == "" {
		http.Error(w, "no keys configured", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, keys)
}

// --- Admin handlers ---

func (a *app) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	a.tmpl.ExecuteTemplate(w, "login.html", nil)
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.FormValue("token"))
	if token != a.adminToken {
		a.tmpl.ExecuteTemplate(w, "login.html", map[string]string{"Error": "Token 不正确"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    a.adminToken,
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   30 * 24 * 3600,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (a *app) handleAdmin(w http.ResponseWriter, r *http.Request) {
	keys, err := listKeys(a.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("handleAdmin listKeys error: %v", err)
		return
	}
	scripts, err := listScripts(a.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		log.Printf("handleAdmin listScripts error: %v", err)
		return
	}
	data := map[string]any{
		"Keys":    keys,
		"Scripts": scripts,
	}
	a.tmpl.ExecuteTemplate(w, "admin.html", data)
}

func (a *app) handleAdminPost(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("action")

	switch action {
	case "add_key":
		name := strings.TrimSpace(r.FormValue("name"))
		key := strings.TrimSpace(r.FormValue("key"))
		if name == "" || key == "" {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
		if err := addKey(a.db, name, key); err != nil {
			log.Printf("addKey error: %v", err)
		}

	case "delete_key":
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if id > 0 {
			if err := deleteKey(a.db, id); err != nil {
				log.Printf("deleteKey error: %v", err)
			}
		}

	case "save_script":
		path := strings.TrimSpace(r.FormValue("path"))
		content := r.FormValue("content")
		if path != "" {
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if err := upsertScript(a.db, path, content); err != nil {
				log.Printf("upsertScript error: %v", err)
			}
		}

	case "new_script":
		path := strings.TrimSpace(r.FormValue("path"))
		content := r.FormValue("content")
		if path != "" {
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			if err := upsertScript(a.db, path, content); err != nil {
				log.Printf("upsertScript error: %v", err)
			}
		}

	case "delete_script":
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if id > 0 {
			if err := deleteScript(a.db, id); err != nil {
				log.Printf("deleteScript error: %v", err)
			}
		}
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}
