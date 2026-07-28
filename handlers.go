package main

import (
	"encoding/json"
	"errors"
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
	http.Redirect(w, r, safeReturnPath(r.Header.Get("Referer")), http.StatusFound)
}

// safeReturnPath turns a Referer value into a same-site redirect target.
// Only path and query are kept, so the redirect can never leave this site;
// anything unparseable or scheme-relative ("//host") falls back to "/".
func safeReturnPath(ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return "/"
	}
	path := u.EscapedPath()
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.HasPrefix(path, "/\\") {
		return "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path
}

// --- Admin handlers ---

func (a *app) loginData(r *http.Request, errMsg string) map[string]any {
	lang := a.i18n.ResolveLang(detectLang(r, a.i18n.fallback))
	return map[string]any{
		"T":             a.i18n.GetLang(lang),
		"Lang":          lang,
		"Translations":  a.i18n.All(),
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

func (a *app) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	lang := a.i18n.ResolveLang(detectLang(r, a.i18n.fallback))

	// Verify Turnstile if enabled
	if a.turnstileSite != "" {
		cfToken := r.FormValue("cf-turnstile-response")
		if !verifyTurnstile(a.turnstileKey, cfToken, realIP(r)) {
			data := a.loginData(r, a.i18n.T(lang, "captcha_error"))
			if err := a.tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
				log.Printf("template login error: %v", err)
			}
			return
		}
	}

	token := strings.TrimSpace(r.FormValue("token"))
	if !tokenEqual(token, a.adminToken) {
		data := a.loginData(r, a.i18n.T(lang, "token_error"))
		if err := a.tmpl.ExecuteTemplate(w, "login.html", data); err != nil {
			log.Printf("template login error: %v", err)
		}
		return
	}
	setAuthCookie(w, r, a.sessionToken)
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (a *app) handleAdmin(w http.ResponseWriter, r *http.Request) {
	lang := a.i18n.ResolveLang(detectLang(r, a.i18n.fallback))
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
		"T":            t,
		"Lang":         lang,
		"Translations": a.i18n.All(),
		"Host":         r.Host,
		"Version":      version,
		"Error":        r.URL.Query().Get("err"),
		"Modules":      moduleViews,
	}
	if err := a.tmpl.ExecuteTemplate(w, "admin.html", pageData); err != nil {
		log.Printf("template admin error: %v", err)
	}
}

func (a *app) handleAdminPost(w http.ResponseWriter, r *http.Request) {
	moduleName := r.FormValue("module")
	action := r.FormValue("action")
	jsonResponse := wantsAdminJSON(r)

	var actionErr error
	found := false
	for _, m := range a.modules {
		if m.Name() == moduleName {
			found = true
			actionErr = m.AdminAction(action, r)
			break
		}
	}
	if !found {
		actionErr = adminBadRequest("unknown admin module")
	}
	if actionErr != nil {
		log.Printf("module %s action %s error: %v", moduleName, action, actionErr)
		if jsonResponse {
			writeAdminJSONError(w, actionErr)
			return
		}
		// Plain form path: surface the error on the admin page instead of
		// silently redirecting.
		_, msg := adminErrorStatus(actionErr)
		http.Redirect(w, r, "/admin?err="+url.QueryEscape(msg), http.StatusFound)
		return
	}

	if jsonResponse {
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

type adminHTTPError struct {
	status  int
	message string
}

func (e adminHTTPError) Error() string {
	return e.message
}

func adminBadRequest(message string) error {
	return adminHTTPError{status: http.StatusBadRequest, message: message}
}

func wantsAdminJSON(r *http.Request) bool {
	return r.Header.Get("X-Requested-With") == "fetch" || strings.Contains(r.Header.Get("Accept"), "application/json")
}

// adminErrorStatus maps an action error to an HTTP status and a message safe
// to show in the admin UI.
func adminErrorStatus(err error) (int, string) {
	var httpErr adminHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.status, httpErr.message
	}
	return http.StatusInternalServerError, "internal error"
}

func writeAdminJSONError(w http.ResponseWriter, err error) {
	status, message := adminErrorStatus(err)
	writeAdminJSON(w, status, map[string]any{"ok": false, "error": message})
}

func writeAdminJSON(w http.ResponseWriter, status int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("admin json encode error: %v", err)
	}
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
