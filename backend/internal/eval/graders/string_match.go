package graders

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/tidwall/gjson"
)

// StringMatch implements GR-01: exact, contains, and regex matching on a field.
type StringMatch struct {
	id        string
	name      string
	fieldPath string
	matchType string // "exact" | "contains" | "regex"
	expected  string
	compiled  *regexp.Regexp // non-nil when matchType == "regex"
}

// NewStringMatch constructs a StringMatch grader from a GraderDef.
// The regex pattern (if any) is compiled at construction time.
func NewStringMatch(def store.GraderDef) (*StringMatch, error) {
	fp, _ := def.Config["field_path"].(string)
	mt, _ := def.Config["match_type"].(string)
	ev, _ := def.Config["expected_value"].(string)

	g := &StringMatch{id: def.ID, name: def.Name, fieldPath: fp, matchType: mt, expected: ev}
	if mt == "regex" {
		re, err := regexp.Compile(ev)
		if err != nil {
			return nil, fmt.Errorf("string_match: invalid regex %q: %w", ev, err)
		}
		g.compiled = re
	}
	return g, nil
}

// Grade evaluates the grader against data.
func (g *StringMatch) Grade(data map[string]any) store.GraderResult {
	base := store.GraderResult{GraderType: "string_match"}

	actual, ok := resolveField(data, g.fieldPath)
	if !ok {
		base.Verdict = store.VerdictError
		base.Explanation = "field not found"
		return base
	}
	base.ActualValue = actual

	str := coerceString(actual)

	switch g.matchType {
	case "exact":
		if str == g.expected {
			base.Verdict = store.VerdictPass
		} else {
			base.Verdict = store.VerdictFail
			base.Explanation = fmt.Sprintf("expected %q, got %q", g.expected, str)
		}
	case "contains":
		if strings.Contains(str, g.expected) {
			base.Verdict = store.VerdictPass
		} else {
			base.Verdict = store.VerdictFail
			base.Explanation = fmt.Sprintf("%q not found in %q", g.expected, str)
		}
	case "regex":
		if g.compiled.MatchString(str) {
			base.Verdict = store.VerdictPass
		} else {
			base.Verdict = store.VerdictFail
			base.Explanation = fmt.Sprintf("%q did not match pattern %q", str, g.expected)
		}
	default:
		base.Verdict = store.VerdictError
		base.Explanation = fmt.Sprintf("unknown match_type %q", g.matchType)
	}
	return base
}

// resolveField looks up fieldPath in data using gjson. Returns (value, true) if found.
func resolveField(data map[string]any, fieldPath string) (any, bool) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	r := gjson.GetBytes(b, fieldPath)
	if !r.Exists() {
		return nil, false
	}
	return r.Value(), true
}

// coerceString converts any value to a string for comparison.
func coerceString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
