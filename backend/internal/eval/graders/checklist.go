package graders

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Checklist implements GR-05: LLM-evaluated multi-criterion checklist grader.
type Checklist struct {
	provider      string
	model         string
	apiKey        string
	criteria      []string
	passThreshold float64
	fieldPath     string
	client        aiprovider.LLMClient
}

// NewChecklist constructs a Checklist grader from a GraderDef and a pre-created LLMClient.
func NewChecklist(def store.GraderDef, client aiprovider.LLMClient) (*Checklist, error) {
	provider, _ := def.Config["provider"].(string)
	model, _ := def.Config["model"].(string)
	apiKey, _ := def.Config["api_key"].(string)
	fieldPath, _ := def.Config["field_path"].(string)

	rawCriteria, ok := def.Config["criteria"]
	if !ok {
		return nil, fmt.Errorf("checklist: criteria is required")
	}
	criteria, err := toStringSlice(rawCriteria)
	if err != nil || len(criteria) == 0 {
		return nil, fmt.Errorf("checklist: criteria must be a non-empty array of strings")
	}

	// Treat 0.0 as "not configured" (same convention as the suite-level pass_threshold)
	// so that the JSON zero-value cannot silently make a checklist always pass.
	passThreshold := 1.0
	if v, ok := def.Config["pass_threshold"]; ok {
		switch n := v.(type) {
		case float64:
			if n > 0 {
				passThreshold = n
			}
		case int:
			if n > 0 {
				passThreshold = float64(n)
			}
		}
	}

	return &Checklist{
		provider:      provider,
		model:         model,
		apiKey:        apiKey,
		criteria:      criteria,
		passThreshold: passThreshold,
		fieldPath:     fieldPath,
		client:        client,
	}, nil
}

// Grade evaluates each criterion via a single LLM call and returns a scored result.
func (g *Checklist) Grade(ctx context.Context, data map[string]any) store.GraderResult {
	base := store.GraderResult{GraderType: "checklist"}

	var target any
	if g.fieldPath != "" {
		v, ok := resolveField(data, g.fieldPath)
		if !ok {
			base.Verdict = store.VerdictError
			base.Explanation = "field not found"
			return base
		}
		target = v
	} else {
		target = data
	}
	base.ActualValue = target

	targetJSON, err := json.Marshal(target)
	if err != nil {
		base.Verdict = store.VerdictError
		base.Explanation = fmt.Sprintf("could not serialize target: %v", err)
		return base
	}

	criteriaJSON, _ := json.Marshal(g.criteria)
	systemMsg := `You are a checklist evaluator. For each criterion in the list, decide whether the content satisfies it.
Respond with ONLY a JSON array — one object per criterion, in the same order — no preamble, no markdown, no extra text:
[{"criterion":"...","met":true,"explanation":"..."}, ...]`
	prompt := fmt.Sprintf("Criteria:\n%s\n\nContent:\n%s", string(criteriaJSON), string(targetJSON))

	resp, err := g.client.Complete(ctx, aiprovider.LLMRequest{
		APIKey:    g.apiKey,
		Model:     g.model,
		SystemMsg: systemMsg,
		Prompt:    prompt,
	})
	if err != nil {
		base.Verdict = store.VerdictError
		base.Explanation = err.Error()
		return base
	}

	completion := extractJSONArray(strings.TrimSpace(resp.Completion))

	var items []struct {
		Criterion   string `json:"criterion"`
		Met         bool   `json:"met"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(completion), &items); err != nil {
		base.Verdict = store.VerdictError
		base.Explanation = "checklist response could not be parsed"
		return base
	}
	if len(items) == 0 {
		base.Verdict = store.VerdictError
		base.Explanation = "checklist: LLM returned no criteria results"
		return base
	}

	criteriaResults := make([]store.CriterionResult, len(items))
	met := 0
	for i, item := range items {
		criteriaResults[i] = store.CriterionResult{
			Criterion:   item.Criterion,
			Met:         item.Met,
			Explanation: item.Explanation,
		}
		if item.Met {
			met++
		}
	}

	score := 0.0
	if len(items) > 0 {
		score = float64(met) / float64(len(items))
	}
	base.Score = &score
	base.CriteriaResults = criteriaResults
	base.Explanation = fmt.Sprintf("%d of %d criteria met", met, len(items))

	if score >= g.passThreshold {
		base.Verdict = store.VerdictPass
	} else {
		base.Verdict = store.VerdictFail
	}
	return base
}

// extractJSONArray finds the last complete JSON array in s by walking backward from the
// last ']' to its matching '['. This correctly handles preamble text that contains
// stray '[' characters before the real JSON array (e.g. bracketed notes or counts).
func extractJSONArray(s string) string {
	end := strings.LastIndex(s, "]")
	if end < 0 {
		return s
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case ']':
			depth++
		case '[':
			depth--
			if depth == 0 {
				return s[i : end+1]
			}
		}
	}
	return s
}

// toStringSlice converts []interface{} (from JSON decode) or []string to []string.
func toStringSlice(v any) ([]string, error) {
	switch typed := v.(type) {
	case []string:
		return typed, nil
	case []any:
		result := make([]string, len(typed))
		for i, item := range typed {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("criteria[%d] is not a string", i)
			}
			result[i] = s
		}
		return result, nil
	}
	return nil, fmt.Errorf("criteria must be an array")
}
