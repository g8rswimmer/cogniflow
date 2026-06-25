package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/auth"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

type authHandler struct {
	store     store.Store
	jwtSecret []byte
	jwtTTL    time.Duration
}

// login handles POST /v1/auth/login.
func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "email and password are required")
		return
	}

	user, err := h.store.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		// Deliberate: same error for "not found" and "wrong password" to avoid enumeration.
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}
	if err := auth.CheckPassword(user.PasswordHash, body.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}

	org, err := h.store.GetOrganization(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load organization")
		return
	}

	token, err := auth.Sign(auth.Claims{
		UserID:      user.ID,
		OrgID:       user.OrgID,
		Email:       user.Email,
		Role:        user.Role,
		OrgName:     org.Name,
		Permissions: user.Permissions,
	}, h.jwtSecret, h.jwtTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to issue token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  toUserResponse(user, org.Name),
	})
}

// me handles GET /v1/auth/me (authenticated).
func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthenticated")
		return
	}

	user, err := h.store.GetUser(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	org, err := h.store.GetOrganization(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load organization")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user, org.Name))
}

// getInvite handles GET /v1/auth/invite/{token} (public — preview before accepting).
func (h *authHandler) getInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	inv, err := h.store.GetInvitationByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "invitation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if inv.AcceptedAt != nil {
		writeError(w, http.StatusGone, "INVITATION_USED", "invitation already accepted")
		return
	}
	if time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusGone, "INVITATION_EXPIRED", "invitation has expired")
		return
	}
	org, err := h.store.GetOrganization(r.Context(), inv.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load organization")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":    inv.Email,
		"role":     inv.Role,
		"org_name": org.Name,
	})
}

// acceptInvite handles POST /v1/auth/accept-invite (public).
func (h *authHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body")
		return
	}
	if body.Token == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "token and password are required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "password must be at least 8 characters")
		return
	}

	inv, err := h.store.GetInvitationByToken(r.Context(), body.Token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "invitation not found or already used")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if inv.AcceptedAt != nil {
		writeError(w, http.StatusConflict, "INVITATION_USED", "invitation already accepted")
		return
	}
	if time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusGone, "INVITATION_EXPIRED", "invitation has expired")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	now := time.Now().UTC()
	user, err := h.store.CreateUser(r.Context(), store.User{
		OrgID:        inv.OrgID,
		Email:        inv.Email,
		PasswordHash: hash,
		Role:         inv.Role,
		Permissions:  inv.Permissions,
	})
	if err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "EMAIL_IN_USE", "an account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if err := h.store.AcceptInvitation(r.Context(), inv.ID, now); err != nil {
		// Non-fatal: user is already created. Log so ops can manually clean up
		// the invitation row, which would otherwise remain reusable until expiry.
		slog.WarnContext(r.Context(), "failed to mark invitation accepted; token remains reusable until expiry",
			"invitation_id", inv.ID, "error", err)
	}

	org, err := h.store.GetOrganization(r.Context(), user.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load organization")
		return
	}

	token, err := auth.Sign(auth.Claims{
		UserID:      user.ID,
		OrgID:       user.OrgID,
		Email:       user.Email,
		Role:        user.Role,
		OrgName:     org.Name,
		Permissions: user.Permissions,
	}, h.jwtSecret, h.jwtTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to issue token")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user":  toUserResponse(user, org.Name),
	})
}

// ---- response helpers -------------------------------------------------------

type userResponse struct {
	ID          string   `json:"id"`
	OrgID       string   `json:"org_id"`
	OrgName     string   `json:"org_name"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	CreatedAt   any      `json:"created_at"`
}

func toUserResponse(u store.User, orgName string) userResponse {
	perms := u.Permissions
	if perms == nil {
		perms = []string{}
	}
	return userResponse{
		ID:          u.ID,
		OrgID:       u.OrgID,
		OrgName:     orgName,
		Email:       u.Email,
		Role:        u.Role,
		Permissions: perms,
		CreatedAt:   u.CreatedAt,
	}
}

// newInviteToken generates a cryptographically random 32-byte hex token.
func newInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
