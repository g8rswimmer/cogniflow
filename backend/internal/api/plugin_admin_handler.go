package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/node"
	nodeplugin "github.com/g8rswimmer/cogniflow/internal/node/plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

type pluginAdminHandler struct {
	store    store.Store
	registry *node.NodeRegistry
}

// list handles GET /v1/admin/plugins.
func (h *pluginAdminHandler) list(w http.ResponseWriter, r *http.Request) {
	regs, err := h.store.ListPluginRegistrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": regs})
}

// register handles POST /v1/admin/plugins.
func (h *pluginAdminHandler) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if body.Address == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "address is required")
		return
	}

	reg, err := nodeplugin.RegisterOne(r.Context(), body.Address, h.registry)
	if err != nil {
		if errors.Is(err, node.ErrDuplicateTypeID) {
			writeError(w, http.StatusConflict, "PLUGIN_ALREADY_REGISTERED", err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "PLUGIN_UNAVAILABLE", err.Error())
		return
	}

	if err := h.store.SavePluginRegistration(r.Context(), reg); err != nil {
		// Registration succeeded but persistence failed — undo the registry entry
		// so the in-memory state and DB stay in sync.
		_ = h.registry.Unregister(reg.TypeID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			fmt.Sprintf("persisting registration: %s", err.Error()))
		return
	}

	writeJSON(w, http.StatusCreated, reg)
}

// update handles PUT /v1/admin/plugins/{type_id}.
func (h *pluginAdminHandler) update(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	typeID := r.PathValue("type_id")

	// Verify the typeID is known in the persistent store before touching the
	// registry. This prevents UpdateOne from silently creating a new entry for
	// a typeID that was never registered via the admin API.
	if _, err := h.store.GetPluginRegistration(r.Context(), typeID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "no persisted plugin with type_id: "+typeID)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var body struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if body.Address == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "address is required")
		return
	}

	reg, err := nodeplugin.UpdateOne(r.Context(), typeID, body.Address, h.registry)
	if err != nil {
		if errors.Is(err, nodeplugin.ErrTypeIDMismatch) {
			writeError(w, http.StatusUnprocessableEntity, "TYPE_ID_MISMATCH", err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "PLUGIN_UNAVAILABLE", err.Error())
		return
	}

	if err := h.store.SavePluginRegistration(r.Context(), reg); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			fmt.Sprintf("persisting registration: %s", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, reg)
}

// deregister handles DELETE /v1/admin/plugins/{type_id}.
func (h *pluginAdminHandler) deregister(w http.ResponseWriter, r *http.Request) {
	typeID := r.PathValue("type_id")

	if err := h.registry.Unregister(typeID); err != nil {
		if errors.Is(err, node.ErrNodeNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin not registered: "+typeID)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Remove from DB; ignore not-found in case it was an ephemeral (env-var) plugin.
	if err := h.store.DeletePluginRegistration(r.Context(), typeID); err != nil &&
		!errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
