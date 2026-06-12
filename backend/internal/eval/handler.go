package eval

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
)

const maxBodyBytes = 1 << 20 // 1 MB

// Handler provides HTTP handlers for all eval endpoints.
// It follows the same struct+methods pattern as the other handlers in api/.
type Handler struct {
	store    store.Store
	vault    *GraderVault
	registry *node.NodeRegistry
}

// NewHandler creates a Handler.
func NewHandler(st store.Store, vault *GraderVault, registry *node.NodeRegistry) *Handler {
	return &Handler{store: st, vault: vault, registry: registry}
}

// ---- Suite endpoints -------------------------------------------------------

// ListByWorkflow handles GET /v1/workflows/{workflow_id}/eval-suites.
func (h *Handler) ListByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflow_id")
	suites, err := h.store.ListEvalSuites(r.Context(), workflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if suites == nil {
		suites = []store.EvalSuiteSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"eval_suites": suites})
}

// CreateSuite handles POST /v1/workflows/{workflow_id}/eval-suites.
func (h *Handler) CreateSuite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	workflowID := r.PathValue("workflow_id")

	var body store.EvalSuite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "name is required")
		return
	}
	body.WorkflowID = workflowID
	applyDefaults(&body)

	created, err := h.store.CreateEvalSuite(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// GetSuite handles GET /v1/eval-suites/{suite_id}.
// Returns the suite with its test cases (api_keys masked).
func (h *Handler) GetSuite(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")
	suite, err := h.store.GetEvalSuite(r.Context(), suiteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	testCases, err := h.store.ListTestCases(r.Context(), suiteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if testCases == nil {
		testCases = []store.TestCase{}
	}
	// Mask sensitive config values before returning.
	for i := range testCases {
		testCases[i].Graders = h.vault.MaskGraders(testCases[i].Graders)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":               suite.ID,
		"workflow_id":      suite.WorkflowID,
		"name":             suite.Name,
		"description":      suite.Description,
		"pass_threshold":   suite.PassThreshold,
		"max_concurrency":  suite.MaxConcurrency,
		"workflow_deleted": suite.WorkflowDeleted,
		"created_at":       suite.CreatedAt,
		"updated_at":       suite.UpdatedAt,
		"test_cases":       testCases,
	})
}

// UpdateSuite handles PUT /v1/eval-suites/{suite_id}.
func (h *Handler) UpdateSuite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	suiteID := r.PathValue("suite_id")

	existing, err := h.store.GetEvalSuite(r.Context(), suiteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var body struct {
		Name           string  `json:"name"`
		Description    string  `json:"description"`
		PassThreshold  float64 `json:"pass_threshold"`
		MaxConcurrency int     `json:"max_concurrency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if body.Name == "" {
		body.Name = existing.Name
	}
	if body.PassThreshold == 0 {
		body.PassThreshold = existing.PassThreshold
	}
	if body.MaxConcurrency == 0 {
		body.MaxConcurrency = existing.MaxConcurrency
	}

	existing.Name = body.Name
	existing.Description = body.Description
	existing.PassThreshold = body.PassThreshold
	existing.MaxConcurrency = body.MaxConcurrency

	updated, err := h.store.UpdateEvalSuite(r.Context(), existing)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteSuite handles DELETE /v1/eval-suites/{suite_id}.
func (h *Handler) DeleteSuite(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")
	err := h.store.DeleteEvalSuite(r.Context(), suiteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Test Case endpoints ---------------------------------------------------

// ListCases handles GET /v1/eval-suites/{suite_id}/test-cases.
func (h *Handler) ListCases(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")
	if _, err := h.store.GetEvalSuite(r.Context(), suiteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	cases, err := h.store.ListTestCases(r.Context(), suiteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if cases == nil {
		cases = []store.TestCase{}
	}
	for i := range cases {
		cases[i].Graders = h.vault.MaskGraders(cases[i].Graders)
	}
	writeJSON(w, http.StatusOK, map[string]any{"test_cases": cases})
}

// CreateCase handles POST /v1/eval-suites/{suite_id}/test-cases.
func (h *Handler) CreateCase(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	suiteID := r.PathValue("suite_id")

	suite, err := h.store.GetEvalSuite(r.Context(), suiteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var tc store.TestCase
	if err := json.NewDecoder(r.Body).Decode(&tc); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if tc.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "name is required")
		return
	}
	tc.SuiteID = suiteID

	wfNodes, err := h.workflowNodes(r, suite.WorkflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if errs := validateTestCase(tc, wfNodes); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	encrypted, err := h.vault.EncryptGraders(tc.Graders)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	tc.Graders = encrypted

	if tc.InitialData == nil {
		tc.InitialData = map[string]any{}
	}
	if tc.Mocks == nil {
		tc.Mocks = []store.NodeMock{}
	}

	created, err := h.store.CreateTestCase(r.Context(), tc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	created.Graders = h.vault.MaskGraders(created.Graders)
	writeJSON(w, http.StatusCreated, created)
}

// GetCase handles GET /v1/eval-suites/{suite_id}/test-cases/{case_id}.
func (h *Handler) GetCase(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	tc, err := h.store.GetTestCase(r.Context(), caseID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	tc.Graders = h.vault.MaskGraders(tc.Graders)
	writeJSON(w, http.StatusOK, tc)
}

// UpdateCase handles PUT /v1/eval-suites/{suite_id}/test-cases/{case_id}.
func (h *Handler) UpdateCase(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	suiteID := r.PathValue("suite_id")
	caseID := r.PathValue("case_id")

	suite, err := h.store.GetEvalSuite(r.Context(), suiteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	existing, err := h.store.GetTestCase(r.Context(), caseID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var incoming store.TestCase
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if incoming.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "name is required")
		return
	}

	// Preserve encrypted api_key for graders where the client sent "***".
	existingByID := make(map[string]store.GraderDef, len(existing.Graders))
	for _, g := range existing.Graders {
		existingByID[g.ID] = g
	}
	for i, g := range incoming.Graders {
		if !sensitiveGraderTypes[g.Type] {
			continue
		}
		rawKey, ok := g.Config["api_key"]
		if !ok {
			continue
		}
		if strKey, _ := rawKey.(string); strKey == "***" {
			if prev, found := existingByID[g.ID]; found {
				if prevKey, ok := prev.Config["api_key"]; ok {
					cfg := cloneConfig(incoming.Graders[i].Config)
					cfg["api_key"] = prevKey
					incoming.Graders[i].Config = cfg
				}
			}
		}
	}

	wfNodes, err := h.workflowNodes(r, suite.WorkflowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if errs := validateTestCase(incoming, wfNodes); len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	encrypted, err := h.vault.EncryptGraders(incoming.Graders)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	existing.Name = incoming.Name
	existing.Description = incoming.Description
	existing.InitialData = incoming.InitialData
	existing.Mocks = incoming.Mocks
	existing.Graders = encrypted

	if existing.InitialData == nil {
		existing.InitialData = map[string]any{}
	}
	if existing.Mocks == nil {
		existing.Mocks = []store.NodeMock{}
	}

	updated, err := h.store.UpdateTestCase(r.Context(), existing)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	updated.Graders = h.vault.MaskGraders(updated.Graders)
	writeJSON(w, http.StatusOK, updated)
}

// DeleteCase handles DELETE /v1/eval-suites/{suite_id}/test-cases/{case_id}.
func (h *Handler) DeleteCase(w http.ResponseWriter, r *http.Request) {
	caseID := r.PathValue("case_id")
	err := h.store.DeleteTestCase(r.Context(), caseID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ReorderCases handles PUT /v1/eval-suites/{suite_id}/test-cases/order.
func (h *Handler) ReorderCases(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	suiteID := r.PathValue("suite_id")

	var body struct {
		CaseIDs []string `json:"case_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if len(body.CaseIDs) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "case_ids is required")
		return
	}

	if err := h.store.ReorderTestCases(r.Context(), suiteID, body.CaseIDs); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "one or more test case IDs not found in suite")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Run endpoints (stubs — implemented in ME2) ----------------------------

// TriggerRun handles POST /v1/eval-suites/{suite_id}/runs.
func (h *Handler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "eval run execution is not yet available")
}

// ListRuns handles GET /v1/eval-suites/{suite_id}/runs.
func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")
	if _, err := h.store.GetEvalSuite(r.Context(), suiteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	filter := store.EvalRunFilter{
		SuiteID: suiteID,
		Status:  store.EvalRunStatus(r.URL.Query().Get("status")),
		Limit:   50,
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	runs, err := h.store.ListEvalRuns(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if runs == nil {
		runs = []store.EvalRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"eval_runs": runs})
}

// GetRun handles GET /v1/eval-runs/{eval_run_id}.
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("eval_run_id")
	run, err := h.store.GetEvalRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	results, err := h.store.ListTestCaseResults(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if results == nil {
		results = []store.TestCaseResult{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 run.ID,
		"suite_id":           run.SuiteID,
		"status":             run.Status,
		"total_cases":        run.TotalCases,
		"passed_count":       run.PassedCount,
		"failed_count":       run.FailedCount,
		"error_count":        run.ErrorCount,
		"started_at":         run.StartedAt,
		"finished_at":        run.FinishedAt,
		"created_at":         run.CreatedAt,
		"test_case_results":  results,
	})
}

// GetTestCaseResult handles GET /v1/eval-runs/{eval_run_id}/test-case-results/{result_id}.
func (h *Handler) GetTestCaseResult(w http.ResponseWriter, r *http.Request) {
	resultID := r.PathValue("result_id")
	result, err := h.store.GetTestCaseResult(r.Context(), resultID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case result not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---- Validation ------------------------------------------------------------

type fieldValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// validateTestCase validates mock node IDs and grader configs at save time.
func validateTestCase(tc store.TestCase, wfNodes []store.WorkflowNode) []fieldValidationError {
	var errs []fieldValidationError
	errs = append(errs, validateMocks(tc.Mocks, wfNodes)...)
	errs = append(errs, validateGraderConfigs(tc.Graders)...)
	return errs
}

func validateMocks(mocks []store.NodeMock, wfNodes []store.WorkflowNode) []fieldValidationError {
	nodeIDs := make(map[string]bool, len(wfNodes))
	for _, n := range wfNodes {
		nodeIDs[n.ID] = true
	}
	var errs []fieldValidationError
	for i, m := range mocks {
		if !nodeIDs[m.NodeID] {
			errs = append(errs, fieldValidationError{
				Field:   fmt.Sprintf("mocks[%d].node_id", i),
				Message: fmt.Sprintf("node ID %q not found in workflow", m.NodeID),
			})
		}
	}
	return errs
}

func validateGraderConfigs(graders []store.GraderDef) []fieldValidationError {
	var errs []fieldValidationError
	for i, g := range graders {
		prefix := fmt.Sprintf("graders[%d]", i)
		switch g.Type {
		case "string_match":
			mt, _ := g.Config["match_type"].(string)
			if mt == "regex" {
				pat, _ := g.Config["expected_value"].(string)
				if _, err := regexp.Compile(pat); err != nil {
					errs = append(errs, fieldValidationError{
						Field:   prefix + ".config.expected_value",
						Message: "invalid regex: " + err.Error(),
					})
				}
			}
		case "json_schema":
			schemaVal, ok := g.Config["schema"]
			if !ok {
				errs = append(errs, fieldValidationError{
					Field:   prefix + ".config.schema",
					Message: "schema is required for json_schema grader",
				})
				continue
			}
			b, err := json.Marshal(schemaVal)
			if err != nil {
				errs = append(errs, fieldValidationError{
					Field:   prefix + ".config.schema",
					Message: "schema is not valid JSON",
				})
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal(b, &obj); err != nil {
				errs = append(errs, fieldValidationError{
					Field:   prefix + ".config.schema",
					Message: "schema must be a JSON object",
				})
			}
		}
	}
	return errs
}

// ---- Helpers ---------------------------------------------------------------

func applyDefaults(s *store.EvalSuite) {
	if s.PassThreshold == 0 {
		s.PassThreshold = 1.0
	}
	if s.MaxConcurrency == 0 {
		s.MaxConcurrency = 1
	}
}

// workflowNodes loads the nodes for a workflow so mock node IDs can be validated.
// Returns an empty slice (not an error) if the workflow does not exist — the
// validation will then report all mock node IDs as invalid.
func (h *Handler) workflowNodes(r *http.Request, workflowID string) ([]store.WorkflowNode, error) {
	wf, err := h.store.GetWorkflow(r.Context(), workflowID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return []store.WorkflowNode{}, nil
		}
		return nil, err
	}
	return wf.Nodes, nil
}

// ---- Response helpers -------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": map[string]any{},
		},
	})
}

func writeValidationErrors(w http.ResponseWriter, errs []fieldValidationError) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"code":    "VALIDATION_FAILED",
			"message": fmt.Sprintf("Test case validation failed: %d error(s)", len(errs)),
			"details": map[string]any{
				"validation_errors": errs,
			},
		},
	})
}

