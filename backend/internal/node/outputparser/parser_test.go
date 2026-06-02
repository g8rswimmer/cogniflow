package outputparser

import (
	"testing"

	"github.com/g8rswimmer/cogniflow/internal/store"
)

func TestApply_Empty(t *testing.T) {
	data := map[string]any{"completion": "hello"}
	out := Apply(data, nil)
	if out["completion"] != "hello" {
		t.Errorf("original field lost")
	}
}

func TestApply_JSONPath_TopLevel(t *testing.T) {
	data := map[string]any{
		"completion": `{"user_id":"42","status":"active"}`,
	}
	parsers := map[string]store.OutputParser{
		"user_id": {Kind: "json_path", Source: "completion", Pattern: "user_id"},
		"status":  {Kind: "json_path", Source: "completion", Pattern: "status"},
	}
	out := Apply(data, parsers)
	if out["user_id"] != "42" {
		t.Errorf("want user_id=42, got %v", out["user_id"])
	}
	if out["status"] != "active" {
		t.Errorf("want status=active, got %v", out["status"])
	}
	if out["completion"] != data["completion"] {
		t.Error("original completion field lost")
	}
}

func TestApply_JSONPath_Nested(t *testing.T) {
	data := map[string]any{
		"completion": `{"result":{"score":0.98}}`,
	}
	out := Apply(data, map[string]store.OutputParser{
		"score": {Kind: "json_path", Source: "completion", Pattern: "result.score"},
	})
	// json_path preserves the native JSON number type.
	if out["score"] != float64(0.98) {
		t.Errorf("want score=float64(0.98), got %v (%T)", out["score"], out["score"])
	}
}

func TestApply_JSONPath_BoolPreserved(t *testing.T) {
	data := map[string]any{
		"completion": `{"compromised":true,"risk":0.9}`,
	}
	out := Apply(data, map[string]store.OutputParser{
		"user_acc_compromise": {Kind: "json_path", Source: "completion", Pattern: "compromised"},
		"risk_score":          {Kind: "json_path", Source: "completion", Pattern: "risk"},
	})
	// Boolean true must be a bool, not the string "true", so downstream CEL can use == true.
	if out["user_acc_compromise"] != true {
		t.Errorf("want bool true, got %v (%T)", out["user_acc_compromise"], out["user_acc_compromise"])
	}
	if out["risk_score"] != float64(0.9) {
		t.Errorf("want float64(0.9), got %v (%T)", out["risk_score"], out["risk_score"])
	}
}

func TestApply_JSONPath_ArrayReturnedAsJSONString(t *testing.T) {
	data := map[string]any{
		"completion": `{"tags":["a","b","c"]}`,
	}
	out := Apply(data, map[string]store.OutputParser{
		"tags": {Kind: "json_path", Source: "completion", Pattern: "tags"},
	})
	// Arrays must come back as JSON text, not a Go []interface{}, so that
	// downstream text/template renders ["a","b","c"] not [a b c].
	if out["tags"] != `["a","b","c"]` {
		t.Errorf("want JSON array string, got %v (%T)", out["tags"], out["tags"])
	}
}

func TestApply_JSONPath_ObjectReturnedAsJSONString(t *testing.T) {
	data := map[string]any{
		"completion": `{"meta":{"env":"prod"}}`,
	}
	out := Apply(data, map[string]store.OutputParser{
		"meta": {Kind: "json_path", Source: "completion", Pattern: "meta"},
	})
	// Objects must come back as JSON text, not a Go map, so templates stay valid.
	if out["meta"] != `{"env":"prod"}` {
		t.Errorf("want JSON object string, got %v (%T)", out["meta"], out["meta"])
	}
}

func TestApply_NonStringSource_IntEncoded(t *testing.T) {
	// Source field is an int (e.g. status_code from an HTTP Request node).
	// It must be JSON-encoded before extraction so the parser can operate on it.
	data := map[string]any{"status_code": 200}
	out := Apply(data, map[string]store.OutputParser{
		"code": {Kind: "regex", Source: "status_code", Pattern: `(\d+)`, CaptureGroup: 1},
	})
	if out["code"] != "200" {
		t.Errorf("want code=200, got %v", out["code"])
	}
}

func TestApply_NonStringSource_MapEncoded(t *testing.T) {
	// Source field is a map (e.g. headers from an HTTP Request node).
	// JSON-encoding it allows json_path to drill into the structure.
	data := map[string]any{
		"headers": map[string]any{"Content-Type": "application/json"},
	}
	out := Apply(data, map[string]store.OutputParser{
		"ct": {Kind: "json_path", Source: "headers", Pattern: "Content-Type"},
	})
	if out["ct"] != "application/json" {
		t.Errorf("want ct=application/json, got %v", out["ct"])
	}
}

