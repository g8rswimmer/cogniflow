package eval

import (
	"context"
	"fmt"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/eval/grader_plugin"
	"github.com/g8rswimmer/cogniflow/internal/eval/graders"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Grader evaluates one grader definition against a resolved data map and returns a result.
// ctx is threaded through so LLM-backed graders can respect cancellation.
type Grader interface {
	Grade(ctx context.Context, data map[string]any) store.GraderResult
}

// LLMFactory returns an LLMClient for the given provider name (e.g. "anthropic", "openai").
type LLMFactory func(provider string) (aiprovider.LLMClient, error)

// BuildGrader constructs the appropriate Grader for the given definition.
// factory is required for llm_judge and checklist; pass nil for deterministic graders.
// registry is required for plugin graders; pass nil to disable plugin grader support.
func BuildGrader(def store.GraderDef, factory LLMFactory, registry *grader_plugin.GraderRegistry) (Grader, error) {
	switch def.Type {
	case "string_match":
		return graders.NewStringMatch(def)
	case "numeric_threshold":
		return graders.NewNumericThreshold(def)
	case "json_schema":
		return graders.NewJSONSchema(def)
	case "llm_judge":
		if factory == nil {
			return nil, fmt.Errorf("llm_judge grader requires an LLM factory")
		}
		provider, _ := def.Config["provider"].(string)
		client, err := factory(provider)
		if err != nil {
			return nil, fmt.Errorf("llm_judge: %w", err)
		}
		return graders.NewLLMJudge(def, client)
	case "checklist":
		if factory == nil {
			return nil, fmt.Errorf("checklist grader requires an LLM factory")
		}
		provider, _ := def.Config["provider"].(string)
		client, err := factory(provider)
		if err != nil {
			return nil, fmt.Errorf("checklist: %w", err)
		}
		return graders.NewChecklist(def, client)
	default:
		if registry != nil {
			g, err := grader_plugin.NewPluginGrader(registry, def.Type, def.Config)
			if err == nil {
				return g, nil
			}
		}
		return nil, fmt.Errorf("unknown grader type %q", def.Type)
	}
}
