package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/anthropic"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/openai"
	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/eval"
	"github.com/g8rswimmer/cogniflow/internal/node"
	nodeplugin "github.com/g8rswimmer/cogniflow/internal/node/plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// NewRouter wires all HTTP routes and returns the handler wrapped with
// CORS, request-ID, and access-log middleware.
// srvCtx is a server-lifetime context cancelled on shutdown; it is passed to
// the EvalRunner so background goroutines stop when the server goes down.
func NewRouter(
	srvCtx context.Context,
	db *sqlx.DB,
	st store.Store,
	registry *node.NodeRegistry,
	dispatcher trigger.Dispatcher,
	bus *engine.EventBus,
	eng *engine.WorkflowEngine,
	cipher *crypto.Cipher,
	tm *trigger.Manager,
	level *slog.LevelVar,
) http.Handler {
	mux := http.NewServeMux()

	// Infrastructure endpoints — unversioned.
	mux.Handle("GET /health", newHealthHandler(db))
	llh := &logLevelHandler{level: level}
	mux.HandleFunc("GET /admin/log-level", llh.get)
	mux.HandleFunc("PUT /admin/log-level", llh.set)

	// v1 API.
	wh := &workflowHandler{store: st, registry: registry, triggers: tm}
	mux.HandleFunc("GET /v1/workflows", wh.list)
	mux.HandleFunc("POST /v1/workflows", wh.create)
	mux.HandleFunc("GET /v1/workflows/{id}", wh.get)
	mux.HandleFunc("PUT /v1/workflows/{id}", wh.update)
	mux.HandleFunc("DELETE /v1/workflows/{id}", wh.delete)

	nth := &nodeTypeHandler{registry: registry}
	mux.HandleFunc("GET /v1/node-types", nth.list)

	rh := &runHandler{store: st, dispatcher: dispatcher}
	mux.HandleFunc("POST /v1/workflows/{id}/runs", rh.triggerRun)
	mux.HandleFunc("GET /v1/workflows/{id}/runs", rh.listRuns)
	mux.HandleFunc("GET /v1/runs/{run_id}", rh.getRun)

	wsh := &wsHandler{store: st, bus: bus}
	mux.HandleFunc("GET /v1/runs/{run_id}/events", wsh.streamEvents)

	mux.HandleFunc("POST /v1/webhooks/{workflow_id}", tm.WebhookHandler())

	pah := &pluginAdminHandler{store: st, registry: registry, registerFn: nodeplugin.RegisterOne}
	mux.HandleFunc("GET /v1/admin/plugins", pah.list)
	mux.HandleFunc("POST /v1/admin/plugins", pah.register)
	mux.HandleFunc("PUT /v1/admin/plugins/{type_id}", pah.update)
	mux.HandleFunc("DELETE /v1/admin/plugins/{type_id}", pah.deregister)

	// Eval routes — suite CRUD, test case CRUD, and run execution.
	vault := eval.NewGraderVault(cipher)
	runner := eval.NewEvalRunner(srvCtx, st, eng, vault, newLLMFactory())

	evalSched := eval.NewEvalScheduler(srvCtx, runner)
	if cronSuites, err := st.ListEvalSuitesByCronTrigger(srvCtx); err != nil {
		slog.Warn("eval scheduler: could not load cron suites at startup", "error", err)
	} else {
		evalSched.LoadAll(cronSuites)
	}
	evalSched.Start()

	eh := eval.NewHandler(st, vault, registry, runner, evalSched)

	mux.HandleFunc("GET /v1/workflows/{workflow_id}/eval-suites", eh.ListByWorkflow)
	mux.HandleFunc("POST /v1/workflows/{workflow_id}/eval-suites", eh.CreateSuite)
	mux.HandleFunc("GET /v1/eval-suites/{suite_id}", eh.GetSuite)
	mux.HandleFunc("PUT /v1/eval-suites/{suite_id}", eh.UpdateSuite)
	mux.HandleFunc("DELETE /v1/eval-suites/{suite_id}", eh.DeleteSuite)
	mux.HandleFunc("GET /v1/eval-suites/{suite_id}/test-cases", eh.ListCases)
	mux.HandleFunc("POST /v1/eval-suites/{suite_id}/test-cases", eh.CreateCase)
	// /order must be registered before /{case_id} so it is matched first.
	mux.HandleFunc("PUT /v1/eval-suites/{suite_id}/test-cases/order", eh.ReorderCases)
	mux.HandleFunc("GET /v1/eval-suites/{suite_id}/test-cases/{case_id}", eh.GetCase)
	mux.HandleFunc("PUT /v1/eval-suites/{suite_id}/test-cases/{case_id}", eh.UpdateCase)
	mux.HandleFunc("DELETE /v1/eval-suites/{suite_id}/test-cases/{case_id}", eh.DeleteCase)
	mux.HandleFunc("POST /v1/eval-suites/{suite_id}/runs", eh.TriggerRun)
	mux.HandleFunc("GET /v1/eval-suites/{suite_id}/runs", eh.ListRuns)
	mux.HandleFunc("GET /v1/eval-runs/{eval_run_id}", eh.GetRun)
	mux.HandleFunc("GET /v1/eval-runs/{eval_run_id}/test-case-results/{result_id}", eh.GetTestCaseResult)
	mux.HandleFunc("POST /v1/eval-webhooks/{suite_id}", eh.WebhookTrigger)

	return cors(requestID(logRequests(mux)))
}

// newLLMFactory constructs an eval.LLMFactory backed by singleton OpenAI and Anthropic clients.
// Both clients are created once at startup and reused across all eval runs.
func newLLMFactory() eval.LLMFactory {
	openaiClient := openai.New()
	anthropicClient := anthropic.New()
	return func(provider string) (aiprovider.LLMClient, error) {
		switch provider {
		case "openai":
			return openaiClient, nil
		case "anthropic":
			return anthropicClient, nil
		default:
			return nil, fmt.Errorf("unknown LLM provider %q; supported: openai, anthropic", provider)
		}
	}
}
