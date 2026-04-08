package main

import (
	"database/sql"
	"net/http"
)

// Module defines a pluggable feature module.
// To add a new module: implement this interface and register in main.go.
type Module interface {
	// Name returns the unique identifier for this module.
	Name() string

	// Init runs migrations and prepares statements.
	Init(db *sql.DB) error

	// RegisterRoutes adds public-facing HTTP routes.
	RegisterRoutes(mux *http.ServeMux)

	// AdminTemplateName returns the template name for the admin section.
	// Return "" if this module has no admin UI.
	AdminTemplateName() string

	// AdminData returns data to render the admin template.
	AdminData() (any, error)

	// AdminAction handles a POST action from the admin UI.
	AdminAction(action string, r *http.Request) error
}

// ModuleView carries data for rendering a module's admin section.
type ModuleView struct {
	Name     string
	Template string
	T        map[string]string
	Data     any
}
