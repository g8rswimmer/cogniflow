package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/eval/grader_plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

type graderPluginAdminHandler struct {
	store      store.Store
	registry   *grader_plugin.GraderRegistry
	// registerFn is injected so tests can stub out the gRPC dial without a
	// real server. Production code sets this to grader_plugin.RegisterOne.
	registerFn func(ctx context.Context, addr string, registry *grader_plugin.GraderRegistry) (store.GraderRegistration, error)
}

// list handles GET /v1/admin/grader-plugins.
func (h *graderPluginAdminHandler) list(w http.ResponseWriter, r *http.Request) {
	regs, err := h.store.ListGraderRegistrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if regs == nil {
		regs = []store.GraderRegistration{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"grader_plugins": regs})
}

// register handles POST /v1/admin/grader-plugins.
func (h *graderPluginAdminHandler) register(w http.ResponseWriter, r *http.Request) {
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

	reg, err := h.registerFn(r.Context(), body.Address, h.registry)
	if err != nil {
		if errors.Is(err, grader_plugin.ErrGraderAlreadyRegistered) {
			writeError(w, http.StatusConflict, "GRADER_ALREADY_REGISTERED", err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "GRADER_UNAVAILABLE", err.Error())
		return
	}

	if err := h.store.SaveGraderRegistration(r.Context(), reg); err != nil {
		// Registration succeeded but persistence failed — undo the registry entry
		// so the in-memory state and DB stay in sync.
		_ = h.registry.Unregister(reg.TypeID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			fmt.Sprintf("persisting registration: %s", err.Error()))
		return
	}

	writeJSON(w, http.StatusCreated, reg)
}

// update handles PUT /v1/admin/grader-plugins/{type_id}.
func (h *graderPluginAdminHandler) update(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	typeID := r.PathValue("type_id")

	if _, err := h.store.GetGraderRegistration(r.Context(), typeID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "no persisted grader plugin with type_id: "+typeID)
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

	reg, err := grader_plugin.UpdateOne(r.Context(), typeID, body.Address, h.registry)
	if err != nil {
		if errors.Is(err, grader_plugin.ErrTypeIDMismatch) {
			writeError(w, http.StatusUnprocessableEntity, "TYPE_ID_MISMATCH", err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "GRADER_UNAVAILABLE", err.Error())
		return
	}

	if err := h.store.SaveGraderRegistration(r.Context(), reg); err != nil {
		_ = h.registry.Unregister(reg.TypeID)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR",
			fmt.Sprintf("persisting registration: %s", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, reg)
}

// deregister handles DELETE /v1/admin/grader-plugins/{type_id}.
func (h *graderPluginAdminHandler) deregister(w http.ResponseWriter, r *http.Request) {
	typeID := r.PathValue("type_id")

	if _, err := h.store.GetGraderRegistration(r.Context(), typeID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "no persisted grader plugin with type_id: "+typeID)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if err := h.registry.Unregister(typeID); err != nil && !errors.Is(err, grader_plugin.ErrGraderNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if err := h.store.DeleteGraderRegistration(r.Context(), typeID); err != nil &&
		!errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
