package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

type logLevelHandler struct {
	level *slog.LevelVar
}

// get handles GET /admin/log-level — returns the current log level.
func (h *logLevelHandler) get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"level": h.level.Level().String()})
}

// set handles PUT /admin/log-level — updates the log level without a restart.
// Body: {"level": "debug"|"info"|"warn"|"error"}
func (h *logLevelHandler) set(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}

	var newLevel slog.Level
	switch strings.ToLower(body.Level) {
	case "debug":
		newLevel = slog.LevelDebug
	case "info":
		newLevel = slog.LevelInfo
	case "warn":
		newLevel = slog.LevelWarn
	case "error":
		newLevel = slog.LevelError
	default:
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "level must be one of: debug, info, warn, error")
		return
	}

	h.level.Set(newLevel)
	slog.Info("log level changed", "level", newLevel.String())
	writeJSON(w, http.StatusOK, map[string]string{"level": newLevel.String()})
}
