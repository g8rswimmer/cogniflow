package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicateEmail is returned when a user with the same email already exists.
var ErrDuplicateEmail = errors.New("email already in use")

// ---- Multi-tenancy context helpers ------------------------------------------

type orgIDCtxKey struct{}

// WithOrgID stores the tenant org_id in the context so store implementations
// can scope queries without changing every method signature.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDCtxKey{}, orgID)
}

// OrgIDFrom retrieves the org_id from the context. Returns "" if not set
// (system_admin path — no tenant filter applied).
func OrgIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(orgIDCtxKey{}).(string)
	return s
}

// ---- Auth domain types ------------------------------------------------------

// Organization is a tenant in the system.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// DefaultPermissions is the full set of scopes granted to a new member by default.
var DefaultPermissions = []string{
	"workflow:read", "workflow:write", "workflow:run",
	"eval:read", "eval:write", "eval:run",
}

// User is an authenticated principal belonging to one organization.
type User struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never serialised to API responses
	Role         string    `json:"role"` // system_admin | org_admin | member
	Permissions  []string  `json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OrgEmailSettings holds per-org SMTP credentials and the Go text/template for
// invite emails. SMTPPassword is stored AES-256-GCM encrypted (enc:<base64>)
// and is returned in plaintext by ConfigVault; callers must mask it before
// including it in API responses. When SMTPHost is empty, email sending is
// disabled for the org. When Subject/Body are empty, built-in defaults are used.
type OrgEmailSettings struct {
	OrgID        string    `json:"org_id"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     string    `json:"smtp_port"`
	SMTPUser     string    `json:"smtp_user"`
	SMTPPassword string    `json:"smtp_password"` // plaintext in memory; encrypted at rest
	SMTPFrom     string    `json:"smtp_from"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Invitation is a pending invite that creates a User on acceptance.
type Invitation struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Permissions []string   `json:"permissions"`
	Token       string     `json:"token"`
	InvitedBy   string     `json:"invited_by"`
	ExpiresAt   time.Time  `json:"expires_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// NodePosition holds the canvas coordinates of a workflow node.
type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// RetryPolicy configures per-node retry behaviour.
type RetryPolicy struct {
	MaxRetries int `json:"max_retries"`
	BackoffMs  int `json:"backoff_ms"`
}

// OutputParser defines how to extract a named value from a node's raw output after execution.
// Extracted fields are merged into the node's output and become available to downstream nodes
// via template syntax (e.g. {{.n1.extracted_field}}).
type OutputParser struct {
	// Kind is the extraction method: "json_path" or "regex".
	Kind string `json:"kind"`
	// Source is the field in the node's raw output to extract from (e.g. "completion").
	Source string `json:"source"`
	// Pattern is the gjson path (for json_path) or regular expression (for regex).
	Pattern string `json:"pattern"`
	// CaptureGroup is the regex capture group index to return (0 = full match, 1 = first group, etc).
	// Ignored when Kind is "json_path".
	CaptureGroup int `json:"capture_group,omitempty"`
}

// WorkflowNode is one node instance in a workflow graph.
type WorkflowNode struct {
	ID            string                    `json:"id"`
	TypeID        string                    `json:"type_id"`
	Label         string                    `json:"label,omitempty"`
	Position      NodePosition              `json:"position"`
	Config        map[string]any            `json:"config,omitempty"`
	SensitiveKeys map[string]bool           `json:"-"` // keys encrypted at rest; set by config vault
	RetryPolicy   *RetryPolicy              `json:"retry_policy,omitempty"`
	OutputParsers map[string]OutputParser   `json:"output_parsers,omitempty"`
}

// WorkflowEdge is a directed edge between two nodes.
type WorkflowEdge struct {
	ID          string  `json:"id"`
	SourceID    string  `json:"source_id"`
	TargetID    string  `json:"target_id"`
	BranchLabel *string `json:"branch_label"`
	IsLoopBack  bool    `json:"is_loop_back"`
}

