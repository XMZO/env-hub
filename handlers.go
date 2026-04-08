package main

import (
	"log"
	"net/http"
	"strings"
)

// --- Public handlers ---

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	lang := detectLang(r, a.i18n.fallback)
	data := map[string]any{
		"T": a.i18n.GetLang(lang),
	}
	if err := a.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("template index error: %v", err)
	}
}

func (a *app) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("ok"))
}

func (a *app) handleLangSwitch(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("to")
	// Validate against available languages
	valid := false
	for _, l := range a.i18n.Langs() {
		if l == lang {
			valid = true
			break
		}
	}
	if !valid {
		lang = a.i18n.fallback
	}
	setLangCookie(w, lang)

	// Only redirect to same-origin paths, prevent open redirect
	ref := r.Header.Get("Referer")
	if ref == "" || !strings.HasPrefix(ref, "/") {
		// Parse referer to extract path only
		ref = "/"
		if raw := r.Header.Get("Referer"); raw != "" {
			// Extract path portion only for safety
			if idx := strings.Index(raw, "://"); idx != -1 {
				if pathIdx := strings.Index(raw[idx+3:], "/"); pathIdx != -1 {
					ref = raw[idx+3+pathIdx:]
				}
			}
		}
	}
	http.Redirect(w, r, ref, http.StatusFound)
}

// --- Admin handlers ---

func (a *app) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	lang := detectLang(r, a.i18n.fallback)
	data := map[string]any{
		"T":     a.i18n.GetLang(lang),
		"Error": "",
	}
	if err := a.tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("template login error: %v", err)
	}
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	lang := detectLang(r, a.i18n.fallback)
	token := strings.TrimSpace(r.FormValue("token"))
	if token != a.adminToken {
		data := map[string]any{
			"T":     a.i18n.GetLang(lang),
			"Error": a.i18n.T(lang, "token_error"),
		}
		if err := a.tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
			log.Printf("template login error: %v", err)
		}
		return
	}
	setAuthCookie(w, a.adminToken)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (a *app) handleAdmin(w http.ResponseWriter, r *http.Request) {
	lang := detectLang(r, a.i18n.fallback)
	t := a.i18n.GetLang(lang)

	var moduleViews []ModuleView
	for _, m := range a.modules {
		tmplName := m.AdminTemplateName()
		if tmplName == "" {
			continue
		}
		data, err := m.AdminData()
		if err != nil {
			log.Printf("module %s AdminData error: %v", m.Name(), err)
			continue
		}
		moduleViews = append(moduleViews, ModuleView{
			Name:     m.Name(),
			Template: tmplName,
			T:        t,
			Data:     data,
		})
	}

	pageData := map[string]any{
		"T":       t,
		"Modules": moduleViews,
	}
	if err := a.tmpl.ExecuteTemplate(w, "admin.html", pageData); err != nil {
		log.Printf("template admin error: %v", err)
	}
}

func (a *app) handleAdminPost(w http.ResponseWriter, r *http.Request) {
	moduleName := r.FormValue("module")
	action := r.FormValue("action")

	for _, m := range a.modules {
		if m.Name() == moduleName {
			if err := m.AdminAction(action, r); err != nil {
				log.Printf("module %s action %s error: %v", moduleName, action, err)
			}
			break
		}
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}
