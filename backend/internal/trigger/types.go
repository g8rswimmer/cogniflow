package trigger

import "context"

// RunRequest is the unified trigger payload regardless of trigger source.
// All three trigger types (manual, webhook, cron) converge on this type.
type RunRequest struct {
	WorkflowID            string
	InitialData           map[string]any
	TriggeredBy           string                    // "manual" | "webhook" | "cron" | "eval"
	NodeMocks             map[string]map[string]any // optional; eval engine sets this to stub nodes
	WorkflowVersionNumber *int                      // optional; set by callers that already know the version
}

// Dispatcher is the shared sink for all trigger types. engine.WorkflowEngine implements it.
type Dispatcher interface {
	Dispatch(ctx context.Context, req RunRequest) (string, error) // returns run_id
}
