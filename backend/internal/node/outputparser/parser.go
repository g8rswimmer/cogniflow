// Package outputparser applies post-execution extraction rules to a node's raw output,
// producing additional named fields that downstream nodes can reference via templates.
//
// Two extraction strategies are supported:
//
//	"json_path" — uses gjson dot-path syntax to extract a field from a JSON string.
//	             Example: source="completion", pattern="user.id" extracts {"user":{"id":"42"}}
//	             → field value "42".
//
//	"regex"     — applies a regular expression to the source field value.
//	             CaptureGroup 0 returns the full match; 1+ returns a specific group.
//
// Extracted fields are merged into the node's output map. If a parser fails
// (no match, invalid pattern), its target field is silently omitted so that
// an optional extractor does not fail the whole node.
package outputparser

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/tidwall/gjson"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

// Apply extracts named values from raw output data using the given parsers and
// returns a new map containing both the original fields and the extracted ones.
// If parsers is empty, data is returned unchanged.
func Apply(data map[string]any, parsers map[string]store.OutputParser) map[string]any {
	if len(parsers) == 0 {
		return data
	}
	result := make(map[string]any, len(data)+len(parsers))
	for k, v := range data {
		result[k] = v
	}
	for name, p := range parsers {
		src := sourceString(data, p.Source)
		val, err := extract(src, p)
		if err == nil {
			result[name] = val
		}
		// Extraction failures are silently skipped — a missing field is preferable
		// to failing the whole node for an optional extractor.
	}
	return result
}

// Validate checks that a parser's pattern is syntactically valid.
// Returns a descriptive error suitable for surfacing at workflow-save time.
func Validate(name string, p store.OutputParser) error {
	switch p.Kind {
	case "json_path":
		if p.Pattern == "" {
			return fmt.Errorf("output_parser %q: json_path pattern must not be empty", name)
		}
		return nil
	case "regex":
		if _, err := regexp.Compile(p.Pattern); err != nil {
			return fmt.Errorf("output_parser %q: invalid regex pattern: %w", name, err)
		}
		if p.CaptureGroup < 0 {
			return fmt.Errorf("output_parser %q: capture_group must be >= 0, got %d", name, p.CaptureGroup)
		}
		return nil
	default:
		return fmt.Errorf("output_parser %q: unknown kind %q (want json_path or regex)", name, p.Kind)
	}
}

// ValidateAll calls Validate for every parser in the map.
func ValidateAll(parsers map[string]store.OutputParser) error {
	for name, p := range parsers {
		if err := Validate(name, p); err != nil {
			return err
		}
	}
	return nil
}

// sourceString returns the source field as a string suitable for extraction.
// If the field is already a string it is returned as-is. Non-string values
// (e.g. int status_code, map headers) are JSON-encoded so that gjson and
// regexp can still operate on them. A missing or nil source yields "".
func sourceString(data map[string]any, key string) string {
	raw, ok := data[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	return string(b)
}

// extract returns the extracted value. For json_path, scalar JSON types are
// preserved (bool → bool, number → float64, string → string) so downstream
// CEL expressions can use typed comparisons like `ctx["n1"]["compromised"] == true`.
// JSON arrays and objects are returned as their raw JSON text so that
// text/template renders valid JSON rather than Go's [a b] slice syntax.
// JSON null is treated as no-match and the field is omitted.
// Regex extractions always return strings.
//
// Note: JSON numbers are returned as float64; large integers (> 2^53) will
// lose precision. Use json_path only for integers within float64's exact range,
// or extract them as strings using a regex parser instead.
func extract(src string, p store.OutputParser) (any, error) {
	switch p.Kind {
	case "json_path":
		r := gjson.Get(src, p.Pattern)
		if !r.Exists() {
			return nil, fmt.Errorf("json_path %q: no match in source", p.Pattern)
		}
		v := r.Value()
		if v == nil {
			// JSON null — omit the field rather than storing nil.
			return nil, fmt.Errorf("json_path %q: value is null", p.Pattern)
		}
		// Arrays and objects: return the raw JSON text so text/template renders
		// valid JSON (["a","b"]) rather than Go's slice format ([a b]).
		switch v.(type) {
		case []interface{}, map[string]interface{}:
			return r.Raw, nil
		}
		return v, nil
	case "regex":
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regex %q: compile error: %w", p.Pattern, err)
		}
		matches := re.FindStringSubmatch(src)
		if matches == nil {
			return nil, fmt.Errorf("regex %q: no match in source", p.Pattern)
		}
		idx := p.CaptureGroup
		if idx < 0 || idx >= len(matches) {
			return nil, fmt.Errorf("regex: capture group %d out of range (have %d groups)", idx, len(matches)-1)
		}
		return matches[idx], nil
	default:
		return nil, fmt.Errorf("unknown parser kind %q", p.Kind)
	}
}
