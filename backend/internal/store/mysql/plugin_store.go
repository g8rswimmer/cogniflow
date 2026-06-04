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

type pluginRegistrationRow struct {
	TypeID       string    `db:"type_id"`
	Address      string    `db:"address"`
	DisplayName  string    `db:"display_name"`
	Category     string    `db:"category"`
	Description  string    `db:"description"`
	InputSchema  []byte    `db:"input_schema"`
	OutputSchema []byte    `db:"output_schema"`
	RegisteredAt time.Time `db:"registered_at"`
}

// SavePluginRegistration inserts or replaces a plugin registration row.
func (s *WorkflowStore) SavePluginRegistration(ctx context.Context, reg store.PluginRegistration) error {
	_, err := s.db.NamedExecContext(ctx, `
		INSERT INTO plugin_registrations
		    (type_id, address, display_name, category, description, input_schema, output_schema)
		VALUES
		    (:type_id, :address, :display_name, :category, :description, :input_schema, :output_schema)
		ON DUPLICATE KEY UPDATE
		    address       = VALUES(address),
		    display_name  = VALUES(display_name),
		    category      = VALUES(category),
		    description   = VALUES(description),
		    input_schema  = VALUES(input_schema),
		    output_schema = VALUES(output_schema)`,
		pluginRegistrationRow{
			TypeID:       reg.TypeID,
			Address:      reg.Address,
			DisplayName:  reg.DisplayName,
			Category:     reg.Category,
			Description:  reg.Description,
			InputSchema:  []byte(reg.InputSchema),
			OutputSchema: []byte(reg.OutputSchema),
		},
	)
	if err != nil {
		return fmt.Errorf("plugin store: save %q: %w", reg.TypeID, err)
	}
	return nil
}

// GetPluginRegistration returns the registration for the given TypeID.
func (s *WorkflowStore) GetPluginRegistration(ctx context.Context, typeID string) (store.PluginRegistration, error) {
	var row pluginRegistrationRow
	err := s.db.GetContext(ctx, &row, `
		SELECT type_id, address, display_name, category, description,
		       input_schema, output_schema, registered_at
		FROM plugin_registrations
		WHERE type_id = ?`, typeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.PluginRegistration{}, fmt.Errorf("plugin %q: %w", typeID, store.ErrNotFound)
		}
		return store.PluginRegistration{}, fmt.Errorf("plugin store: get %q: %w", typeID, err)
	}
	return rowToPluginReg(row), nil
}

// ListPluginRegistrations returns all stored plugin registrations ordered by registration time.
func (s *WorkflowStore) ListPluginRegistrations(ctx context.Context) ([]store.PluginRegistration, error) {
	var rows []pluginRegistrationRow
	err := s.db.SelectContext(ctx, &rows, `
		SELECT type_id, address, display_name, category, description,
		       input_schema, output_schema, registered_at
		FROM plugin_registrations
		ORDER BY registered_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("plugin store: list: %w", err)
	}
	regs := make([]store.PluginRegistration, len(rows))
	for i, r := range rows {
		regs[i] = rowToPluginReg(r)
	}
	return regs, nil
}

// DeletePluginRegistration removes a stored plugin registration.
func (s *WorkflowStore) DeletePluginRegistration(ctx context.Context, typeID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM plugin_registrations WHERE type_id = ?`, typeID)
	if err != nil {
		return fmt.Errorf("plugin store: delete %q: %w", typeID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("plugin %q: %w", typeID, store.ErrNotFound)
	}
	return nil
}

func rowToPluginReg(r pluginRegistrationRow) store.PluginRegistration {
	return store.PluginRegistration{
		TypeID:       r.TypeID,
		Address:      r.Address,
		DisplayName:  r.DisplayName,
		Category:     r.Category,
		Description:  r.Description,
		InputSchema:  json.RawMessage(r.InputSchema),
		OutputSchema: json.RawMessage(r.OutputSchema),
		RegisteredAt: r.RegisteredAt,
	}
}
