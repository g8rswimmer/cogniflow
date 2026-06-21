package mysql

import (
	"context"
	"errors"
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// insertTestGraderReg inserts a grader registration directly via SQL,
// bypassing SaveGraderRegistration (which uses MySQL-specific ON DUPLICATE KEY syntax).
func insertTestGraderReg(t *testing.T, s *WorkflowStore, reg store.GraderRegistration) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO grader_registrations (type_id, address, display_name, description, config_schema)
		 VALUES (?, ?, ?, ?, ?)`,
		reg.TypeID, reg.Address, reg.DisplayName, reg.Description, string(reg.ConfigSchema),
	)
	if err != nil {
		t.Fatalf("insertTestGraderReg %q: %v", reg.TypeID, err)
	}
}

func TestGraderRegistration_GetAfterInsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestGraderReg(t, s, store.GraderRegistration{
		TypeID:       "my.grader",
		Address:      "localhost:9001",
		DisplayName:  "My Grader",
		Description:  "a test grader",
		ConfigSchema: []byte(`{"type":"object"}`),
	})

	got, err := s.GetGraderRegistration(ctx, "my.grader")
	if err != nil {
		t.Fatalf("GetGraderRegistration: %v", err)
	}
	if got.TypeID != "my.grader" {
		t.Errorf("want TypeID my.grader, got %s", got.TypeID)
	}
	if got.Address != "localhost:9001" {
		t.Errorf("want Address localhost:9001, got %s", got.Address)
	}
	if got.DisplayName != "My Grader" {
		t.Errorf("want DisplayName 'My Grader', got %s", got.DisplayName)
	}
	if got.Description != "a test grader" {
		t.Errorf("want Description 'a test grader', got %s", got.Description)
	}
}

func TestGraderRegistration_Get_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetGraderRegistration(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent type_id")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGraderRegistration_List(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"a.grader", "b.grader", "c.grader"} {
		insertTestGraderReg(t, s, store.GraderRegistration{
			TypeID:       id,
			Address:      "localhost:9001",
			DisplayName:  id,
			ConfigSchema: []byte(`{}`),
		})
	}

	regs, err := s.ListGraderRegistrations(ctx)
	if err != nil {
		t.Fatalf("ListGraderRegistrations: %v", err)
	}
	if len(regs) != 3 {
		t.Errorf("want 3 registrations, got %d", len(regs))
	}
}

func TestGraderRegistration_List_Empty(t *testing.T) {
	s := newTestStore(t)
	regs, err := s.ListGraderRegistrations(context.Background())
	if err != nil {
		t.Fatalf("ListGraderRegistrations: %v", err)
	}
	if len(regs) != 0 {
		t.Errorf("want empty list, got %d", len(regs))
	}
}

func TestGraderRegistration_Delete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	insertTestGraderReg(t, s, store.GraderRegistration{
		TypeID:       "del.grader",
		Address:      "localhost:9001",
		ConfigSchema: []byte(`{}`),
	})

	if err := s.DeleteGraderRegistration(ctx, "del.grader"); err != nil {
		t.Fatalf("DeleteGraderRegistration: %v", err)
	}

	_, err := s.GetGraderRegistration(ctx, "del.grader")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestGraderRegistration_Delete_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteGraderRegistration(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent grader registration")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
