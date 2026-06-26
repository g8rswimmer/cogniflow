package nodeutil

import "fmt"

// GetRequiredString returns config[key] as a string, returning a descriptive
// error if the key is absent, nil, or not a string. Use this instead of the
// silent two-value type assertion (v, _ := cfg[k].(string)) so that
// misconfigured nodes surface a clear error rather than silently using "".
func GetRequiredString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%q is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%q must be a string, got %T", key, v)
	}
	return s, nil
}

// GetOptionalString returns config[key] as a string, or "" if absent/nil.
// Returns an error only if the key is present but not a string type.
func GetOptionalString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%q must be a string, got %T", key, v)
	}
	return s, nil
}
