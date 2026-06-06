package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("not found")

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
}

// Trigger describes how a workflow is activated.
type Trigger struct {
	Kind     string `json:"kind"`                // manual | webhook | cron
	CronExpr string `json:"cron_expr,omitempty"` // when kind == cron
}

// Workflow is the full definition of a workflow including its graph.
type Workflow struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Trigger        Trigger        `json:"trigger"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	Nodes          []WorkflowNode `json:"nodes"`
	Edges          []WorkflowEdge `json:"edges"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
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

// RunStatus represents the lifecycle state of a workflow run.
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
)

// Run is one execution instance of a workflow.
type Run struct {
	ID          string         `json:"run_id"`
	WorkflowID  string         `json:"workflow_id"`
	TriggeredBy string         `json:"triggered_by"`
	Status      RunStatus      `json:"status"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	FinalOutput map[string]any `json:"final_output,omitempty"`
	ErrorDetail map[string]any `json:"error_detail,omitempty"`
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
	// Workflow CRUD
	CreateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
	GetWorkflow(ctx context.Context, id string) (Workflow, error)
	ListWorkflows(ctx context.Context) ([]WorkflowSummary, error)
	UpdateWorkflow(ctx context.Context, w Workflow) (Workflow, error)
	DeleteWorkflow(ctx context.Context, id string) error

	// Runs
	CreateRun(ctx context.Context, r Run) (Run, error)
	UpdateRunStatus(ctx context.Context, runID string, status RunStatus, output map[string]any) error
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
}