// Trigger describes how a workflow is activated.
type Trigger struct {
	Kind     string `json:"kind"`                // manual | webhook | cron | kafka | sqs
	CronExpr string `json:"cron_expr,omitempty"` // when kind == cron

	// Kafka trigger fields
	KafkaBrokers string `json:"kafka_brokers,omitempty"` // comma-separated host:port list
	KafkaTopic   string `json:"kafka_topic,omitempty"`
	KafkaGroupID string `json:"kafka_group_id,omitempty"` // defaults to "cogniflow-{workflowID}"

	// SQS trigger fields
	SQSQueueURL string `json:"sqs_queue_url,omitempty"`
	SQSRegion   string `json:"sqs_region,omitempty"`
}

// Workflow is the full definition of a workflow including its graph.
type Workflow struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Trigger           Trigger         `json:"trigger"`
	TimeoutSeconds    int             `json:"timeout_seconds"`
	Nodes             []WorkflowNode  `json:"nodes"`
	Edges             []WorkflowEdge  `json:"edges"`
	InitialDataSchema json.RawMessage `json:"initial_data_schema,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// WorkflowSummary is a lightweight view of a workflow for list responses.
type WorkflowSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	TriggerKind    string    `json:"trigger_kind"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	NodeCount      int       `json:"node_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// WorkflowVersionSummary is a lightweight view of a workflow version for list responses.
type WorkflowVersionSummary struct {
	ID            string    `json:"id"`
	WorkflowID    string    `json:"workflow_id"`
	VersionNumber int       `json:"version_number"`
	NodeCount     int       `json:"node_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// WorkflowVersion is a full point-in-time snapshot of a workflow definition.
type WorkflowVersion struct {
	ID            string    `json:"id"`
	WorkflowID    string    `json:"workflow_id"`
	VersionNumber int       `json:"version_number"`
	Definition    Workflow  `json:"definition"`
	CreatedAt     time.Time `json:"created_at"`
}

// RunStatus represents the lifecycle state of a workflow run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
)

// NodeResult is the persisted outcome of a single node execution within a run.
type NodeResult struct {
	Status string         `json:"status"`           // "succeeded" | "failed"
	Output map[string]any `json:"output,omitempty"` // populated on succeeded
	Error  string         `json:"error,omitempty"`  // populated on failed
}

// Run is one execution instance of a workflow.
type Run struct {
	ID                    string                `json:"run_id"`
	WorkflowID            string                `json:"workflow_id"`
	TriggeredBy           string                `json:"triggered_by"`
	Status                RunStatus             `json:"status"`
	WorkflowVersionNumber *int                  `json:"workflow_version_number"`
	StartedAt             *time.Time            `json:"started_at,omitempty"`
	FinishedAt            *time.Time            `json:"finished_at,omitempty"`
	FinalOutput           map[string]any        `json:"final_output,omitempty"`
	ErrorDetail           map[string]any        `json:"error_detail,omitempty"`
	NodeResults           map[string]NodeResult `json:"node_results,omitempty"`
}

// RunFilter constrains ListRuns queries.
type RunFilter struct {
	WorkflowID string
	Status     RunStatus
	Since      time.Time
	Until      time.Time
	Limit      int
}

// TriggerConfig is the persisted trigger configuration for a workflow.
type TriggerConfig struct {
	Kind     string
	CronExpr string

	// Kafka
	KafkaBrokers string
	KafkaTopic   string
	KafkaGroupID string

	// SQS
	SQSQueueURL string
	SQSRegion   string
}

// WorkflowTrigger pairs a workflow ID with its trigger config.
type WorkflowTrigger struct {
	WorkflowID string
	Config     TriggerConfig
}

// RAGChunk is a text chunk with its embedding for vector storage.
type RAGChunk struct {
	ID         string
	DocumentID string
	ChunkIndex int
	ChunkText  string
	Embedding  []float32
}

// RAGChunkResult is a retrieved chunk with its similarity score.
type RAGChunkResult struct {
	ID        string  `json:"id"`
	ChunkText string  `json:"chunk_text"`
	Score     float32 `json:"score"`
}

// GraderRegistration is a persisted out-of-process gRPC grader plugin registration.
type GraderRegistration struct {
	TypeID       string          `json:"type_id"`
	Address      string          `json:"address"`
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	RegisteredAt time.Time       `json:"registered_at"`
}

