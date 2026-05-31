package api

import (
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type healthHandler struct {
	db        *sqlx.DB
	startTime time.Time
}

func newHealthHandler(db *sqlx.DB) *healthHandler {
	return &healthHandler{db: db, startTime: time.Now()}
}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.PingContext(r.Context()); err != nil {
		dbStatus = "error"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"db":     dbStatus,
		"uptime": int(time.Since(h.startTime).Seconds()),
	})
}
