package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogLevelHandler_Get(t *testing.T) {
	var lv slog.LevelVar
	lv.Set(slog.LevelInfo)
	h := &logLevelHandler{level: &lv}

	req := httptest.NewRequest(http.MethodGet, "/admin/log-level", nil)
	w := httptest.NewRecorder()
	h.get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["level"] != "INFO" {
		t.Errorf("want INFO, got %q", body["level"])
	}
}

func TestLogLevelHandler_Set_Valid(t *testing.T) {
	cases := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug}, // case-insensitive
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var lv slog.LevelVar
			h := &logLevelHandler{level: &lv}

			body, _ := json.Marshal(map[string]string{"level": tc.input})
			req := httptest.NewRequest(http.MethodPut, "/admin/log-level", bytes.NewReader(body))
			w := httptest.NewRecorder()
			h.set(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
			}
			if lv.Level() != tc.want {
				t.Errorf("want level %v, got %v", tc.want, lv.Level())
			}
		})
	}
}

func TestLogLevelHandler_Set_Invalid(t *testing.T) {
	var lv slog.LevelVar
	lv.Set(slog.LevelInfo)
	h := &logLevelHandler{level: &lv}

	body, _ := json.Marshal(map[string]string{"level": "verbose"})
	req := httptest.NewRequest(http.MethodPut, "/admin/log-level", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.set(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
	// Level must be unchanged.
	if lv.Level() != slog.LevelInfo {
		t.Errorf("level must not change on invalid input, got %v", lv.Level())
	}
}

func TestLogLevelHandler_Set_BadJSON(t *testing.T) {
	var lv slog.LevelVar
	h := &logLevelHandler{level: &lv}

	req := httptest.NewRequest(http.MethodPut, "/admin/log-level", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.set(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
