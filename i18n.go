package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

//go:embed locales/*.json
var localeFS embed.FS

type I18n struct {
	langs    map[string]map[string]string
	fallback string
}

func newI18n(fallback string) (*I18n, error) {
	i := &I18n{
		langs:    make(map[string]map[string]string),
		fallback: fallback,
	}

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		lang := strings.TrimSuffix(e.Name(), ".json")
		i.langs[lang] = m
	}

	return i, nil
}

// GetLang returns the full translation map for a language.
func (i *I18n) GetLang(lang string) map[string]string {
	if m, ok := i.langs[lang]; ok {
		return m
	}
	return i.langs[i.fallback]
}

// T looks up a single key.
func (i *I18n) T(lang, key string) string {
	if m, ok := i.langs[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := i.langs[i.fallback]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return key
}

// Langs returns all available language codes.
func (i *I18n) Langs() []string {
	keys := make([]string, 0, len(i.langs))
	for k := range i.langs {
		keys = append(keys, k)
	}
	return keys
}

// detectLang reads language from cookie > Accept-Language > fallback.
func detectLang(r *http.Request, fallback string) string {
	if c, err := r.Cookie("lang"); err == nil {
		return c.Value
	}
	accept := r.Header.Get("Accept-Language")
	if strings.Contains(accept, "zh") {
		return "zh"
	}
	return fallback
}

// setLangCookie sets a language preference cookie.
func setLangCookie(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "lang",
		Value:    lang,
		Path:     "/",
		MaxAge:   int(365 * 24 * time.Hour / time.Second),
		SameSite: http.SameSiteLaxMode,
	})
}
