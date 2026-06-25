package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/g8rswimmer/cogniflow/internal/auth"
	"github.com/g8rswimmer/cogniflow/internal/email"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const passwordMask = "***"

type orgEmailSettingsHandler struct {
	store store.Store
}

type orgEmailSettingsResponse struct {
	OrgID          string `json:"org_id"`
	SMTPHost       string `json:"smtp_host"`
	SMTPPort       string `json:"smtp_port"`
	SMTPUser       string `json:"smtp_user"`
	SMTPPassword   string `json:"smtp_password"` // "***" when set, "" when not set
	SMTPFrom       string `json:"smtp_from"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	SMTPConfigured bool   `json:"smtp_configured"`
	IsDefault      bool   `json:"is_default"`
}

func toSettingsResponse(s store.OrgEmailSettings, isDefault bool) orgEmailSettingsResponse {
	pwd := ""
	if s.SMTPPassword != "" {
		pwd = passwordMask
	}
	return orgEmailSettingsResponse{
		OrgID:          s.OrgID,
		SMTPHost:       s.SMTPHost,
		SMTPPort:       s.SMTPPort,
		SMTPUser:       s.SMTPUser,
		SMTPPassword:   pwd,
		SMTPFrom:       s.SMTPFrom,
		Subject:        s.Subject,
		Body:           s.Body,
		SMTPConfigured: s.SMTPHost != "",
		IsDefault:      isDefault,
	}
}

// getOrgEmailSettings handles GET /v1/org/email-settings.
func (h *orgEmailSettingsHandler) getOrgEmailSettings(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFrom(r.Context())

	settings, err := h.store.GetOrgEmailSettings(r.Context(), claims.OrgID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, toSettingsResponse(store.OrgEmailSettings{
			OrgID:    claims.OrgID,
			SMTPPort: "587",
			Subject:  email.DefaultSubject,
			Body:     email.DefaultBody,
		}, true))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	resp := toSettingsResponse(settings, false)
	if settings.Subject == "" {
		resp.Subject = email.DefaultSubject
	}
	if settings.Body == "" {
		resp.Body = email.DefaultBody
	}
	writeJSON(w, http.StatusOK, resp)
}

// upsertOrgEmailSettings handles PUT /v1/org/email-settings.
func (h *orgEmailSettingsHandler) upsertOrgEmailSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	claims, _ := auth.ClaimsFrom(r.Context())

	var body struct {
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     string `json:"smtp_port"`
		SMTPUser     string `json:"smtp_user"`
		SMTPPassword string `json:"smtp_password"`
		SMTPFrom     string `json:"smtp_from"`
		Subject      string `json:"subject"`
		Body         string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}

	// Validate Go templates when non-empty.
	if body.Subject != "" {
		if err := email.ParseTemplate(body.Subject); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_TEMPLATE",
				"invalid subject template: "+err.Error())
			return
		}
	}
	if body.Body != "" {
		if err := email.ParseTemplate(body.Body); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_TEMPLATE",
				"invalid body template: "+err.Error())
			return
		}
	}

	// If the client sent "***", keep the existing password rather than overwriting.
	password := body.SMTPPassword
	if password == passwordMask {
		existing, err := h.store.GetOrgEmailSettings(r.Context(), claims.OrgID)
		if err == nil {
			password = existing.SMTPPassword
		} else {
			password = ""
		}
	}

	port := body.SMTPPort
	if port == "" {
		port = "587"
	}

	settings := store.OrgEmailSettings{
		OrgID:        claims.OrgID,
		SMTPHost:     body.SMTPHost,
		SMTPPort:     port,
		SMTPUser:     body.SMTPUser,
		SMTPPassword: password,
		SMTPFrom:     body.SMTPFrom,
		Subject:      body.Subject,
		Body:         body.Body,
	}
	if err := h.store.UpsertOrgEmailSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	saved, _ := h.store.GetOrgEmailSettings(r.Context(), claims.OrgID)
	writeJSON(w, http.StatusOK, toSettingsResponse(saved, false))
}

// deleteOrgEmailSettings handles DELETE /v1/org/email-settings.
func (h *orgEmailSettingsHandler) deleteOrgEmailSettings(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFrom(r.Context())
	if err := h.store.DeleteOrgEmailSettings(r.Context(), claims.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// upsertOrgEmailSettingsAdmin handles PUT /v1/admin/orgs/{org_id}/email-settings.
// Allows system_admin to configure email settings for any org.
func (h *orgEmailSettingsHandler) upsertOrgEmailSettingsAdmin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	orgID := r.PathValue("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "missing org_id")
		return
	}

	var body struct {
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     string `json:"smtp_port"`
		SMTPUser     string `json:"smtp_user"`
		SMTPPassword string `json:"smtp_password"`
		SMTPFrom     string `json:"smtp_from"`
		Subject      string `json:"subject"`
		Body         string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}

	if body.Subject != "" {
		if err := email.ParseTemplate(body.Subject); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_TEMPLATE",
				"invalid subject template: "+err.Error())
			return
		}
	}
	if body.Body != "" {
		if err := email.ParseTemplate(body.Body); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_TEMPLATE",
				"invalid body template: "+err.Error())
			return
		}
	}

	port := body.SMTPPort
	if port == "" {
		port = "587"
	}

	settings := store.OrgEmailSettings{
		OrgID:        orgID,
		SMTPHost:     body.SMTPHost,
		SMTPPort:     port,
		SMTPUser:     body.SMTPUser,
		SMTPPassword: body.SMTPPassword,
		SMTPFrom:     body.SMTPFrom,
		Subject:      body.Subject,
		Body:         body.Body,
	}
	if err := h.store.UpsertOrgEmailSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	saved, _ := h.store.GetOrgEmailSettings(r.Context(), orgID)
	writeJSON(w, http.StatusOK, toSettingsResponse(saved, false))
}
