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
		src, _ := data[p.Source].(string)
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

func extract(src string, p store.OutputParser) (string, error) {
	switch p.Kind {
	case "json_path":
		r := gjson.Get(src, p.Pattern)
		if !r.Exists() {
			return "", fmt.Errorf("json_path %q: no match in source", p.Pattern)
		}
		return r.String(), nil
	case "regex":
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return "", fmt.Errorf("regex %q: compile error: %w", p.Pattern, err)
		}
		matches := re.FindStringSubmatch(src)
		if matches == nil {
			return "", fmt.Errorf("regex %q: no match in source", p.Pattern)
		}
		idx := p.CaptureGroup
		if idx < 0 || idx >= len(matches) {
			return "", fmt.Errorf("regex: capture group %d out of range (have %d groups)", idx, len(matches)-1)
		}
		return matches[idx], nil
	default:
		return "", fmt.Errorf("unknown parser kind %q", p.Kind)
	}
}
