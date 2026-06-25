package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// ---- Organizations ----------------------------------------------------------

func (s *WorkflowStore) CreateOrganization(ctx context.Context, org store.Organization) (store.Organization, error) {
	if org.ID == "" {
		org.ID = newUUID()
	}
	org.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO organizations (id, name, created_at) VALUES (?, ?, ?)`,
		org.ID, org.Name, org.CreatedAt)
	if err != nil {
		return store.Organization{}, fmt.Errorf("auth store: create organization: %w", err)
	}
	return org, nil
}

func (s *WorkflowStore) GetOrganization(ctx context.Context, id string) (store.Organization, error) {
	var row dbOrganization
	err := s.db.GetContext(ctx, &row,
		`SELECT id, name, created_at FROM organizations WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Organization{}, store.ErrNotFound
	}
	if err != nil {
		return store.Organization{}, fmt.Errorf("auth store: get organization: %w", err)
	}
	return row.toOrganization(), nil
}

func (s *WorkflowStore) ListOrganizations(ctx context.Context) ([]store.Organization, error) {
	var rows []dbOrganization
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT id, name, created_at FROM organizations ORDER BY name`); err != nil {
		return nil, fmt.Errorf("auth store: list organizations: %w", err)
	}
	orgs := make([]store.Organization, len(rows))
	for i, r := range rows {
		orgs[i] = r.toOrganization()
	}
	return orgs, nil
}

func (s *WorkflowStore) DeleteOrganization(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("auth store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete org resources in dependency order (no FK constraints — app layer).
	tables := []string{"users", "invitations", "eval_suites", "rag_documents", "org_email_settings"}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t+" WHERE org_id = ?", id); err != nil {
			return fmt.Errorf("auth store: delete org %s from %s: %w", id, t, err)
		}
	}
	// Workflows: delete child rows first.
	var wfIDs []string
	if err := tx.SelectContext(ctx, &wfIDs, `SELECT id FROM workflows WHERE org_id = ?`, id); err != nil {
		return fmt.Errorf("auth store: list org workflows: %w", err)
	}
	for _, wfID := range wfIDs {
		if err := deleteNodeConfigs(ctx, tx, wfID); err != nil {
			return err
		}
		for _, tbl := range []string{"workflow_nodes", "workflow_edges", "runs", "workflow_versions"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+tbl+" WHERE workflow_id = ?", wfID); err != nil {
				return fmt.Errorf("auth store: delete %s for workflow %s: %w", tbl, wfID, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflows WHERE org_id = ?`, id); err != nil {
		return fmt.Errorf("auth store: delete org workflows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM organizations WHERE id = ?`, id); err != nil {
		return fmt.Errorf("auth store: delete organization: %w", err)
	}

	return tx.Commit()
}

// ---- Users ------------------------------------------------------------------

