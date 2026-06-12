package graders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/store"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

// JSONSchemaGrader implements GR-04: JSON Schema draft-07 validation.
type JSONSchemaGrader struct {
	id        string
	name      string
	fieldPath string // optional; if empty, the full data map is validated
	schema    *jsonschema.Schema
}

// NewJSONSchema constructs a JSONSchemaGrader from a GraderDef.
// The JSON Schema is compiled at construction time.
func NewJSONSchema(def store.GraderDef) (*JSONSchemaGrader, error) {
	fp, _ := def.Config["field_path"].(string)

	schemaVal, ok := def.Config["schema"]
	if !ok {
		return nil, fmt.Errorf("json_schema: schema is required")
	}

	b, err := json.Marshal(schemaVal)
	if err != nil {
		return nil, fmt.Errorf("json_schema: cannot marshal schema: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("json_schema: invalid schema: %w", err)
	}
	compiled, err := c.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("json_schema: compile schema: %w", err)
	}

	return &JSONSchemaGrader{id: def.ID, name: def.Name, fieldPath: fp, schema: compiled}, nil
}

// Grade validates either the full data map or a resolved field against the schema.
func (g *JSONSchemaGrader) Grade(_ context.Context, data map[string]any) store.GraderResult {
	base := store.GraderResult{GraderType: "json_schema"}

	var target any = data
	if g.fieldPath != "" {
		resolved, ok := resolveField(data, g.fieldPath)
		if !ok {
			base.Verdict = store.VerdictError
			base.Explanation = "field not found"
			return base
		}
		target = resolved
		base.ActualValue = resolved
	} else {
		base.ActualValue = data
	}

	// The jsonschema library validates JSON-compatible Go values directly.
	if err := g.schema.Validate(target); err != nil {
		base.Verdict = store.VerdictFail
		base.Explanation = formatSchemaError(err)
		return base
	}

	base.Verdict = store.VerdictPass
	return base
}

func formatSchemaError(err error) string {
	var ve *jsonschema.ValidationError
	if ok := isValidationError(err, &ve); ok && ve != nil {
		msgs := collectMessages(ve)
		return strings.Join(msgs, "; ")
	}
	return err.Error()
}

func isValidationError(err error, out **jsonschema.ValidationError) bool {
	if ve, ok := err.(*jsonschema.ValidationError); ok {
		*out = ve
		return true
	}
	return false
}

func collectMessages(ve *jsonschema.ValidationError) []string {
	var msgs []string
	if ve.Message != "" {
		loc := ve.InstanceLocation
		if loc == "" {
			loc = "#"
		}
		msgs = append(msgs, loc+": "+ve.Message)
	}
	for _, cause := range ve.Causes {
		msgs = append(msgs, collectMessages(cause)...)
	}
	return msgs
}