// PluginRegistration is a persisted out-of-process gRPC plugin registration.
// Plugins registered via PLUGIN_ADDRESSES (ephemeral) are not stored here.
type PluginRegistration struct {
	TypeID       string          `json:"type_id"`
	Address      string          `json:"address"`
	DisplayName  string          `json:"display_name"`
	Category     string          `json:"category"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	RegisteredAt time.Time       `json:"registered_at"`
}

// Store is the persistence interface. The MySQL implementation lives in
// internal/store/mysql/. Tests use an in-memory stub.
type Store interface {
	// Organizations
	CreateOrganization(ctx context.Context, org Organization) (Organization, error)
	GetOrganization(ctx context.Context, id string) (Organization, error)
	ListOrganizations(ctx context.Context) ([]Organization, error)
	DeleteOrganization(ctx context.Context, id string) error

	// Users
	CreateUser(ctx context.Context, u User) (User, error)
	GetUser(ctx context.Context, id string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	// ListUsers returns all users in orgID; when orgID is empty all users are returned (system_admin).
	ListUsers(ctx context.Context, orgID string) ([]User, error)
	UpdateUserRole(ctx context.Context, userID, role string) error
	UpdateUserPermissions(ctx context.Context, userID string, permissions []string) error
	DeleteUser(ctx context.Context, userID string) error

	// Invitations
	CreateInvitation(ctx context.Context, inv Invitation) (Invitation, error)
	GetInvitationByToken(ctx context.Context, token string) (Invitation, error)
	AcceptInvitation(ctx context.Context, invID string, now time.Time) error

	// Org Email Settings (SMTP + invite template)
	UpsertOrgEmailSettings(ctx context.Context, s OrgEmailSettings) error
	GetOrgEmailSettings(ctx context.Context, orgID string) (OrgEmailSettings, error)
	DeleteOrgEmailSettings(ctx context.Context, orgID string) error

	// Workflow CRUD
	CreateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
	GetWorkflow(ctx context.Context, id string) (Workflow, error)
	// GetWorkflowSchema returns only the initial_data_schema for a workflow.
	// It is cheaper than GetWorkflow (single-column SELECT, no node/edge/config load)
	// and is used by the run trigger for advisory schema validation.
	// Returns ErrNotFound if the workflow does not exist.
	GetWorkflowSchema(ctx context.Context, id string) (json.RawMessage, error)
	ListWorkflows(ctx context.Context) ([]WorkflowSummary, error)
	UpdateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
	DeleteWorkflow(ctx context.Context, id string) error

	// Workflow Versions
	CreateWorkflowVersion(ctx context.Context, w Workflow) error
	GetLatestWorkflowVersionNumber(ctx context.Context, workflowID string) (*int, error)
	ListWorkflowVersions(ctx context.Context, workflowID string) ([]WorkflowVersionSummary, error)
	GetWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (WorkflowVersion, error)
	DeleteWorkflowVersions(ctx context.Context, workflowID string) error
	RestoreWorkflowVersion(ctx context.Context, workflowID string, versionNum int) (Workflow, error)

	// Runs
	CreateRun(ctx context.Context, r Run) (Run, error)
	UpdateRunStatus(ctx context.Context, runID string, status RunStatus, output map[string]any) error
	SaveRunNodeResults(ctx context.Context, runID string, results map[string]NodeResult) error
	GetRun(ctx context.Context, runID string) (Run, error)
	ListRuns(ctx context.Context, f RunFilter) ([]Run, error)

	// RAG
	UpsertChunks(ctx context.Context, chunks []RAGChunk) error
	SearchChunks(ctx context.Context, embedding []float32, topK int, docFilter string) ([]RAGChunkResult, error)

	// Triggers
	SaveTriggerConfig(ctx context.Context, workflowID string, cfg TriggerConfig) error
	GetTriggerConfig(ctx context.Context, workflowID string) (TriggerConfig, error)
	ListTriggerConfigs(ctx context.Context) ([]WorkflowTrigger, error)

	// Plugin Registrations
	SavePluginRegistration(ctx context.Context, reg PluginRegistration) error
	GetPluginRegistration(ctx context.Context, typeID string) (PluginRegistration, error)
	ListPluginRegistrations(ctx context.Context) ([]PluginRegistration, error)
	DeletePluginRegistration(ctx context.Context, typeID string) error

	// Grader Plugin Registrations
	SaveGraderRegistration(ctx context.Context, reg GraderRegistration) error
	GetGraderRegistration(ctx context.Context, typeID string) (GraderRegistration, error)
	ListGraderRegistrations(ctx context.Context) ([]GraderRegistration, error)
	DeleteGraderRegistration(ctx context.Context, typeID string) error

	// Eval Suites
	CreateEvalSuite(ctx context.Context, s EvalSuite) (EvalSuite, error)
	GetEvalSuite(ctx context.Context, id string) (EvalSuite, error)
	ListEvalSuites(ctx context.Context, workflowID string) ([]EvalSuiteSummary, error)
	// ListEvalSuitesByCronTrigger returns all suites with trigger_kind == "cron".
	// Used at startup to re-arm the eval scheduler after a server restart.
	ListEvalSuitesByCronTrigger(ctx context.Context) ([]EvalSuite, error)
	UpdateEvalSuite(ctx context.Context, s EvalSuite) (EvalSuite, error)
	DeleteEvalSuite(ctx context.Context, id string) error

	// Test Cases
	CreateTestCase(ctx context.Context, tc TestCase) (TestCase, error)
	GetTestCase(ctx context.Context, id string) (TestCase, error)
	ListTestCases(ctx context.Context, suiteID string) ([]TestCase, error)
	UpdateTestCase(ctx context.Context, tc TestCase) (TestCase, error)
	DeleteTestCase(ctx context.Context, id string) error
	ReorderTestCases(ctx context.Context, suiteID string, orderedIDs []string) error

	// Eval Runs
	CreateEvalRun(ctx context.Context, r EvalRun) (EvalRun, error)
	GetEvalRun(ctx context.Context, id string) (EvalRun, error)
	ListEvalRuns(ctx context.Context, f EvalRunFilter) ([]EvalRun, error)
	UpdateEvalRunStatus(ctx context.Context, runID string, status EvalRunStatus, counts EvalRunCounts) error
	IncrementEvalRunCounts(ctx context.Context, runID string, delta EvalRunCounts) error

	// Test Case Results
	CreateTestCaseResult(ctx context.Context, r TestCaseResult) (TestCaseResult, error)
	GetTestCaseResult(ctx context.Context, id string) (TestCaseResult, error)
	ListTestCaseResults(ctx context.Context, evalRunID string) ([]TestCaseResult, error)
}

// ---- Eval types ----------------------------------------------------------

// GraderVerdict is the outcome of a single grader evaluation.
type GraderVerdict string

const (
	VerdictPass  GraderVerdict = "pass"
	VerdictFail  GraderVerdict = "fail"
	VerdictError GraderVerdict = "error"
)

// CriterionResult is the per-item outcome for the Checklist grader (GR-05).
type CriterionResult struct {
	Criterion   string `json:"criterion"`
	Met         bool   `json:"met"`
	Explanation string `json:"explanation"`
}

// GraderResult is the outcome of one Grader evaluation within a TestCase.
type GraderResult struct {
	GraderID        string            `json:"grader_id"`
	GraderName      string            `json:"grader_name"`
	GraderType      string            `json:"grader_type"`
	Verdict         GraderVerdict     `json:"verdict"`
	Score           *float64          `json:"score,omitempty"`
	Explanation     string            `json:"explanation"`
	ActualValue     any               `json:"actual_value,omitempty"`
	CriteriaResults []CriterionResult `json:"criteria_results,omitempty"`
}

// GraderDef is the persisted grader definition stored in eval_test_cases.graders.
// Sensitive api_key values are encrypted at rest; MaskGraders replaces them
// with "***" before including in API responses.
type GraderDef struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`   // "string_match"|"numeric_threshold"|"llm_judge"|"json_schema"|"checklist"
	Scope  string         `json:"scope"`  // "workflow"|"node"
	NodeID string         `json:"node_id,omitempty"`
	Config map[string]any `json:"config"`
}

