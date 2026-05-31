package api

import (
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// NewRouter wires all HTTP routes and returns the configured mux.
func NewRouter(db *sqlx.DB, st store.Store, registry *node.NodeRegistry, dispatcher trigger.Dispatcher, bus *engine.EventBus) *http.ServeMux {
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

	rh := &runHandler{store: st, dispatcher: dispatcher}
	mux.HandleFunc("POST /workflows/{id}/runs", rh.triggerRun)
	mux.HandleFunc("GET /workflows/{id}/runs", rh.listRuns)
	mux.HandleFunc("GET /runs/{run_id}", rh.getRun)

	wsh := &wsHandler{store: st, bus: bus}
	mux.HandleFunc("GET /runs/{run_id}/events", wsh.streamEvents)

	return mux
}
