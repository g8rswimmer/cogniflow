package nodeutil

import "testing"

func TestRenderTemplate_Literal(t *testing.T) {
	out, err := RenderTemplate("hello world", map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello world" {
		t.Errorf("want 'hello world', got %q", out)
	}
}

func TestRenderTemplate_Substitution(t *testing.T) {
	out, err := RenderTemplate("Hello {{.n1.name}}", map[string]any{
		"n1": map[string]any{"name": "Alice"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "Hello Alice" {
		t.Errorf("want 'Hello Alice', got %q", out)
	}
}

func TestRenderTemplate_MissingKey_ReturnsError(t *testing.T) {
	_, err := RenderTemplate("{{.missing}}", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRenderTemplate_ParseError(t *testing.T) {
	_, err := RenderTemplate("{{.broken", map[string]any{})
	if err == nil {
		t.Fatal("expected error for malformed template")
	}
}

func TestToInt_Int(t *testing.T) {
	if got := ToInt(42, 0); got != 42 {
		t.Errorf("want 42, got %d", got)
	}
}

func TestToInt_Float64(t *testing.T) {
	if got := ToInt(float64(7), 0); got != 7 {
		t.Errorf("want 7, got %d", got)
	}
}

func TestToInt_Fallback(t *testing.T) {
	if got := ToInt(nil, 5); got != 5 {
		t.Errorf("want fallback 5, got %d", got)
	}
}

func TestToInt_StringFallback(t *testing.T) {
	if got := ToInt("not a number", 3); got != 3 {
		t.Errorf("want fallback 3, got %d", got)
	}
}
