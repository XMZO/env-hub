package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// --- Public handlers ---

func (a *app) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("ok"))
}

func (a *app) handleLangSwitch(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("to")
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

	ref := r.Header.Get("Referer")
	if ref == "" || !strings.HasPrefix(ref, "/") {
		ref = "/"
		if raw := r.Header.Get("Referer"); raw != "" {
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

func (a *app) loginData(r *http.Request, errMsg string) map[string]any {
	lang := detectLang(r, a.i18n.fallback)
	return map[string]any{
		"T":             a.i18n.GetLang(lang),
		"Host":          r.Host,
		"Error":         errMsg,
		"TurnstileSite": a.turnstileSite,
	}
}

func (a *app) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if err := a.tmpl.ExecuteTemplate(w, "login.html", a.loginData(r, "")); err != nil {
		log.Printf("template login error: %v", err)
	}
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	lang := detectLang(r, a.i18n.fallback)

	// Verify Turnstile if enabled
	if a.turnstileSite != "" {
		cfToken := r.FormValue("cf-turnstile-response")
		if !verifyTurnstile(a.turnstileKey, cfToken, clientIP(r)) {
			data := a.loginData(r, a.i18n.T(lang, "captcha_error"))
			if err := a.tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
				log.Printf("template login error: %v", err)
			}
			return
		}
	}

	token := strings.TrimSpace(r.FormValue("token"))
	if token != a.adminToken {
		data := a.loginData(r, a.i18n.T(lang, "token_error"))
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
		"Host":    r.Host,
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

// --- Turnstile verification ---

var turnstileClient = &http.Client{Timeout: 5 * time.Second}

func verifyTurnstile(secret, token, remoteIP string) bool {
	if token == "" {
		return false
	}
	resp, err := turnstileClient.PostForm(
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		url.Values{
			"secret":   {secret},
			"response": {token},
			"remoteip": {remoteIP},
		},
	)
	if err != nil {
		log.Printf("turnstile verify error: %v", err)
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("turnstile decode error: %v", err)
		return false
	}
	return result.Success
}

// clientIP reuses realIP from ratelimit.go
func clientIP(r *http.Request) string {
	return realIP(r)
}
