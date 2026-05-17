package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
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
		"Modules":      moduleViews,
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

type scriptSimulationResponse struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Sandbox  string `json:"sandbox"`
	Error    string `json:"error,omitempty"`
}

func (a *app) handleScriptSimulation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if err := r.ParseForm(); err != nil {
		writeSimulationJSON(w, http.StatusBadRequest, scriptSimulationResponse{Error: "invalid form"})
		return
	}

	content := r.FormValue("content")
	shell := r.FormValue("shell")
	allowNetwork := r.FormValue("network") == "true"
	if shell != "bash" {
		shell = "sh"
	}
	if strings.TrimSpace(content) == "" {
		writeSimulationJSON(w, http.StatusBadRequest, scriptSimulationResponse{Error: "script is empty"})
		return
	}
	if len(content) > 128*1024 {
		writeSimulationJSON(w, http.StatusBadRequest, scriptSimulationResponse{Error: "script is too large"})
		return
	}

	resp := runScriptSimulation(r.Context(), content, shell, allowNetwork)
	status := http.StatusOK
	if resp.Error != "" {
		status = http.StatusBadRequest
	}
	writeSimulationJSON(w, status, resp)
}

func writeSimulationJSON(w http.ResponseWriter, status int, resp scriptSimulationResponse) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func runScriptSimulation(ctx context.Context, content, shell string, allowNetwork bool) scriptSimulationResponse {
	if _, err := exec.LookPath("docker"); err != nil {
		return scriptSimulationResponse{
			ExitCode: -1,
			Error:    "docker is required for sandbox simulation",
			Sandbox:  scriptSimulationSandboxSummary(allowNetwork),
			Output:   "Docker was not found on the server. Simulation never falls back to running on the host.",
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	image := simulationImage()
	args := []string{"/bin/sh", "-s"}
	if shell == "bash" {
		args = []string{"/bin/bash", "-s"}
	}

	containerName := "env-hub-sim-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	dockerArgs := []string{
		"run",
		"-i",
		"--rm",
		"--name", containerName,
		"--user", "65534:65534",
		"--cpus", "1",
		"--memory", "256m",
		"--memory-swap", "256m",
		"--pids-limit", "128",
		"--cap-drop", "ALL",
		"--read-only",
		"--security-opt", "no-new-privileges",
		"--tmpfs", "/tmp:rw,nosuid,nodev,size=64m,mode=1777",
		"--workdir", "/tmp",
		"--env", "HOME=/tmp",
		"--entrypoint", "",
		"--stop-timeout", "2",
	}
	if !allowNetwork {
		dockerArgs = append(dockerArgs, "--network", "none")
	}
	dockerArgs = append(dockerArgs, image)
	dockerArgs = append(dockerArgs, args...)
	cmd := exec.CommandContext(runCtx, "docker", dockerArgs...)

	var out bytes.Buffer
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	output := limitSimulationOutput(out.String())
	resp := scriptSimulationResponse{
		OK:       err == nil,
		ExitCode: 0,
		Output:   output,
		Sandbox:  scriptSimulationSandboxSummary(allowNetwork),
	}

	if runCtx.Err() == context.DeadlineExceeded {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "-f", containerName).Run()
		resp.OK = false
		resp.ExitCode = -1
		resp.Error = "simulation timed out after 45s"
		return resp
	}

	if err != nil {
		resp.OK = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = -1
			resp.Error = err.Error()
		}
	}

	return resp
}

func simulationImage() string {
	if image := strings.TrimSpace(os.Getenv("SIMULATION_IMAGE")); image != "" {
		return image
	}
	if image := strings.TrimSpace(os.Getenv("ENV_HUB_IMAGE")); image != "" {
		return image
	}
	return "ghcr.io/xmzo/env-hub:latest"
}

func scriptSimulationSandboxSummary(allowNetwork bool) string {
	network := "network=disabled"
	if allowNetwork {
		network = "network=enabled"
	}
	return "docker, " + network + ", image=" + simulationImage() + ", user=65534:65534, no host mounts, read-only rootfs, cap-drop=ALL, no-new-privileges, tmpfs=/tmp, cpu=1, memory=256m, pids=128, timeout=45s"
}

func limitSimulationOutput(output string) string {
	const max = 20 * 1024
	if len(output) <= max {
		return output
	}
	return output[:max] + "\n[env-hub] output truncated\n"
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