// NodeMock overrides a node's Execute() call during eval runs.
type NodeMock struct {
	NodeID string         `json:"node_id"`
	Output map[string]any `json:"output"`
}

// EvalSuite is a named collection of test cases linked to one workflow.
type EvalSuite struct {
	ID              string    `json:"id"               db:"id"`
	WorkflowID      string    `json:"workflow_id"      db:"workflow_id"`
	Name            string    `json:"name"             db:"name"`
	Description     string    `json:"description"      db:"description"`
	PassThreshold   float64   `json:"pass_threshold"   db:"pass_threshold"`
	MaxConcurrency  int       `json:"max_concurrency"  db:"max_concurrency"`
	WorkflowDeleted bool      `json:"workflow_deleted" db:"workflow_deleted"`
	// TriggerKind is "none", "cron", or "webhook".
	TriggerKind string `json:"trigger_kind" db:"trigger_kind"`
	// CronExpr is populated when TriggerKind == "cron". Stored inside trigger_config JSON.
	CronExpr string `json:"cron_expr,omitempty"`
	// WebhookSecret holds the AES-256-GCM encrypted secret ("enc:...") as read
	// from the DB. The handler decrypts it for use and masks it in API responses.
	WebhookSecret string `json:"webhook_secret,omitempty"`
	CreatedAt     time.Time `json:"created_at"       db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"       db:"updated_at"`
}

