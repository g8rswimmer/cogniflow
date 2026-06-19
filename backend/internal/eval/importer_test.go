package eval

import (
	"strings"
	"testing"
)

// ---- detectFormat -----------------------------------------------------------

func TestDetectFormat(t *testing.T) {
	cases := []struct {
		filename string
		want     string
		wantErr  bool
	}{
		{"data.csv", "csv", false},
		{"data.CSV", "csv", false},
		{"DATA.Csv", "csv", false},
		{"data.jsonl", "jsonl", false},
		{"data.JSONL", "jsonl", false},
		{"data.json", "", true},
		{"data.txt", "", true},
		{"noextension", "", true},
		{"archive.tar.gz", "", true},
	}
	for _, tc := range cases {
		got, err := detectFormat(tc.filename)
		if tc.wantErr {
			if err == nil {
				t.Errorf("detectFormat(%q): expected error, got nil", tc.filename)
			}
			continue
		}
		if err != nil {
			t.Errorf("detectFormat(%q): unexpected error: %v", tc.filename, err)
			continue
		}
		if got != tc.want {
			t.Errorf("detectFormat(%q): got %q, want %q", tc.filename, got, tc.want)
		}
	}
}

// ---- parseCSV ---------------------------------------------------------------

func TestParseCSV(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		_, _, err := parseCSV(strings.NewReader(""))
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})

	t.Run("header only", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name,description\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("missing name column", func(t *testing.T) {
		_, _, err := parseCSV(strings.NewReader("foo,bar\nval1,val2\n"))
		if err == nil {
			t.Fatal("expected error for missing name column")
		}
	})

	t.Run("single valid row no extra columns", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name,description\nAlice,A test\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].Name != "Alice" {
			t.Errorf("name: got %q, want %q", rows[0].Name, "Alice")
		}
		if rows[0].Description != "A test" {
			t.Errorf("description: got %q, want %q", rows[0].Description, "A test")
		}
		if len(rows[0].InitialData) != 0 {
			t.Errorf("expected empty initial_data, got %v", rows[0].InitialData)
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("row with extra columns becomes initial_data", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name,description,city,score\nBob,B test,NYC,42\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].InitialData["city"] != "NYC" {
			t.Errorf("city: got %v", rows[0].InitialData["city"])
		}
		if rows[0].InitialData["score"] != "42" {
			t.Errorf("score: got %v", rows[0].InitialData["score"])
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("row missing name value", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name,description\n,A test\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 1 {
			t.Fatalf("got %d errs, want 1", len(errs))
		}
		if errs[0].Row != 2 {
			t.Errorf("error row: got %d, want 2", errs[0].Row)
		}
	})

	t.Run("mixed valid and invalid rows", func(t *testing.T) {
		csv := "name,description\nAlice,first\n,missing name\nBob,third\n"
		rows, errs, err := parseCSV(strings.NewReader(csv))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("got %d rows, want 2", len(rows))
		}
		if len(errs) != 1 {
			t.Errorf("got %d errs, want 1", len(errs))
		}
		if errs[0].Row != 3 {
			t.Errorf("error row: got %d, want 3", errs[0].Row)
		}
	})

	t.Run("no description column", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name,city\nAlice,NYC\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].Description != "" {
			t.Errorf("description: got %q, want empty", rows[0].Description)
		}
		if rows[0].InitialData["city"] != "NYC" {
			t.Errorf("city: got %v", rows[0].InitialData["city"])
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("name only column", func(t *testing.T) {
		rows, errs, err := parseCSV(strings.NewReader("name\nAlice\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if len(rows[0].InitialData) != 0 {
			t.Errorf("expected empty initial_data, got %v", rows[0].InitialData)
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})
}

// ---- parseJSONL -------------------------------------------------------------

func TestParseJSONL(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		rows, errs, err := parseJSONL(strings.NewReader(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("blank lines only", func(t *testing.T) {
		rows, errs, err := parseJSONL(strings.NewReader("\n\n  \n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("valid single line with typed initial_data", func(t *testing.T) {
		line := `{"name":"Case A","description":"desc","initial_data":{"x":1,"flag":true}}`
		rows, errs, err := parseJSONL(strings.NewReader(line))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].Name != "Case A" {
			t.Errorf("name: got %q", rows[0].Name)
		}
		if rows[0].Description != "desc" {
			t.Errorf("description: got %q", rows[0].Description)
		}
		// JSON numbers decode as float64
		if rows[0].InitialData["x"] != float64(1) {
			t.Errorf("x: got %v (%T)", rows[0].InitialData["x"], rows[0].InitialData["x"])
		}
		if rows[0].InitialData["flag"] != true {
			t.Errorf("flag: got %v", rows[0].InitialData["flag"])
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})

	t.Run("missing name field", func(t *testing.T) {
		rows, errs, err := parseJSONL(strings.NewReader(`{"description":"no name"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 1 {
			t.Fatalf("got %d errs, want 1", len(errs))
		}
		if errs[0].Row != 1 {
			t.Errorf("error row: got %d, want 1", errs[0].Row)
		}
	})

	t.Run("invalid JSON line", func(t *testing.T) {
		rows, errs, err := parseJSONL(strings.NewReader("{not json}"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows, want 0", len(rows))
		}
		if len(errs) != 1 {
			t.Fatalf("got %d errs, want 1", len(errs))
		}
	})

	t.Run("mixed valid and invalid", func(t *testing.T) {
		input := `{"name":"Good","initial_data":{"k":1}}` + "\n" +
			`{bad json}` + "\n" +
			`{"name":"Also good"}` + "\n"
		rows, errs, err := parseJSONL(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 2 {
			t.Errorf("got %d rows, want 2", len(rows))
		}
		if len(errs) != 1 {
			t.Errorf("got %d errs, want 1", len(errs))
		}
		if errs[0].Row != 2 {
			t.Errorf("error row: got %d, want 2", errs[0].Row)
		}
	})

	t.Run("null initial_data coerced to empty map", func(t *testing.T) {
		rows, errs, err := parseJSONL(strings.NewReader(`{"name":"X"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].InitialData == nil {
			t.Error("expected non-nil initial_data, got nil")
		}
		if len(rows[0].InitialData) != 0 {
			t.Errorf("expected empty initial_data, got %v", rows[0].InitialData)
		}
		if len(errs) != 0 {
			t.Errorf("got %d errs, want 0", len(errs))
		}
	})
}
