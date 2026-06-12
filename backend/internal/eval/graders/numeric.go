package graders

import (
	"encoding/json"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// NumericThreshold implements GR-02: numeric comparison on a field value.
type NumericThreshold struct {
	id        string
	name      string
	fieldPath string
	operator  string // "==" | "!=" | ">" | ">=" | "<" | "<="
	threshold float64
}

// NewNumericThreshold constructs a NumericThreshold grader from a GraderDef.
func NewNumericThreshold(def store.GraderDef) (*NumericThreshold, error) {
	fp, _ := def.Config["field_path"].(string)
	op, _ := def.Config["operator"].(string)

	var threshold float64
	switch v := def.Config["threshold"].(type) {
	case float64:
		threshold = v
	case int:
		threshold = float64(v)
	case int64:
		threshold = float64(v)
	default:
		return nil, fmt.Errorf("numeric_threshold: threshold must be a number")
	}

	switch op {
	case "==", "!=", ">", ">=", "<", "<=":
	default:
		return nil, fmt.Errorf("numeric_threshold: unknown operator %q", op)
	}

	return &NumericThreshold{id: def.ID, name: def.Name, fieldPath: fp, operator: op, threshold: threshold}, nil
}

// Grade evaluates the grader against data.
func (g *NumericThreshold) Grade(data map[string]any) store.GraderResult {
	base := store.GraderResult{GraderType: "numeric_threshold"}

	actual, ok := resolveField(data, g.fieldPath)
	if !ok {
		base.Verdict = store.VerdictError
		base.Explanation = "field not found"
		return base
	}
	base.ActualValue = actual

	num, ok := toFloat64(actual)
	if !ok {
		base.Verdict = store.VerdictError
		base.Explanation = fmt.Sprintf("field is not numeric (got %T)", actual)
		return base
	}

	pass := compare(num, g.operator, g.threshold)
	if pass {
		base.Verdict = store.VerdictPass
	} else {
		base.Verdict = store.VerdictFail
		base.Explanation = fmt.Sprintf("%v %s %v is false", num, g.operator, g.threshold)
	}
	return base
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

func compare(a float64, op string, b float64) bool {
	switch op {
	case "==":
		return a == b
	case "!=":
		return a != b
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	}
	return false
}
