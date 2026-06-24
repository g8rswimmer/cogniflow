package auth

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

type ctxKey string

const claimsCtxKey ctxKey = "auth_claims"

// Authenticate is middleware that validates a Bearer JWT on every request.
// On success the Claims are stored in the request context; on failure a 401 is returned.
func Authenticate(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				writeJSON401(w, "missing or invalid Authorization header")
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")
			claims, err := Verify(tokenStr, jwtSecret)
			if err != nil {
				slog.Debug("auth: token verification failed", "error", err)
				writeJSON401(w, "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns 403 if the authenticated user's role is not in the allowed list.
// Must be applied after Authenticate.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFrom(r.Context())
			if !ok {
				writeJSON401(w, "unauthenticated")
				return
			}
			if !slices.Contains(roles, claims.Role) {
				writeJSON403(w, "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission returns 403 if the user is a member who does NOT have the named scope.
// system_admin and org_admin always pass regardless of their permissions list.
func RequirePermission(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFrom(r.Context())
			if !ok {
				writeJSON401(w, "unauthenticated")
				return
			}
			if !HasPermission(claims, scope) {
				writeJSON403(w, "missing permission: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HasPermission returns true if the user has the named scope.
// system_admin and org_admin bypass permission checks entirely.
func HasPermission(claims Claims, scope string) bool {
	if claims.Role == "system_admin" || claims.Role == "org_admin" {
		return true
	}
	return slices.Contains(claims.Permissions, scope)
}

// ClaimsFrom retrieves the JWT claims stored by Authenticate middleware.
func ClaimsFrom(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsCtxKey).(Claims)
	return c, ok
}

// OrgIDFrom extracts the org_id from the JWT claims in the context.
func OrgIDFrom(ctx context.Context) string {
	c, _ := ctx.Value(claimsCtxKey).(Claims)
	return c.OrgID
}

// RoleFrom extracts the role from the JWT claims in the context.
func RoleFrom(ctx context.Context) string {
	c, _ := ctx.Value(claimsCtxKey).(Claims)
	return c.Role
}

func writeJSON401(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"` + jsonEscape(msg) + `"}}`))
}

func writeJSON403(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":{"code":"FORBIDDEN","message":"` + jsonEscape(msg) + `"}}`))
}

// jsonEscape performs minimal escaping of a string for embedding in a JSON literal.
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