func (s *WorkflowStore) CreateUser(ctx context.Context, u store.User) (store.User, error) {
	if u.ID == "" {
		u.ID = newUUID()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	if len(u.Permissions) == 0 {
		u.Permissions = store.DefaultPermissions
	}
	permsJSON, err := json.Marshal(u.Permissions)
	if err != nil {
		return store.User{}, fmt.Errorf("auth store: marshal permissions: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users (id, org_id, email, password_hash, role, permissions, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.OrgID, u.Email, u.PasswordHash, u.Role, string(permsJSON), u.CreatedAt, u.UpdatedAt)
	if err != nil {
		if isDuplicateKey(err) {
			return store.User{}, store.ErrDuplicateEmail
		}
		return store.User{}, fmt.Errorf("auth store: create user: %w", err)
	}
	return u, nil
}

func (s *WorkflowStore) GetUser(ctx context.Context, id string) (store.User, error) {
	var row dbUser
	err := s.db.GetContext(ctx, &row,
		`SELECT id, org_id, email, password_hash, role, permissions, created_at, updated_at
		 FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, fmt.Errorf("auth store: get user: %w", err)
	}
	return row.toUser()
}

func (s *WorkflowStore) GetUserByEmail(ctx context.Context, email string) (store.User, error) {
	var row dbUser
	err := s.db.GetContext(ctx, &row,
		`SELECT id, org_id, email, password_hash, role, permissions, created_at, updated_at
		 FROM users WHERE email = ?`, email)
	if errors.Is(err, sql.ErrNoRows) {
		return store.User{}, store.ErrNotFound
	}
	if err != nil {
		return store.User{}, fmt.Errorf("auth store: get user by email: %w", err)
	}
	return row.toUser()
}

func (s *WorkflowStore) ListUsers(ctx context.Context, orgID string) ([]store.User, error) {
	var rows []dbUser
	var err error
	if orgID == "" {
		err = s.db.SelectContext(ctx, &rows,
			`SELECT id, org_id, email, password_hash, role, permissions, created_at, updated_at
			 FROM users ORDER BY email`)
	} else {
		err = s.db.SelectContext(ctx, &rows,
			`SELECT id, org_id, email, password_hash, role, permissions, created_at, updated_at
			 FROM users WHERE org_id = ? ORDER BY email`, orgID)
	}
	if err != nil {
		return nil, fmt.Errorf("auth store: list users: %w", err)
	}
	users := make([]store.User, 0, len(rows))
	for _, r := range rows {
		u, err := r.toUser()
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *WorkflowStore) UpdateUserRole(ctx context.Context, userID, role string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ? WHERE id = ?`, role, userID)
	if err != nil {
		return fmt.Errorf("auth store: update user role: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *WorkflowStore) UpdateUserPermissions(ctx context.Context, userID string, permissions []string) error {
	b, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("auth store: marshal permissions: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET permissions = ? WHERE id = ?`, string(b), userID)
	if err != nil {
		return fmt.Errorf("auth store: update user permissions: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *WorkflowStore) DeleteUser(ctx context.Context, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return fmt.Errorf("auth store: delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- Invitations ------------------------------------------------------------

func (s *WorkflowStore) CreateInvitation(ctx context.Context, inv store.Invitation) (store.Invitation, error) {
	if inv.ID == "" {
		inv.ID = newUUID()
	}
	inv.CreatedAt = time.Now().UTC()
	permsJSON, err := json.Marshal(inv.Permissions)
	if err != nil {
		return store.Invitation{}, fmt.Errorf("auth store: marshal permissions: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO invitations (id, org_id, email, role, permissions, token, invited_by, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.OrgID, inv.Email, inv.Role, string(permsJSON),
		inv.Token, inv.InvitedBy, inv.ExpiresAt, inv.CreatedAt)
	if err != nil {
		return store.Invitation{}, fmt.Errorf("auth store: create invitation: %w", err)
	}
	return inv, nil
}

func (s *WorkflowStore) GetInvitationByToken(ctx context.Context, token string) (store.Invitation, error) {
	var row dbInvitation
	err := s.db.GetContext(ctx, &row,
		`SELECT id, org_id, email, role, permissions, token, invited_by, expires_at, accepted_at, created_at
		 FROM invitations WHERE token = ?`, token)
	if errors.Is(err, sql.ErrNoRows) {
		return store.Invitation{}, store.ErrNotFound
	}
	if err != nil {
		return store.Invitation{}, fmt.Errorf("auth store: get invitation: %w", err)
	}
	return row.toInvitation()
}

func (s *WorkflowStore) AcceptInvitation(ctx context.Context, invID string, now time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE invitations SET accepted_at = ? WHERE id = ? AND accepted_at IS NULL`,
		now, invID)
	if err != nil {
		return fmt.Errorf("auth store: accept invitation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- DB row types -----------------------------------------------------------

type dbOrganization struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

func (r dbOrganization) toOrganization() store.Organization {
	return store.Organization{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt}
}

type dbUser struct {
	ID           string    `db:"id"`
	OrgID        string    `db:"org_id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	Role         string    `db:"role"`
	Permissions  string    `db:"permissions"` // JSON
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

func (r dbUser) toUser() (store.User, error) {
	var perms []string
	if err := json.Unmarshal([]byte(r.Permissions), &perms); err != nil {
		return store.User{}, fmt.Errorf("auth store: unmarshal permissions for user %s: %w", r.ID, err)
	}
	return store.User{
		ID:           r.ID,
		OrgID:        r.OrgID,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Role:         r.Role,
		Permissions:  perms,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}, nil
}

type dbInvitation struct {
	ID          string     `db:"id"`
	OrgID       string     `db:"org_id"`
	Email       string     `db:"email"`
	Role        string     `db:"role"`
	Permissions string     `db:"permissions"` // JSON
	Token       string     `db:"token"`
	InvitedBy   string     `db:"invited_by"`
	ExpiresAt   time.Time  `db:"expires_at"`
	AcceptedAt  *time.Time `db:"accepted_at"`
	CreatedAt   time.Time  `db:"created_at"`
}

func (r dbInvitation) toInvitation() (store.Invitation, error) {
	var perms []string
	if err := json.Unmarshal([]byte(r.Permissions), &perms); err != nil {
		return store.Invitation{}, fmt.Errorf("auth store: unmarshal permissions for invitation %s: %w", r.ID, err)
	}
	return store.Invitation{
		ID:          r.ID,
		OrgID:       r.OrgID,
		Email:       r.Email,
		Role:        r.Role,
		Permissions: perms,
		Token:       r.Token,
		InvitedBy:   r.InvitedBy,
		ExpiresAt:   r.ExpiresAt,
		AcceptedAt:  r.AcceptedAt,
		CreatedAt:   r.CreatedAt,
	}, nil
}

// isDuplicateKey returns true when the error indicates a duplicate unique key violation.
// Matches MySQL ("Duplicate entry") and SQLite ("UNIQUE constraint failed") error messages.
func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate entry") || strings.Contains(msg, "UNIQUE constraint failed")
}
