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

func TestResolveParams_Nil(t *testing.T) {
	args, err := ResolveParams(nil, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args != nil {
		t.Errorf("want nil, got %v", args)
	}
}

func TestResolveParams_EmptySlice(t *testing.T) {
	args, err := ResolveParams([]any{}, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 0 {
		t.Errorf("want 0 args, got %d", len(args))
	}
}

func TestResolveParams_LiteralStrings(t *testing.T) {
	args, err := ResolveParams([]any{"foo", "bar"}, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 2 || args[0] != "foo" || args[1] != "bar" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestResolveParams_TemplateStrings(t *testing.T) {
	args, err := ResolveParams([]any{"{{._initial.id}}"}, map[string]any{
		"_initial": map[string]any{"id": "42"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != "42" {
		t.Errorf("want 42, got %v", args[0])
	}
}

func TestResolveParams_NotArray_ReturnsError(t *testing.T) {
	_, err := ResolveParams("not-an-array", map[string]any{})
	if err == nil {
		t.Fatal("expected error for non-array params")
	}
}

func TestResolveParams_TemplateMissingKey_ReturnsError(t *testing.T) {
	_, err := ResolveParams([]any{"{{.missing}}"}, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing template key")
	}
}

func TestResolveParams_NumericPassThrough(t *testing.T) {
	// float64 is what JSON decoding produces for numeric literals.
	args, err := ResolveParams([]any{float64(42), true, "hello"}, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != float64(42) {
		t.Errorf("want float64(42), got %v (%T)", args[0], args[0])
	}
	if args[1] != true {
		t.Errorf("want true, got %v", args[1])
	}
	if args[2] != "hello" {
		t.Errorf("want hello, got %v", args[2])
	}
}
