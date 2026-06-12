package graders

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const judgeSystemPrompt = `You are a pass/fail evaluator. Given content and a rubric, decide whether the content satisfies the rubric.
Respond with ONLY a JSON object on a single line — no preamble, no markdown, no extra text:
{"verdict":"pass","explanation":"..."} or {"verdict":"fail","explanation":"..."}`

// LLMJudge implements GR-03: LLM-as-judge binary pass/fail grader.
type LLMJudge struct {
	provider  string
	model     string
	apiKey    string
	rubric    string
	fieldPath string
	client    aiprovider.LLMClient
}

// NewLLMJudge constructs an LLMJudge grader from a GraderDef and a pre-created LLMClient.
func NewLLMJudge(def store.GraderDef, client aiprovider.LLMClient) (*LLMJudge, error) {
	provider, _ := def.Config["provider"].(string)
	model, _ := def.Config["model"].(string)
	apiKey, _ := def.Config["api_key"].(string)
	rubric, _ := def.Config["rubric"].(string)
	fieldPath, _ := def.Config["field_path"].(string)

	if rubric == "" {
		return nil, fmt.Errorf("llm_judge: rubric is required")
	}

	return &LLMJudge{
		provider:  provider,
		model:     model,
		apiKey:    apiKey,
		rubric:    rubric,
		fieldPath: fieldPath,
		client:    client,
	}, nil
}

// Grade calls the judge LLM and returns a pass/fail verdict with explanation.
func (g *LLMJudge) Grade(ctx context.Context, data map[string]any) store.GraderResult {
	base := store.GraderResult{GraderType: "llm_judge"}

	// Resolve field or use full data.
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

	prompt := fmt.Sprintf("Rubric: %s\n\nContent:\n%s", g.rubric, string(targetJSON))

	resp, err := g.client.Complete(ctx, aiprovider.LLMRequest{
		APIKey:    g.apiKey,
		Model:     g.model,
		SystemMsg: judgeSystemPrompt,
		Prompt:    prompt,
	})
	if err != nil {
		base.Verdict = store.VerdictError
		base.Explanation = err.Error()
		return base
	}

	completion := extractJSON(strings.TrimSpace(resp.Completion))

	var jr struct {
		Verdict     string `json:"verdict"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(completion), &jr); err != nil {
		base.Verdict = store.VerdictError
		base.Explanation = "judge response could not be parsed"
		return base
	}

	switch jr.Verdict {
	case "pass":
		base.Verdict = store.VerdictPass
	case "fail":
		base.Verdict = store.VerdictFail
	default:
		base.Verdict = store.VerdictError
		base.Explanation = fmt.Sprintf("judge returned unknown verdict %q", jr.Verdict)
		return base
	}
	base.Explanation = jr.Explanation
	return base
}

// extractJSON finds the last complete JSON object in s by walking backward from the
// last '}' to its matching '{'. This correctly handles preamble text that contains
// stray '{' characters before the real JSON answer (e.g. thinking blocks or notes).
func extractJSON(s string) string {
	end := strings.LastIndex(s, "}")
	if end < 0 {
		return s
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return s[i : end+1]
			}
		}
	}
	return s
}
