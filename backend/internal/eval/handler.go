package eval

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

const maxBodyBytes = 1 << 20 // 1 MB

// evalRunnerI is the minimal interface the handler needs from EvalRunner.
// *EvalRunner satisfies it; a stub can be injected in tests.
type evalRunnerI interface {
	Execute(ctx context.Context, suiteID string, triggeredBy string) (string, error)
}

// Handler provides HTTP handlers for all eval endpoints.
// It follows the same struct+methods pattern as the other handlers in api/.
type Handler struct {
	store     store.Store
	vault     *GraderVault
	registry  *node.NodeRegistry
	runner    evalRunnerI    // nil when eval execution not yet wired (safe: TriggerRun guards nil)
	scheduler *EvalScheduler // nil-safe; arms/disarms cron jobs on suite CRUD
	bus       *EvalEventBus  // nil disables streaming (StreamEvalRunEvents returns 501)
}

// NewHandler creates a Handler.
func NewHandler(st store.Store, vault *GraderVault, registry *node.NodeRegistry, runner evalRunnerI, scheduler *EvalScheduler, bus *EvalEventBus) *Handler {
	return &Handler{store: st, vault: vault, registry: registry, runner: runner, scheduler: scheduler, bus: bus}
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
	// Never expose the encrypted webhook secret in list responses; callers receive
	// the plaintext only on create or explicit rotation via the detail endpoint.
	for i := range suites {
		suites[i].WebhookSecret = ""
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

	plainSecret, err := h.applyTrigger(&body, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	created, err := h.store.CreateEvalSuite(r.Context(), body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if created.TriggerKind == "cron" {
		if err := h.scheduler.Arm(created.ID, created.CronExpr); err != nil {
			slog.Warn("eval handler: failed to arm cron suite", "suite_id", created.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, suiteResponse(created, plainSecret))
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

	resp := suiteResponse(suite, "") // secret masked on GET
	resp["test_cases"] = testCases
	writeJSON(w, http.StatusOK, resp)
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
		Name                string   `json:"name"`
		Description         *string  `json:"description"` // pointer so omitted ≠ explicit empty string
		PassThreshold       *float64 `json:"pass_threshold"` // pointer: nil = omitted, 0.0 = explicit zero
		MaxConcurrency      int      `json:"max_concurrency"`
		TriggerKind         string   `json:"trigger_kind"`
		CronExpr            string   `json:"cron_expr"`
		RotateWebhookSecret bool     `json:"rotate_webhook_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid request body: "+err.Error())
		return
	}
	if body.RotateWebhookSecret && body.TriggerKind != "" && body.TriggerKind != "webhook" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "rotate_webhook_secret cannot be combined with a non-webhook trigger_kind")
		return
	}
	if body.Name == "" {
		body.Name = existing.Name
	}
	if body.PassThreshold != nil {
		existing.PassThreshold = *body.PassThreshold
	}
	if body.MaxConcurrency == 0 {
		body.MaxConcurrency = existing.MaxConcurrency
	}

	existing.Name = body.Name
	if body.Description != nil {
		existing.Description = *body.Description
	}
	existing.MaxConcurrency = body.MaxConcurrency

	// Apply trigger changes. If trigger_kind is omitted, keep the existing one.
	if body.TriggerKind != "" {
		existing.TriggerKind = body.TriggerKind
		existing.CronExpr = body.CronExpr
		if body.TriggerKind != "webhook" {
			existing.WebhookSecret = "" // clear secret when switching away from webhook
		}
	}
	// Rotate the webhook secret when explicitly requested.
	if body.RotateWebhookSecret {
		existing.TriggerKind = "webhook"
	}

	// Generate a new secret when switching to webhook (empty secret) or rotating.
	generateSecret := body.RotateWebhookSecret || (existing.TriggerKind == "webhook" && existing.WebhookSecret == "")
	plainSecret, err := h.applyTrigger(&existing, generateSecret)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	updated, err := h.store.UpdateEvalSuite(r.Context(), existing)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	switch updated.TriggerKind {
	case "cron":
		if err := h.scheduler.Arm(updated.ID, updated.CronExpr); err != nil {
			slog.Warn("eval handler: failed to arm cron suite", "suite_id", updated.ID, "error", err)
		}
	default:
		h.scheduler.Disarm(updated.ID)
	}
	writeJSON(w, http.StatusOK, suiteResponse(updated, plainSecret))
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
	h.scheduler.Disarm(suiteID)
	w.WriteHeader(http.StatusNoContent)
}

// ---- Test Case endpoints ---------------------------------------------------

// ImportTestCases handles POST /v1/eval-suites/{suite_id}/test-cases/import.
// Accepts a multipart/form-data upload with a single "file" field (.csv or .jsonl).
// Valid rows are created as TestCases; row-level errors are reported in the response
// without aborting the remaining rows (partial success is normal).
func (h *Handler) ImportTestCases(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")

	if _, err := h.store.GetEvalSuite(r.Context(), suiteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileBytes)
	if err := r.ParseMultipartForm(maxImportFileBytes); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "could not parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "file field is required")
		return
	}
	defer file.Close() //nolint:errcheck

	format, err := detectFormat(header.Filename)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}

	var (
		rows    []importRow
		rowErrs []importRowError
	)
	switch format {
	case "csv":
		rows, rowErrs, err = parseCSV(file)
	case "jsonl":
		rows, rowErrs, err = parseJSONL(file)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "could not parse file: "+err.Error())
		return
	}

	if len(rows)+len(rowErrs) > maxImportRows {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED",
			fmt.Sprintf("import exceeds maximum of %d rows", maxImportRows))
		return
	}

	if rowErrs == nil {
		rowErrs = []importRowError{}
	}

	created := 0
	for _, row := range rows {
		tc := store.TestCase{
			SuiteID:     suiteID,
			Name:        row.Name,
			Description: row.Description,
			InitialData: row.InitialData,
			Mocks:       []store.NodeMock{},
			Graders:     []store.GraderDef{},
		}
		if _, err := h.store.CreateTestCase(r.Context(), tc); err != nil {
			rowErrs = append(rowErrs, importRowError{Row: row.RowNum, Message: "store error: " + err.Error()})
			continue
		}
		created++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"created": created,
		"skipped": len(rowErrs),
		"errors":  rowErrs,
	})
}

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
	suiteID := r.PathValue("suite_id")
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
	if tc.SuiteID != suiteID {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
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
	// Ownership check — prevents cross-suite IDOR.
	if existing.SuiteID != suiteID {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
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
	// If "***" is sent for a grader whose ID does not match any stored grader,
	// the key cannot be recovered — reject rather than silently lose it.
	existingByID := make(map[string]store.GraderDef, len(existing.Graders))
	for _, g := range existing.Graders {
		existingByID[g.ID] = g
	}
	var sentinelErrs []fieldValidationError
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
			} else {
				sentinelErrs = append(sentinelErrs, fieldValidationError{
					Field:   fmt.Sprintf("graders[%d].config.api_key", i),
					Message: `api_key must be a real value; "***" can only be used when updating a grader with the same id as a stored grader`,
				})
			}
		}
	}
	if len(sentinelErrs) > 0 {
		writeValidationErrors(w, sentinelErrs)
		return
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
	suiteID := r.PathValue("suite_id")
	caseID := r.PathValue("case_id")

	// Verify ownership before deleting — prevents cross-suite IDOR.
	tc, err := h.store.GetTestCase(r.Context(), caseID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if tc.SuiteID != suiteID {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "test case not found")
		return
	}

	if err := h.store.DeleteTestCase(r.Context(), caseID); err != nil {
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

// ---- Run endpoints ---------------------------------------------------------

// TriggerRun handles POST /v1/eval-suites/{suite_id}/runs.
// Returns 201 with the new EvalRun ID immediately; execution is async.
func (h *Handler) TriggerRun(w http.ResponseWriter, r *http.Request) {
	suiteID := r.PathValue("suite_id")

	if h.runner == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "eval run execution is not yet available")
		return
	}

	evalRunID, err := h.runner.Execute(r.Context(), suiteID, "manual")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		if errors.Is(err, ErrWorkflowDeleted) {
			writeError(w, http.StatusBadRequest, "WORKFLOW_DELETED", "linked workflow has been deleted")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": evalRunID})
}

// WebhookTrigger handles POST /v1/eval-webhooks/{suite_id}.
// Validates the Authorization: Bearer <token> header against the suite's
// stored webhook secret and triggers an async eval run on success.
func (h *Handler) WebhookTrigger(w http.ResponseWriter, r *http.Request) {
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
	if suite.WorkflowDeleted {
		writeError(w, http.StatusBadRequest, "WORKFLOW_DELETED", "linked workflow has been deleted")
		return
	}
	if suite.TriggerKind != "webhook" {
		writeError(w, http.StatusBadRequest, "INVALID_TRIGGER", "suite is not configured for webhook trigger")
		return
	}

	// RFC 7617: the "Bearer" scheme name is case-insensitive.
	authHeader := r.Header.Get("Authorization")
	const bearerPrefix = "bearer "
	if len(authHeader) <= len(bearerPrefix) || !strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization: Bearer <token> header required")
		return
	}
	token := authHeader[len(bearerPrefix):]
	if token == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization: Bearer <token> header required")
		return
	}

	plainSecret, err := h.vault.DecryptValue(suite.WebhookSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify webhook secret")
		return
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(plainSecret)) != 1 {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid webhook token")
		return
	}

	if h.runner == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "eval run execution is not yet available")
		return
	}

	evalRunID, err := h.runner.Execute(r.Context(), suiteID, "webhook")
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval suite not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"eval_run_id": evalRunID})
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
		"id":                run.ID,
		"suite_id":          run.SuiteID,
		"status":            run.Status,
		"triggered_by":      run.TriggeredBy,
		"total_cases":       run.TotalCases,
		"passed_count":      run.PassedCount,
		"failed_count":      run.FailedCount,
		"error_count":       run.ErrorCount,
		"started_at":        run.StartedAt,
		"finished_at":       run.FinishedAt,
		"created_at":        run.CreatedAt,
		"test_case_results": results,
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

// ---- Baseline comparison ---------------------------------------------------

type evalRunCompareChangeType string

const (
	changeTypeRegressed evalRunCompareChangeType = "regressed"
	changeTypeImproved  evalRunCompareChangeType = "improved"
	changeTypeUnchanged evalRunCompareChangeType = "unchanged"
	changeTypeNewCase   evalRunCompareChangeType = "new_case"
	changeTypeMissing   evalRunCompareChangeType = "missing"
)

// changeOrder defines the display priority for comparison results (regressions first).
var changeOrder = map[evalRunCompareChangeType]int{
	changeTypeRegressed: 0,
	changeTypeImproved:  1,
	changeTypeNewCase:   2,
	changeTypeMissing:   3,
	changeTypeUnchanged: 4,
}

type testCaseComparison struct {
	TestCaseID       string                   `json:"test_case_id"`
	TestCaseName     string                   `json:"test_case_name"`
	ChangeType       evalRunCompareChangeType `json:"change_type"`
	HeadPassed       *bool                    `json:"head_passed"`
	BaselinePassed   *bool                    `json:"baseline_passed"`
	HeadResultID     string                   `json:"head_result_id,omitempty"`
	BaselineResultID string                   `json:"baseline_result_id,omitempty"`
}

type evalRunCompareResponse struct {
	HeadRunID      string               `json:"head_run_id"`
	BaselineRunID  string               `json:"baseline_run_id"`
	SuiteID        string               `json:"suite_id"`
	RegressedCount int                  `json:"regressed_count"`
	ImprovedCount  int                  `json:"improved_count"`
	UnchangedCount int                  `json:"unchanged_count"`
	NewCaseCount   int                  `json:"new_case_count"`
	MissingCount   int                  `json:"missing_count"`
	Cases          []testCaseComparison `json:"cases"`
}

// CompareRuns handles GET /v1/eval-runs/{eval_run_id}/compare?baseline_run_id={id}.
// Both runs must be completed and belong to the same eval suite.
func (h *Handler) CompareRuns(w http.ResponseWriter, r *http.Request) {
	headRunID := r.PathValue("eval_run_id")
	baselineRunID := r.URL.Query().Get("baseline_run_id")
	if baselineRunID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "baseline_run_id query parameter is required")
		return
	}
	if headRunID == baselineRunID {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "head and baseline runs must be different")
		return
	}

	ctx := r.Context()
	headRun, err := h.store.GetEvalRun(ctx, headRunID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "eval run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	baselineRun, err := h.store.GetEvalRun(ctx, baselineRunID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "baseline eval run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if headRun.SuiteID != baselineRun.SuiteID {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "both runs must belong to the same eval suite")
		return
	}
	if headRun.Status != store.EvalRunCompleted {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "head run must have completed status")
		return
	}
	if baselineRun.Status != store.EvalRunCompleted {
		writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "baseline run must have completed status")
		return
	}

	headResults, err := h.store.ListTestCaseResults(ctx, headRunID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	baselineResults, err := h.store.ListTestCaseResults(ctx, baselineRunID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	headByCase := make(map[string]store.TestCaseResult, len(headResults))
	for _, res := range headResults {
		headByCase[res.TestCaseID] = res
	}
	baseByCase := make(map[string]store.TestCaseResult, len(baselineResults))
	for _, res := range baselineResults {
		baseByCase[res.TestCaseID] = res
	}

	allIDs := make(map[string]struct{}, len(headByCase)+len(baseByCase))
	for id := range headByCase {
		allIDs[id] = struct{}{}
	}
	for id := range baseByCase {
		allIDs[id] = struct{}{}
	}

	resp := evalRunCompareResponse{
		HeadRunID:     headRunID,
		BaselineRunID: baselineRunID,
		SuiteID:       headRun.SuiteID,
		Cases:         []testCaseComparison{},
	}

	for id := range allIDs {
		head, inHead := headByCase[id]
		base, inBase := baseByCase[id]

		var comp testCaseComparison
		switch {
		case inHead && inBase:
			hp, bp := head.Passed, base.Passed
			comp = testCaseComparison{
				TestCaseID:       id,
				TestCaseName:     head.TestCaseName,
				HeadPassed:       &hp,
				BaselinePassed:   &bp,
				HeadResultID:     head.ID,
				BaselineResultID: base.ID,
			}
			switch {
			case !hp && bp:
				comp.ChangeType = changeTypeRegressed
				resp.RegressedCount++
			case hp && !bp:
				comp.ChangeType = changeTypeImproved
				resp.ImprovedCount++
			default:
				comp.ChangeType = changeTypeUnchanged
				resp.UnchangedCount++
			}
		case inHead:
			hp := head.Passed
			comp = testCaseComparison{
				TestCaseID:   id,
				TestCaseName: head.TestCaseName,
				ChangeType:   changeTypeNewCase,
				HeadPassed:   &hp,
				HeadResultID: head.ID,
			}
			resp.NewCaseCount++
		default:
			bp := base.Passed
			comp = testCaseComparison{
				TestCaseID:       id,
				TestCaseName:     base.TestCaseName,
				ChangeType:       changeTypeMissing,
				BaselinePassed:   &bp,
				BaselineResultID: base.ID,
			}
			resp.MissingCount++
		}
		resp.Cases = append(resp.Cases, comp)
	}

	sort.SliceStable(resp.Cases, func(i, j int) bool {
		oi, oj := changeOrder[resp.Cases[i].ChangeType], changeOrder[resp.Cases[j].ChangeType]
		if oi != oj {
			return oi < oj
		}
		if resp.Cases[i].TestCaseName != resp.Cases[j].TestCaseName {
			return resp.Cases[i].TestCaseName < resp.Cases[j].TestCaseName
		}
		return resp.Cases[i].TestCaseID < resp.Cases[j].TestCaseID
	})

	writeJSON(w, http.StatusOK, resp)
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
		case "llm_judge":
			rubric, _ := g.Config["rubric"].(string)
			if rubric == "" {
				errs = append(errs, fieldValidationError{
					Field:   prefix + ".config.rubric",
					Message: "rubric is required for llm_judge grader",
				})
			}
			errs = append(errs, validateLLMProvider(g.Config, prefix)...)
		case "checklist":
			if !hasNonEmptyCriteria(g.Config) {
				errs = append(errs, fieldValidationError{
					Field:   prefix + ".config.criteria",
					Message: "criteria must be a non-empty array of strings",
				})
			}
			errs = append(errs, validateLLMProvider(g.Config, prefix)...)
		}
	}
	return errs
}

// validateLLMProvider checks that provider is one of the supported values.
func validateLLMProvider(config map[string]any, prefix string) []fieldValidationError {
	provider, _ := config["provider"].(string)
	if provider != "openai" && provider != "anthropic" {
		return []fieldValidationError{{
			Field:   prefix + ".config.provider",
			Message: `provider must be "openai" or "anthropic"`,
		}}
	}
	return nil
}

// hasNonEmptyCriteria returns true when config["criteria"] is a non-empty array.
func hasNonEmptyCriteria(config map[string]any) bool {
	switch v := config["criteria"].(type) {
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	}
	return false
}

// ---- Helpers ---------------------------------------------------------------

func applyDefaults(s *store.EvalSuite) {
	if s.PassThreshold == 0 {
		s.PassThreshold = 1.0
	}
	if s.MaxConcurrency == 0 {
		s.MaxConcurrency = 1
	}
	if s.TriggerKind == "" {
		s.TriggerKind = "none"
	}
}

// applyTrigger validates trigger config and, for webhook suites, generates or
// rotates the secret. generateSecret should be true on create and on secret rotation.
// Returns the plain-text secret (non-empty only when a new secret was generated).
// Mutates s.WebhookSecret in place with the encrypted value before returning.
func (h *Handler) applyTrigger(s *store.EvalSuite, generateSecret bool) (string, error) {
	switch s.TriggerKind {
	case "", "none":
		s.TriggerKind = "none"
		s.CronExpr = ""
		s.WebhookSecret = ""
	case "cron":
		if err := trigger.ValidateCronExpr(s.CronExpr); err != nil {
			return "", fmt.Errorf("invalid cron_expr: %w", err)
		}
		s.WebhookSecret = ""
	case "webhook":
		if generateSecret {
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return "", fmt.Errorf("generate webhook secret: %w", err)
			}
			plain := hex.EncodeToString(raw)
			enc, err := h.vault.EncryptValue(plain)
			if err != nil {
				return "", fmt.Errorf("encrypt webhook secret: %w", err)
			}
			s.WebhookSecret = enc
			return plain, nil
		}
		// Keep existing encrypted secret unchanged.
	default:
		return "", fmt.Errorf("trigger_kind must be one of: none, cron, webhook")
	}
	return "", nil
}

// suiteResponse builds the JSON map for suite create/get/update responses.
// plainSecret is non-empty only on create or secret rotation (one-time reveal).
// On all other reads the webhook_secret field is omitted or masked.
func suiteResponse(s store.EvalSuite, plainSecret string) map[string]any {
	resp := map[string]any{
		"id":               s.ID,
		"workflow_id":      s.WorkflowID,
		"name":             s.Name,
		"description":      s.Description,
		"pass_threshold":   s.PassThreshold,
		"max_concurrency":  s.MaxConcurrency,
		"workflow_deleted": s.WorkflowDeleted,
		"trigger_kind":     s.TriggerKind,
		"created_at":       s.CreatedAt,
		"updated_at":       s.UpdatedAt,
	}
	switch s.TriggerKind {
	case "cron":
		resp["cron_expr"] = s.CronExpr
	case "webhook":
		resp["webhook_url"] = "/v1/eval-webhooks/" + s.ID
		if plainSecret != "" {
			resp["webhook_secret"] = plainSecret
		} else {
			resp["webhook_secret"] = "***"
		}
	}
	return resp
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

