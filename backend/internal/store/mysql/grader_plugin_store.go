package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

type graderRegistrationRow struct {
	TypeID       string    `db:"type_id"`
	Address      string    `db:"address"`
	DisplayName  string    `db:"display_name"`
	Description  string    `db:"description"`
	ConfigSchema []byte    `db:"config_schema"`
	RegisteredAt time.Time `db:"registered_at"`
}

// SaveGraderRegistration inserts or replaces a grader plugin registration row.
func (s *WorkflowStore) SaveGraderRegistration(ctx context.Context, reg store.GraderRegistration) error {
	_, err := s.db.NamedExecContext(ctx, `
		INSERT INTO grader_registrations
		    (type_id, address, display_name, description, config_schema)
		VALUES
		    (:type_id, :address, :display_name, :description, :config_schema)
		ON DUPLICATE KEY UPDATE
		    address       = VALUES(address),
		    display_name  = VALUES(display_name),
		    description   = VALUES(description),
		    config_schema = VALUES(config_schema)`,
		graderRegistrationRow{
			TypeID:       reg.TypeID,
			Address:      reg.Address,
			DisplayName:  reg.DisplayName,
			Description:  reg.Description,
			ConfigSchema: []byte(reg.ConfigSchema),
		},
	)
	if err != nil {
		return fmt.Errorf("grader plugin store: save %q: %w", reg.TypeID, err)
	}
	return nil
}

// GetGraderRegistration returns the registration for the given TypeID.
func (s *WorkflowStore) GetGraderRegistration(ctx context.Context, typeID string) (store.GraderRegistration, error) {
	var row graderRegistrationRow
	err := s.db.GetContext(ctx, &row, `
		SELECT type_id, address, display_name, description, config_schema, registered_at
		FROM grader_registrations
		WHERE type_id = ?`, typeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.GraderRegistration{}, fmt.Errorf("grader plugin %q: %w", typeID, store.ErrNotFound)
		}
		return store.GraderRegistration{}, fmt.Errorf("grader plugin store: get %q: %w", typeID, err)
	}
	return rowToGraderReg(row), nil
}

// ListGraderRegistrations returns all stored grader plugin registrations ordered by registration time.
func (s *WorkflowStore) ListGraderRegistrations(ctx context.Context) ([]store.GraderRegistration, error) {
	var rows []graderRegistrationRow
	err := s.db.SelectContext(ctx, &rows, `
		SELECT type_id, address, display_name, description, config_schema, registered_at
		FROM grader_registrations
		ORDER BY registered_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("grader plugin store: list: %w", err)
	}
	regs := make([]store.GraderRegistration, len(rows))
	for i, r := range rows {
		regs[i] = rowToGraderReg(r)
	}
	return regs, nil
}

// DeleteGraderRegistration removes a stored grader plugin registration.
func (s *WorkflowStore) DeleteGraderRegistration(ctx context.Context, typeID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM grader_registrations WHERE type_id = ?`, typeID)
	if err != nil {
		return fmt.Errorf("grader plugin store: delete %q: %w", typeID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("grader plugin %q: %w", typeID, store.ErrNotFound)
	}
	return nil
}

func rowToGraderReg(r graderRegistrationRow) store.GraderRegistration {
	return store.GraderRegistration{
		TypeID:       r.TypeID,
		Address:      r.Address,
		DisplayName:  r.DisplayName,
		Description:  r.Description,
		ConfigSchema: json.RawMessage(r.ConfigSchema),
		RegisteredAt: r.RegisteredAt,
	}
}
