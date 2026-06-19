package eval

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	maxImportRows      = 500
	maxImportFileBytes = int64(5 * 1024 * 1024) // 5 MB
)

type importRow struct {
	Name        string
	Description string
	InitialData map[string]any
	RowNum      int // source row/line number for error reporting
}

type importRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// detectFormat returns "csv" or "jsonl" based on the file extension.
func detectFormat(filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv":
		return "csv", nil
	case ".jsonl":
		return "jsonl", nil
	default:
		return "", fmt.Errorf("unsupported file type %q; must be .csv or .jsonl", ext)
	}
}

// parseCSV reads a CSV file and returns parsed rows plus any row-level errors.
// The header row must contain a "name" column. A "description" column is optional.
// All other columns become fields in initial_data (values are strings).
// A top-level error is returned only for I/O or structural parse failures.
func parseCSV(r io.Reader) ([]importRow, []importRowError, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil, fmt.Errorf("file is empty")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading CSV header: %w", err)
	}

	nameIdx := -1
	descIdx := -1
	extraCols := map[int]string{} // index → column name for initial_data

	for i, col := range header {
		switch strings.TrimSpace(strings.ToLower(col)) {
		case "name":
			nameIdx = i
		case "description":
			descIdx = i
		default:
			extraCols[i] = strings.TrimSpace(col)
		}
	}
	if nameIdx == -1 {
		return nil, nil, fmt.Errorf("CSV must have a 'name' column")
	}

	var rows []importRow
	var rowErrs []importRowError
	rowNum := 1 // header is row 1; data starts at row 2
	for {
		rowNum++
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rowErrs = append(rowErrs, importRowError{Row: rowNum, Message: fmt.Sprintf("CSV parse error: %s", err)})
			continue
		}

		name := ""
		if nameIdx < len(record) {
			name = strings.TrimSpace(record[nameIdx])
		}
		if name == "" {
			rowErrs = append(rowErrs, importRowError{Row: rowNum, Message: "name is required"})
			continue
		}

		desc := ""
		if descIdx >= 0 && descIdx < len(record) {
			desc = strings.TrimSpace(record[descIdx])
		}

		initialData := map[string]any{}
		for idx, colName := range extraCols {
			if idx < len(record) {
				initialData[colName] = record[idx]
			}
		}

		rows = append(rows, importRow{
			Name:        name,
			Description: desc,
			InitialData: initialData,
			RowNum:      rowNum,
		})
	}

	return rows, rowErrs, nil
}

// parseJSONL reads a JSONL file (one JSON object per line) and returns parsed rows.
// Each line must be a JSON object with a "name" field (required) and optionally
// "description" and "initial_data" fields. Blank lines are skipped silently.
func parseJSONL(r io.Reader) ([]importRow, []importRowError, error) {
	scanner := bufio.NewScanner(r)
	// Initial buffer is 64 KB; max grows to match the file size limit so a
	// single large JSON line doesn't cause a fatal top-level error.
	scanner.Buffer(make([]byte, 64*1024), int(maxImportFileBytes))

	type jsonLine struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InitialData map[string]any `json:"initial_data"`
	}

	var rows []importRow
	var rowErrs []importRowError
	lineNum := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lineNum++

		var jl jsonLine
		if err := json.Unmarshal([]byte(line), &jl); err != nil {
			rowErrs = append(rowErrs, importRowError{Row: lineNum, Message: fmt.Sprintf("invalid JSON: %s", err)})
			continue
		}
		if strings.TrimSpace(jl.Name) == "" {
			rowErrs = append(rowErrs, importRowError{Row: lineNum, Message: "name is required"})
			continue
		}
		if jl.InitialData == nil {
			jl.InitialData = map[string]any{}
		}

		rows = append(rows, importRow{
			Name:        strings.TrimSpace(jl.Name),
			Description: jl.Description,
			InitialData: jl.InitialData,
			RowNum:      lineNum,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("reading JSONL: %w", err)
	}

	return rows, rowErrs, nil
}
