package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed script_templates/*.sh
var scriptTemplateFS embed.FS

// ScriptTemplate represents a reusable script template shown in admin UI.
type ScriptTemplate struct {
	ID          string // filename without .sh
	Name        string // human-readable name
	Path        string // suggested route path
	Description string
	Content     string // actual script content (metadata header stripped)
}

// loadScriptTemplates loads templates from the embedded filesystem plus
// an optional external directory. External templates with the same filename
// override embedded ones.
func loadScriptTemplates(extraDir string) []ScriptTemplate {
	templates := map[string]ScriptTemplate{}

	// Load embedded templates
	_ = fs.WalkDir(scriptTemplateFS, "script_templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		data, err := scriptTemplateFS.ReadFile(path)
		if err != nil {
			return nil
		}
		tpl := parseScriptTemplate(filepath.Base(path), string(data))
		templates[tpl.ID] = tpl
		return nil
	})

	// Load external templates (override by same filename)
	if extraDir != "" {
		if entries, err := os.ReadDir(extraDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(extraDir, e.Name()))
				if err != nil {
					continue
				}
				tpl := parseScriptTemplate(e.Name(), string(data))
				templates[tpl.ID] = tpl
			}
		}
	}

	// Sort by ID for stable order
	out := make([]ScriptTemplate, 0, len(templates))
	for _, t := range templates {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// parseScriptTemplate reads metadata from the first comment block of a shell
// script. Format:
//
//	# name: Human Name
//	# path: /suggested
//	# desc: Short description
//	#!/bin/sh
//	...actual script...
func parseScriptTemplate(filename, raw string) ScriptTemplate {
	// Normalize line endings
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "")

	tpl := ScriptTemplate{
		ID: strings.TrimSuffix(filename, ".sh"),
	}

	lines := strings.Split(raw, "\n")
	contentStart := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Metadata lines look like "# key: value" BEFORE the shebang
		if strings.HasPrefix(trimmed, "#!") {
			contentStart = i
			break
		}
		if strings.HasPrefix(trimmed, "#") {
			kv := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			parts := strings.SplitN(kv, ":", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				val := strings.TrimSpace(parts[1])
				switch key {
				case "name":
					tpl.Name = val
				case "path":
					tpl.Path = val
				case "desc", "description":
					tpl.Description = val
				}
			}
			continue
		}
		// Non-comment before shebang: treat as start of content
		contentStart = i
		break
	}

	tpl.Content = strings.Join(lines[contentStart:], "\n")

	// Fallbacks
	if tpl.Name == "" {
		tpl.Name = tpl.ID
	}
	if tpl.Path == "" {
		tpl.Path = "/" + tpl.ID
	}

	return tpl
}
