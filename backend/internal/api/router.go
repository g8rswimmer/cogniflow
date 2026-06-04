package api

import (
	"log/slog"
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// NewRouter wires all HTTP routes and returns the configured mux.
func NewRouter(db *sqlx.DB, st store.Store, registry *node.NodeRegistry, dispatcher trigger.Dispatcher, bus *engine.EventBus, tm *trigger.Manager, level *slog.LevelVar) *http.ServeMux {
	mux := http.NewServeMux()

	// Infrastructure endpoints — unversioned.
	mux.Handle("GET /health", newHealthHandler(db))
	llh := &logLevelHandler{level: level}
	mux.HandleFunc("GET /admin/log-level", llh.get)
	mux.HandleFunc("PUT /admin/log-level", llh.set)

	// v1 API.
	wh := &workflowHandler{store: st, registry: registry, triggers: tm}
	mux.HandleFunc("GET /v1/workflows", wh.list)
	mux.HandleFunc("POST /v1/workflows", wh.create)
	mux.HandleFunc("GET /v1/workflows/{id}", wh.get)
	mux.HandleFunc("PUT /v1/workflows/{id}", wh.update)
	mux.HandleFunc("DELETE /v1/workflows/{id}", wh.delete)

	nth := &nodeTypeHandler{registry: registry}
	mux.HandleFunc("GET /v1/node-types", nth.list)

	rh := &runHandler{store: st, dispatcher: dispatcher}
	mux.HandleFunc("POST /v1/workflows/{id}/runs", rh.triggerRun)
	mux.HandleFunc("GET /v1/workflows/{id}/runs", rh.listRuns)
	mux.HandleFunc("GET /v1/runs/{run_id}", rh.getRun)

	wsh := &wsHandler{store: st, bus: bus}
	mux.HandleFunc("GET /v1/runs/{run_id}/events", wsh.streamEvents)

	mux.HandleFunc("POST /v1/webhooks/{workflow_id}", tm.WebhookHandler())

	pah := &pluginAdminHandler{store: st, registry: registry}
	mux.HandleFunc("GET /v1/admin/plugins", pah.list)
	mux.HandleFunc("POST /v1/admin/plugins", pah.register)
	mux.HandleFunc("PUT /v1/admin/plugins/{type_id}", pah.update)
	mux.HandleFunc("DELETE /v1/admin/plugins/{type_id}", pah.deregister)

	return mux
}