func TestApply_JSONPath_NullSkipped(t *testing.T) {
	data := map[string]any{
		"completion": `{"status":null}`,
	}
	out := Apply(data, map[string]store.OutputParser{
		"status": {Kind: "json_path", Source: "completion", Pattern: "status"},
	})
	if _, ok := out["status"]; ok {
		t.Error("JSON null should be treated as no-match and the field omitted")
	}
}

func TestApply_JSONPath_NoMatch(t *testing.T) {
	data := map[string]any{"completion": `{"foo":"bar"}`}
	out := Apply(data, map[string]store.OutputParser{
		"missing": {Kind: "json_path", Source: "completion", Pattern: "nonexistent"},
	})
	if _, ok := out["missing"]; ok {
		t.Error("field should be absent on no-match, not empty string")
	}
}

func TestApply_Regex_FullMatch(t *testing.T) {
	data := map[string]any{"completion": "account status: COMPROMISED"}
	out := Apply(data, map[string]store.OutputParser{
		"status_word": {Kind: "regex", Source: "completion", Pattern: `status: (\w+)`, CaptureGroup: 1},
	})
	if out["status_word"] != "COMPROMISED" {
		t.Errorf("want COMPROMISED, got %v", out["status_word"])
	}
}

func TestApply_Regex_Group0(t *testing.T) {
	data := map[string]any{"completion": "user_id=99"}
	out := Apply(data, map[string]store.OutputParser{
		"raw": {Kind: "regex", Source: "completion", Pattern: `user_id=\d+`, CaptureGroup: 0},
	})
	if out["raw"] != "user_id=99" {
		t.Errorf("want user_id=99, got %v", out["raw"])
	}
}

func TestApply_Regex_NoMatch(t *testing.T) {
	data := map[string]any{"completion": "hello"}
	out := Apply(data, map[string]store.OutputParser{
		"num": {Kind: "regex", Source: "completion", Pattern: `\d+`, CaptureGroup: 0},
	})
	if _, ok := out["num"]; ok {
		t.Error("field should be absent on no regex match")
	}
}

func TestApply_UnknownKind_Skipped(t *testing.T) {
	data := map[string]any{"completion": "hi"}
	out := Apply(data, map[string]store.OutputParser{
		"x": {Kind: "invalid", Source: "completion", Pattern: "whatever"},
	})
	if _, ok := out["x"]; ok {
		t.Error("invalid kind should be skipped, not produce a field")
	}
}

func TestApply_PreservesOriginalFields(t *testing.T) {
	data := map[string]any{"completion": `{"id":"5"}`, "prompt_tokens": 10}
	out := Apply(data, map[string]store.OutputParser{
		"id": {Kind: "json_path", Source: "completion", Pattern: "id"},
	})
	if out["prompt_tokens"] != 10 {
		t.Error("original fields must be preserved")
	}
	if out["id"] != "5" {
		t.Errorf("extracted field missing, got %v", out["id"])
	}
}

func TestValidate_ValidJSONPath(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "json_path", Source: "completion", Pattern: "result.id"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_EmptyJSONPathPattern(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "json_path", Source: "completion", Pattern: ""})
	if err == nil {
		t.Error("expected error for empty json_path pattern")
	}
}

func TestValidate_ValidRegex(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "regex", Source: "completion", Pattern: `\d+`})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_InvalidRegex(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "regex", Source: "completion", Pattern: `[invalid`})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestValidate_NegativeCaptureGroup(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "regex", Source: "completion", Pattern: `\d+`, CaptureGroup: -1})
	if err == nil {
		t.Error("expected error for negative capture_group")
	}
}

func TestApply_NegativeCaptureGroup_Skipped(t *testing.T) {
	// Validate() catches this at save time; Apply() must not panic if it reaches
	// extract() with a negative group (e.g. loaded from DB before validation was added).
	data := map[string]any{"completion": "abc123"}
	out := Apply(data, map[string]store.OutputParser{
		"num": {Kind: "regex", Source: "completion", Pattern: `(\d+)`, CaptureGroup: -1},
	})
	if _, ok := out["num"]; ok {
		t.Error("negative capture group should be skipped, not produce a field")
	}
}

func TestValidate_UnknownKind(t *testing.T) {
	err := Validate("x", store.OutputParser{Kind: "xpath", Source: "completion", Pattern: "//foo"})
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

func TestValidateAll_Mixed(t *testing.T) {
	parsers := map[string]store.OutputParser{
		"good": {Kind: "json_path", Source: "completion", Pattern: "id"},
		"bad":  {Kind: "regex", Source: "completion", Pattern: `[broken`},
	}
	if err := ValidateAll(parsers); err == nil {
		t.Error("expected error from invalid parser")
	}
}