// EvalSuiteSummary is EvalSuite enriched with aggregate fields for list responses.
type EvalSuiteSummary struct {
	EvalSuite
	TestCaseCount int        `json:"test_case_count" db:"test_case_count"`
	LastRunStatus *string    `json:"last_run_status" db:"last_run_status"`
	LastRunAt     *time.Time `json:"last_run_at"     db:"last_run_at"`
}

// TestCase is one scenario within an EvalSuite.
type TestCase struct {
	ID          string         `json:"id"          db:"id"`
	SuiteID     string         `json:"suite_id"    db:"suite_id"`
	Name        string         `json:"name"        db:"name"`
	Description string         `json:"description" db:"description"`
	Position    int            `json:"position"    db:"position"`
	InitialData map[string]any `json:"initial_data"`
	Mocks       []NodeMock     `json:"mocks"`
	Graders     []GraderDef    `json:"graders"`
	CreatedAt   time.Time      `json:"created_at"  db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"  db:"updated_at"`
}

// EvalRunStatus is the lifecycle state of an EvalRun.
type EvalRunStatus string

const (
	EvalRunPending   EvalRunStatus = "pending"
	EvalRunRunning   EvalRunStatus = "running"
	EvalRunCompleted EvalRunStatus = "completed"
	EvalRunFailed    EvalRunStatus = "failed"
)

// EvalRun is one async execution of an EvalSuite.
type EvalRun struct {
	ID                    string        `json:"id"                               db:"id"`
	SuiteID               string        `json:"suite_id"                         db:"suite_id"`
	TriggeredBy           string        `json:"triggered_by"                     db:"triggered_by"`
	Status                EvalRunStatus `json:"status"                           db:"status"`
	TotalCases            int           `json:"total_cases"                      db:"total_cases"`
	PassedCount           int           `json:"passed_count"                     db:"passed_count"`
	FailedCount           int           `json:"failed_count"                     db:"failed_count"`
	ErrorCount            int           `json:"error_count"                      db:"error_count"`
	WorkflowVersionNumber *int          `json:"workflow_version_number"           db:"workflow_version_number"`
	StartedAt             *time.Time    `json:"started_at"                       db:"started_at"`
	FinishedAt            *time.Time    `json:"finished_at"                      db:"finished_at"`
	CreatedAt             time.Time     `json:"created_at"                       db:"created_at"`
}

// EvalRunFilter constrains ListEvalRuns queries.
type EvalRunFilter struct {
	SuiteID string
	Status  EvalRunStatus
	Limit   int
	Offset  int
}

// EvalRunCounts holds pass/fail/error deltas or totals for EvalRun updates.
type EvalRunCounts struct {
	PassedCount int
	FailedCount int
	ErrorCount  int
}

// TestCaseResult is the outcome of one TestCase within an EvalRun.
type TestCaseResult struct {
	ID                string                    `json:"id"                  db:"id"`
	EvalRunID         string                    `json:"eval_run_id"         db:"eval_run_id"`
	TestCaseID        string                    `json:"test_case_id"        db:"test_case_id"`
	TestCaseName      string                    `json:"test_case_name"      db:"test_case_name"`
	WorkflowRunID     string                    `json:"workflow_run_id"     db:"workflow_run_id"`
	WorkflowRunStatus string                    `json:"workflow_run_status" db:"workflow_run_status"`
	NodeOutputs       map[string]map[string]any `json:"node_outputs"`
	GraderResults     []GraderResult            `json:"grader_results"`
	Passed            bool                      `json:"passed"              db:"passed"`
	CreatedAt         time.Time                 `json:"created_at"          db:"created_at"`
}
