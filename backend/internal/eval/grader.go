package eval

import (
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/eval/graders"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Grader evaluates one grader definition against a resolved data map and returns a result.
type Grader interface {
	Grade(data map[string]any) store.GraderResult
}

// BuildGrader constructs the appropriate Grader for the given definition.
// Returns an error for unknown types or types not yet available (llm_judge/checklist require ME3).
func BuildGrader(def store.GraderDef) (Grader, error) {
	switch def.Type {
	case "string_match":
		return graders.NewStringMatch(def)
	case "numeric_threshold":
		return graders.NewNumericThreshold(def)
	case "json_schema":
		return graders.NewJSONSchema(def)
	case "llm_judge", "checklist":
		return nil, fmt.Errorf("grader type %q requires ME3 (LLM graders not yet available)", def.Type)
	default:
		return nil, fmt.Errorf("unknown grader type %q", def.Type)
	}
}
