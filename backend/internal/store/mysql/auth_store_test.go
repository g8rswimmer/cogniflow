package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func TestOrganizationCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create
	org, err := s.CreateOrganization(ctx, store.Organization{Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.ID == "" {
		t.Error("expected non-empty ID")
	}
	if org.Name != "Acme" {
		t.Errorf("Name: want Acme, got %q", org.Name)
	}

	// Get
	got, err := s.GetOrganization(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if got.Name != org.Name {
		t.Errorf("Name mismatch: %q vs %q", org.Name, got.Name)
	}

	// List
	orgs, err := s.ListOrganizations(ctx)
	if err != nil {
		t.Fatalf("ListOrganizations: %v", err)
	}
	if len(orgs) < 1 {
		t.Fatal("expected at least 1 org")
	}

	// Get not found
	_, err = s.GetOrganization(ctx, "does-not-exist")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Delete
	if err := s.DeleteOrganization(ctx, org.ID); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}
	_, err = s.GetOrganization(ctx, org.ID)
	if err != store.ErrNotFound {
		t.Errorf("after delete: expected ErrNotFound, got %v", err)
	}
}

func TestDeleteOrganizationCascadesUsers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	org, _ := s.CreateOrganization(ctx, store.Organization{Name: "Cascade Org"})
	_, err := s.CreateUser(ctx, store.User{
		OrgID:        org.ID,
		Email:        "user@cascade.com",
		PasswordHash: "$bcrypt$",
		Role:         "member",
		Permissions:  store.DefaultPermissions,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.DeleteOrganization(ctx, org.ID); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}

	users, err := s.ListUsers(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListUsers after delete: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users after org delete, got %d", len(users))
	}
}

func TestUserCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	org, _ := s.CreateOrganization(ctx, store.Organization{Name: "Test Org"})

	// Create
	u, err := s.CreateUser(ctx, store.User{
		OrgID:        org.ID,
		Email:        "alice@example.com",
		PasswordHash: "hashed",
		Role:         "member",
		Permissions:  []string{"workflow:read", "workflow:run"},
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == "" {
		t.Error("expected non-empty user ID")
	}

	// Get by ID
	got, err := s.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Email != u.Email {
		t.Errorf("Email: want %q, got %q", u.Email, got.Email)
	}
	if len(got.Permissions) != 2 {
		t.Errorf("Permissions: want 2, got %d", len(got.Permissions))
	}

	// Get by email
	byEmail, err := s.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != u.ID {
		t.Errorf("GetUserByEmail ID mismatch")
	}

	// List
	users, err := s.ListUsers(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}

	// Update role
	if err := s.UpdateUserRole(ctx, u.ID, "org_admin"); err != nil {
		t.Fatalf("UpdateUserRole: %v", err)
	}
	updated, _ := s.GetUser(ctx, u.ID)
	if updated.Role != "org_admin" {
		t.Errorf("Role: want org_admin, got %q", updated.Role)
	}

	// Update permissions
	if err := s.UpdateUserPermissions(ctx, u.ID, store.DefaultPermissions); err != nil {
		t.Fatalf("UpdateUserPermissions: %v", err)
	}
	updated, _ = s.GetUser(ctx, u.ID)
	if len(updated.Permissions) != len(store.DefaultPermissions) {
		t.Errorf("Permissions count: want %d, got %d", len(store.DefaultPermissions), len(updated.Permissions))
	}

	// Delete
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	_, err = s.GetUser(ctx, u.ID)
	if err != store.ErrNotFound {
		t.Errorf("after delete: expected ErrNotFound, got %v", err)
	}
}

func TestListUsersAllOrgs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	org1, _ := s.CreateOrganization(ctx, store.Organization{Name: "Org1"})
	org2, _ := s.CreateOrganization(ctx, store.Organization{Name: "Org2"})

	_, _ = s.CreateUser(ctx, store.User{OrgID: org1.ID, Email: "a@org1.com", PasswordHash: "x", Role: "member", Permissions: store.DefaultPermissions})
	_, _ = s.CreateUser(ctx, store.User{OrgID: org2.ID, Email: "b@org2.com", PasswordHash: "x", Role: "member", Permissions: store.DefaultPermissions})

	// Empty orgID returns all.
	all, err := s.ListUsers(ctx, "")
	if err != nil {
		t.Fatalf("ListUsers all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 users, got %d", len(all))
	}

	// Scoped to org1.
	org1Users, err := s.ListUsers(ctx, org1.ID)
	if err != nil {
		t.Fatalf("ListUsers org1: %v", err)
	}
	if len(org1Users) != 1 {
		t.Errorf("expected 1 user in org1, got %d", len(org1Users))
	}
}

func TestDuplicateEmailError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	org, _ := s.CreateOrganization(ctx, store.Organization{Name: "Dup Org"})

	_, _ = s.CreateUser(ctx, store.User{OrgID: org.ID, Email: "dup@example.com", PasswordHash: "x", Role: "member", Permissions: store.DefaultPermissions})
	_, err := s.CreateUser(ctx, store.User{OrgID: org.ID, Email: "dup@example.com", PasswordHash: "x", Role: "member", Permissions: store.DefaultPermissions})
	if err != store.ErrDuplicateEmail {
		t.Errorf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestGetUserNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetUser(ctx, "no-such-id")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	_, err = s.GetUserByEmail(ctx, "nobody@example.com")
	if err != store.ErrNotFound {
		t.Errorf("GetUserByEmail: expected ErrNotFound, got %v", err)
	}
}

func TestInvitationFlow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	org, _ := s.CreateOrganization(ctx, store.Organization{Name: "Invite Org"})
	inviter, _ := s.CreateUser(ctx, store.User{OrgID: org.ID, Email: "admin@invite.com", PasswordHash: "x", Role: "org_admin", Permissions: store.DefaultPermissions})

	// Create invitation
	inv, err := s.CreateInvitation(ctx, store.Invitation{
		OrgID:       org.ID,
		Email:       "newuser@invite.com",
		Role:        "member",
		Permissions: store.DefaultPermissions,
		Token:       "tok-abc-123",
		InvitedBy:   inviter.ID,
		ExpiresAt:   time.Now().Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	if inv.ID == "" {
		t.Error("expected non-empty invitation ID")
	}

	// Retrieve by token
	found, err := s.GetInvitationByToken(ctx, "tok-abc-123")
	if err != nil {
		t.Fatalf("GetInvitationByToken: %v", err)
	}
	if found.Email != "newuser@invite.com" {
		t.Errorf("Email: want newuser@invite.com, got %q", found.Email)
	}
	if found.AcceptedAt != nil {
		t.Error("AcceptedAt should be nil before acceptance")
	}
	if len(found.Permissions) != len(store.DefaultPermissions) {
		t.Errorf("Permissions length: want %d, got %d", len(store.DefaultPermissions), len(found.Permissions))
	}

	// Accept
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.AcceptInvitation(ctx, inv.ID, now); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}

	accepted, err := s.GetInvitationByToken(ctx, "tok-abc-123")
	if err != nil {
		t.Fatalf("GetInvitationByToken after accept: %v", err)
	}
	if accepted.AcceptedAt == nil {
		t.Fatal("AcceptedAt should be non-nil after acceptance")
	}
	diff := accepted.AcceptedAt.Sub(now)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("AcceptedAt: want ~%v, got %v", now, accepted.AcceptedAt)
	}

	// Not found
	_, err = s.GetInvitationByToken(ctx, "no-such-token")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
