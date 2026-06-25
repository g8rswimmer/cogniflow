package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/auth"
	"github.com/g8rswimmer/cogniflow/internal/email"
	"github.com/g8rswimmer/cogniflow/internal/store"
)


type userAdminHandler struct {
	store       store.Store
	jwtSecret   []byte
	jwtTTL      time.Duration
	frontendURL string
}

// ---- Org-admin endpoints (own org users) ------------------------------------

// listOrgUsers handles GET /v1/org/users.
func (h *userAdminHandler) listOrgUsers(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFrom(r.Context())
	orgID := claims.OrgID
	if claims.Role == "system_admin" {
		if q := r.URL.Query().Get("org_id"); q != "" {
			orgID = q
		}
	}

	users, err := h.store.ListUsers(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	org, err := h.store.GetOrganization(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load organization")
		return
	}

	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, toUserResponse(u, org.Name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": resp})
}

// inviteUser handles POST /v1/org/users/invite.
func (h *userAdminHandler) inviteUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	claims, _ := auth.ClaimsFrom(r.Context())

	var body struct {
		Email       string   `json:"email"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "email is required")
		return
	}
	if body.Role == "" {
		body.Role = "member"
	}
	if body.Role != "org_admin" && body.Role != "member" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "role must be org_admin or member")
		return
	}
	if len(body.Permissions) == 0 {
		body.Permissions = store.DefaultPermissions
	}

	token, err := newInviteToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate token")
		return
	}

	inv, err := h.store.CreateInvitation(r.Context(), store.Invitation{
		OrgID:       claims.OrgID,
		Email:       body.Email,
		Role:        body.Role,
		Permissions: body.Permissions,
		Token:       token,
		InvitedBy:   claims.UserID,
		ExpiresAt:   time.Now().UTC().Add(72 * time.Hour),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	emailSent := false
	if emailSettings, err := h.store.GetOrgEmailSettings(r.Context(), claims.OrgID); err == nil && emailSettings.SMTPHost != "" {
		sender := email.New(emailSettings.SMTPHost, emailSettings.SMTPPort, emailSettings.SMTPUser, emailSettings.SMTPPassword, emailSettings.SMTPFrom)
		org, _ := h.store.GetOrganization(r.Context(), claims.OrgID)
		inviteURL := fmt.Sprintf("%s/invite/%s", h.frontendURL, inv.Token)
		data := email.InviteData{
			OrgName:      org.Name,
			InviteURL:    inviteURL,
			InviteeEmail: inv.Email,
			InviterEmail: claims.Email,
			ExpiresAt:    inv.ExpiresAt,
		}
		if err := sender.SendInvite(inv.Email, data, emailSettings.Subject, emailSettings.Body); err != nil {
			slog.WarnContext(r.Context(), "failed to send invite email", "error", err, "to", inv.Email)
		} else {
			emailSent = true
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         inv.ID,
		"email":      inv.Email,
		"role":       inv.Role,
		"token":      inv.Token,
		"expires_at": inv.ExpiresAt,
		"email_sent": emailSent,
	})
}

// updateOrgUserRole handles PUT /v1/org/users/{id}/role.
func (h *userAdminHandler) updateOrgUserRole(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	claims, _ := auth.ClaimsFrom(r.Context())
	targetID := r.PathValue("id")

	if targetID == claims.UserID {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "cannot change your own role")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	// Org admins cannot set system_admin; only system_admin can do that.
	if body.Role == "system_admin" && claims.Role != "system_admin" {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "only system_admin can grant system_admin role")
		return
	}
	if body.Role == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "role is required")
		return
	}

	// Verify target user belongs to caller's org (unless system_admin).
	if claims.Role != "system_admin" {
		target, err := h.store.GetUser(r.Context(), targetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		if target.OrgID != claims.OrgID {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
	}

	if err := h.store.UpdateUserRole(r.Context(), targetID, body.Role); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateOrgUserPermissions handles PUT /v1/org/users/{id}/permissions.
func (h *userAdminHandler) updateOrgUserPermissions(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	claims, _ := auth.ClaimsFrom(r.Context())
	targetID := r.PathValue("id")

	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if body.Permissions == nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "permissions array is required")
		return
	}

	// Verify target belongs to caller's org (unless system_admin).
	if claims.Role != "system_admin" {
		target, err := h.store.GetUser(r.Context(), targetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		if target.OrgID != claims.OrgID {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
	}

	if err := h.store.UpdateUserPermissions(r.Context(), targetID, body.Permissions); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeOrgUser handles DELETE /v1/org/users/{id}.
func (h *userAdminHandler) removeOrgUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFrom(r.Context())
	targetID := r.PathValue("id")

	if targetID == claims.UserID {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "cannot remove yourself")
		return
	}

	// Verify target belongs to caller's org (unless system_admin).
	if claims.Role != "system_admin" {
		target, err := h.store.GetUser(r.Context(), targetID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		if target.OrgID != claims.OrgID {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
	}

	if err := h.store.DeleteUser(r.Context(), targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- System-admin endpoints -------------------------------------------------

// listOrgs handles GET /v1/admin/orgs.
func (h *userAdminHandler) listOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.store.ListOrganizations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if orgs == nil {
		orgs = []store.Organization{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": orgs})
}

// createOrg handles POST /v1/admin/orgs.
func (h *userAdminHandler) createOrg(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body struct {
		Name          string `json:"name"`
		AdminEmail    string `json:"admin_email"`
		AdminPassword string `json:"admin_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if body.Name == "" || body.AdminEmail == "" || body.AdminPassword == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "name, admin_email, and admin_password are required")
		return
	}
	if len(body.AdminPassword) < 8 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "admin_password must be at least 8 characters")
		return
	}

	org, err := h.store.CreateOrganization(r.Context(), store.Organization{Name: body.Name})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	hash, err := auth.HashPassword(body.AdminPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	admin, err := h.store.CreateUser(r.Context(), store.User{
		OrgID:        org.ID,
		Email:        body.AdminEmail,
		PasswordHash: hash,
		Role:         "org_admin",
		Permissions:  store.DefaultPermissions,
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "EMAIL_IN_USE", "an account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"organization": org,
		"admin":        toUserResponse(admin, org.Name),
	})
}

// deleteOrg handles DELETE /v1/admin/orgs/{id}.
func (h *userAdminHandler) deleteOrg(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteOrganization(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "organization not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listAllUsers handles GET /v1/admin/users.
func (h *userAdminHandler) listAllUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsers(r.Context(), "") // empty = all orgs
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Build a quick org name cache to avoid N+1 queries.
	orgNames := make(map[string]string)
	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		name, ok := orgNames[u.OrgID]
		if !ok {
			org, err := h.store.GetOrganization(r.Context(), u.OrgID)
			if err == nil {
				name = org.Name
			}
			orgNames[u.OrgID] = name
		}
		resp = append(resp, toUserResponse(u, name))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": resp})
}

// deleteUser handles DELETE /v1/admin/users/{id}.
func (h *userAdminHandler) deleteUser(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFrom(r.Context())
	targetID := r.PathValue("id")
	if targetID == claims.UserID {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "cannot delete yourself")
		return
	}
	if err := h.store.DeleteUser(r.Context(), targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
