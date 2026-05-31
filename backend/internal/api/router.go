package api

import (
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// NewRouter wires all HTTP routes and returns the configured mux.
func NewRouter(db *sqlx.DB, st store.Store, registry *node.NodeRegistry) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("GET /health", newHealthHandler(db))

	wh := &workflowHandler{store: st, registry: registry}
	mux.HandleFunc("GET /workflows", wh.list)
	mux.HandleFunc("POST /workflows", wh.create)
	mux.HandleFunc("GET /workflows/{id}", wh.get)
	mux.HandleFunc("PUT /workflows/{id}", wh.update)
	mux.HandleFunc("DELETE /workflows/{id}", wh.delete)

	nth := &nodeTypeHandler{registry: registry}
	mux.HandleFunc("GET /node-types", nth.list)

	return mux
}
