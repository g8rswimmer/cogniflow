// Package nodeutil provides helpers shared across all built-in node packages.
package nodeutil

import (
	"bytes"
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
