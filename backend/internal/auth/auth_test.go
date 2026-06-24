package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/auth"
)

var testSecret = []byte("test-secret-that-is-at-least-32-bytes-long")

// ── token tests ──────────────────────────────────────────────────────────────

func TestSignVerifyRoundTrip(t *testing.T) {
	claims := auth.Claims{
		UserID:      "user-1",
		OrgID:       "org-1",
		Email:       "test@example.com",
		Role:        "member",
		Permissions: []string{"workflow:read", "workflow:run"},
	}
	token, err := auth.Sign(claims, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if token == "" {
		t.Fatal("Sign returned empty token")
	}

	got, err := auth.Verify(token, testSecret)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.UserID != claims.UserID {
		t.Errorf("UserID: want %q, got %q", claims.UserID, got.UserID)
	}
	if got.OrgID != claims.OrgID {
		t.Errorf("OrgID: want %q, got %q", claims.OrgID, got.OrgID)
	}
	if got.Email != claims.Email {
		t.Errorf("Email: want %q, got %q", claims.Email, got.Email)
	}
	if got.Role != claims.Role {
		t.Errorf("Role: want %q, got %q", claims.Role, got.Role)
	}
}

func TestVerifyExpiredToken(t *testing.T) {
	claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: "member"}
	token, err := auth.Sign(claims, testSecret, -time.Second) // already expired
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	_, err = auth.Verify(token, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: "member"}
	token, err := auth.Sign(claims, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	_, err = auth.Verify(token, []byte("wrong-secret-that-is-at-least-32-bytes!"))
	if err == nil {
		t.Fatal("expected error for wrong secret, got nil")
	}
}

func TestVerifyMalformedToken(t *testing.T) {
	_, err := auth.Verify("not.a.jwt", testSecret)
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

// ── password tests ───────────────────────────────────────────────────────────

func TestHashPasswordAndCheck(t *testing.T) {
	plain := "secure-password-123"
	hash, err := auth.HashPassword(plain)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}
	if hash == plain {
		t.Fatal("hash should not equal plaintext")
	}

	if err := auth.CheckPassword(hash, plain); err != nil {
		t.Errorf("CheckPassword with correct password: %v", err)
	}
}

func TestCheckPasswordWrong(t *testing.T) {
	hash, _ := auth.HashPassword("correct-password")
	err := auth.CheckPassword(hash, "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

// ── middleware tests ──────────────────────────────────────────────────────────

func makeToken(t *testing.T, claims auth.Claims, ttl time.Duration) string {
	t.Helper()
	tok, err := auth.Sign(claims, testSecret, ttl)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return tok
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func TestAuthenticateMiddleware(t *testing.T) {
	mw := auth.Authenticate(testSecret)

	t.Run("valid token passes", func(t *testing.T) {
		claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: "member"}
		tok := makeToken(t, claims, time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
	})

	t.Run("missing header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})

	t.Run("expired token returns 401", func(t *testing.T) {
		claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: "member"}
		tok := makeToken(t, claims, -time.Second)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("want 401, got %d", rr.Code)
		}
	})
}

func TestRequireRole(t *testing.T) {
	store := func(role string) *http.Request {
		claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: role}
		tok := makeToken(t, claims, time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		// run through Authenticate so claims land in context
		var captured *http.Request
		auth.Authenticate(testSecret)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			captured = r
		})).ServeHTTP(httptest.NewRecorder(), req)
		return captured
	}

	mw := auth.RequireRole("org_admin", "system_admin")

	t.Run("allowed role passes", func(t *testing.T) {
		req := store("org_admin")
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
	})

	t.Run("wrong role returns 403", func(t *testing.T) {
		req := store("member")
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("want 403, got %d", rr.Code)
		}
	})
}

func TestRequirePermission(t *testing.T) {
	makeReq := func(role string, perms []string) *http.Request {
		claims := auth.Claims{UserID: "u1", OrgID: "o1", Role: role, Permissions: perms}
		tok := makeToken(t, claims, time.Hour)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		var captured *http.Request
		auth.Authenticate(testSecret)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			captured = r
		})).ServeHTTP(httptest.NewRecorder(), req)
		return captured
	}

	mw := auth.RequirePermission("workflow:write")

	t.Run("member with scope passes", func(t *testing.T) {
		req := makeReq("member", []string{"workflow:read", "workflow:write"})
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
	})

	t.Run("member without scope returns 403", func(t *testing.T) {
		req := makeReq("member", []string{"workflow:read"})
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("want 403, got %d", rr.Code)
		}
	})

	t.Run("org_admin bypasses permission check", func(t *testing.T) {
		req := makeReq("org_admin", nil)
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
	})

	t.Run("system_admin bypasses permission check", func(t *testing.T) {
		req := makeReq("system_admin", nil)
		rr := httptest.NewRecorder()
		mw(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
	})
}

func TestClaimsFrom(t *testing.T) {
	want := auth.Claims{UserID: "u1", OrgID: "o1", Role: "member"}
	tok := makeToken(t, want, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	var got auth.Claims
	auth.Authenticate(testSecret)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		c, ok := auth.ClaimsFrom(r.Context())
		if !ok {
			t.Error("ClaimsFrom returned false")
		}
		got = c
	})).ServeHTTP(httptest.NewRecorder(), req)

	if got.UserID != want.UserID {
		t.Errorf("UserID: want %q, got %q", want.UserID, got.UserID)
	}
}

func TestOrgIDFrom(t *testing.T) {
	claims := auth.Claims{UserID: "u1", OrgID: "org-abc", Role: "member"}
	tok := makeToken(t, claims, time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	var orgID string
	auth.Authenticate(testSecret)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		orgID = auth.OrgIDFrom(r.Context())
	})).ServeHTTP(httptest.NewRecorder(), req)

	if orgID != "org-abc" {
		t.Errorf("OrgIDFrom: want %q, got %q", "org-abc", orgID)
	}
}

func TestOrgIDFromEmptyContext(t *testing.T) {
	orgID := auth.OrgIDFrom(context.Background())
	if orgID != "" {
		t.Errorf("expected empty string from bare context, got %q", orgID)
	}
}

func TestHasPermission(t *testing.T) {
	cases := []struct {
		name   string
		claims auth.Claims
		scope  string
		want   bool
	}{
		{"system_admin always true", auth.Claims{Role: "system_admin"}, "workflow:write", true},
		{"org_admin always true", auth.Claims{Role: "org_admin"}, "eval:run", true},
		{"member with scope", auth.Claims{Role: "member", Permissions: []string{"workflow:read"}}, "workflow:read", true},
		{"member without scope", auth.Claims{Role: "member", Permissions: []string{"workflow:read"}}, "workflow:write", false},
		{"member empty perms", auth.Claims{Role: "member", Permissions: nil}, "workflow:read", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auth.HasPermission(tc.claims, tc.scope)
			if got != tc.want {
				t.Errorf("HasPermission(%v, %q) = %v, want %v", tc.claims.Role, tc.scope, got, tc.want)
			}
		})
	}
}

// Ensure JSON in error bodies contains the expected structure.
func TestAuthenticateMiddlewareResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	auth.Authenticate(testSecret)(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "UNAUTHORIZED") {
		t.Errorf("body %q does not contain UNAUTHORIZED", body)
	}
}
