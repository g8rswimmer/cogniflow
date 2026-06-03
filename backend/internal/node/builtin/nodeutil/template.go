// Package nodeutil provides helpers shared across all built-in node packages.
package nodeutil

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// RenderTemplate expands s as a Go text/template with data as the context.
// Missing keys produce an error so misconfigured upstream references fail fast
// rather than silently expanding to "<no value>".
// Strings without "{{" are returned as-is with no allocation.
func RenderTemplate(s string, data map[string]any) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	t, err := template.New("").Option("missingkey=error").Parse(s)
	if err != nil {
		return s, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return s, err
	}
	return buf.String(), nil
}

// ToInt converts a config value (which JSON unmarshals as float64) to int.
// Returns fallback when v is absent, nil, or a non-numeric type.
func ToInt(v any, fallback int) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return fallback
	}
}

// ResolveParams renders each element of a "params" config value ([]any of
// template strings) against upstream data and returns the rendered values as
// []any. Returns nil when params is absent. Returns an error if params is not
// a []any or if any element fails template rendering.
func ResolveParams(params any, upstream map[string]any) ([]any, error) {
	if params == nil {
		return nil, nil
	}
	slice, ok := params.([]any)
	if !ok {
		return nil, fmt.Errorf("params must be an array")
	}
	args := make([]any, len(slice))
	for i, v := range slice {
		s, ok := v.(string)
		if !ok {
			// Non-string values (e.g. float64 from JSON) are passed through unchanged
			// so the database driver receives the correct native type.
			args[i] = v
			continue
		}
		rendered, err := RenderTemplate(s, upstream)
		if err != nil {
			return nil, fmt.Errorf("render param[%d]: %w", i, err)
		}
		args[i] = rendered
	}
	return args, nil
}
