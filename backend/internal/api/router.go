package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/g8rswimmer/cogniflow/internal/aiprovider"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/anthropic"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/openai"
	"github.com/g8rswimmer/cogniflow/internal/auth"
	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/eval"
	"github.com/g8rswimmer/cogniflow/internal/eval/grader_plugin"
	"github.com/g8rswimmer/cogniflow/internal/node"
	nodeplugin "github.com/g8rswimmer/cogniflow/internal/node/plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

// NewRouter wires all HTTP routes and returns the handler wrapped with
// CORS, request-ID, and access-log middleware.
// srvCtx is a server-lifetime context cancelled on shutdown.
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
	graderRegistry *grader_plugin.GraderRegistry,
	level *slog.LevelVar,
	jwtSecret []byte,
	jwtTTL time.Duration,
	frontendURL string,
) http.Handler {
	mux := newAuthMux(jwtSecret)

	// Config — always public.
	mux.public("GET /v1/config", newConfigHandler())

	// Infrastructure — always public.
	mux.public("GET /health", newHealthHandler(db))
	llh := &logLevelHandler{level: level}
	mux.public("GET /admin/log-level", http.HandlerFunc(llh.get))
	mux.public("PUT /admin/log-level", http.HandlerFunc(llh.set))

	// Auth endpoints — public (no JWT required).
	ah := &authHandler{store: st, jwtSecret: jwtSecret, jwtTTL: jwtTTL}
	mux.public("POST /v1/auth/login", http.HandlerFunc(ah.login))
	mux.public("GET /v1/auth/invite/{token}", http.HandlerFunc(ah.getInvite))
	mux.public("POST /v1/auth/accept-invite", http.HandlerFunc(ah.acceptInvite))

	// Webhook trigger — public but HMAC-validated inside the handler.
	mux.public("POST /v1/webhooks/{workflow_id}", http.HandlerFunc(tm.WebhookHandler()))

	// Authenticated: current user info.
	mux.authed("GET /v1/auth/me", http.HandlerFunc(ah.me))

	// Workflow routes — guarded by permission scopes.
	wh := &workflowHandler{store: st, registry: registry, triggers: tm}
	mux.perm("GET /v1/workflows", "workflow:read", http.HandlerFunc(wh.list))
	mux.perm("POST /v1/workflows", "workflow:write", http.HandlerFunc(wh.create))
	mux.perm("GET /v1/workflows/{id}", "workflow:read", http.HandlerFunc(wh.get))
	mux.perm("PUT /v1/workflows/{id}", "workflow:write", http.HandlerFunc(wh.update))
	mux.perm("DELETE /v1/workflows/{id}", "workflow:write", http.HandlerFunc(wh.delete))

	wvh := &workflowVersionHandler{store: st, triggers: tm}
	mux.perm("GET /v1/workflows/{id}/versions", "workflow:read", http.HandlerFunc(wvh.listVersions))
	mux.perm("POST /v1/workflows/{id}/versions/{version_number}/restore", "workflow:write", http.HandlerFunc(wvh.restoreVersion))
	mux.perm("GET /v1/workflows/{id}/versions/{version_number}", "workflow:read", http.HandlerFunc(wvh.getVersion))

	nth := &nodeTypeHandler{registry: registry}
	mux.perm("GET /v1/node-types", "workflow:read", http.HandlerFunc(nth.list))

	rh := &runHandler{store: st, dispatcher: dispatcher}
	mux.perm("POST /v1/workflows/{id}/runs", "workflow:run", http.HandlerFunc(rh.triggerRun))
	mux.perm("GET /v1/workflows/{id}/runs", "workflow:read", http.HandlerFunc(rh.listRuns))
	mux.perm("GET /v1/runs/{run_id}", "workflow:read", http.HandlerFunc(rh.getRun))

	wsh := &wsHandler{store: st, bus: bus}
	mux.perm("GET /v1/runs/{run_id}/events", "workflow:read", http.HandlerFunc(wsh.streamEvents))

	// Eval routes.
	vault := eval.NewGraderVault(cipher)
	evalBus := eval.NewEvalEventBus()
	runner := eval.NewEvalRunner(srvCtx, st, eng, vault, newLLMFactory(), graderRegistry, evalBus)

	evalSched := eval.NewEvalScheduler(srvCtx, runner)
	if cronSuites, err := st.ListEvalSuitesByCronTrigger(srvCtx); err != nil {
		slog.Warn("eval scheduler: could not load cron suites at startup", "error", err)
	} else {
		evalSched.LoadAll(cronSuites)
	}
	evalSched.Start()

	eh := eval.NewHandler(st, vault, registry, runner, evalSched, evalBus)

	mux.perm("GET /v1/workflows/{workflow_id}/eval-suites", "eval:read", http.HandlerFunc(eh.ListByWorkflow))
	mux.perm("POST /v1/workflows/{workflow_id}/eval-suites", "eval:write", http.HandlerFunc(eh.CreateSuite))
	mux.perm("GET /v1/eval-suites/{suite_id}", "eval:read", http.HandlerFunc(eh.GetSuite))
	mux.perm("PUT /v1/eval-suites/{suite_id}", "eval:write", http.HandlerFunc(eh.UpdateSuite))
	mux.perm("DELETE /v1/eval-suites/{suite_id}", "eval:write", http.HandlerFunc(eh.DeleteSuite))
	mux.perm("GET /v1/eval-suites/{suite_id}/test-cases", "eval:read", http.HandlerFunc(eh.ListCases))
	mux.perm("POST /v1/eval-suites/{suite_id}/test-cases", "eval:write", http.HandlerFunc(eh.CreateCase))
	mux.perm("PUT /v1/eval-suites/{suite_id}/test-cases/order", "eval:write", http.HandlerFunc(eh.ReorderCases))
	mux.perm("POST /v1/eval-suites/{suite_id}/test-cases/import", "eval:write", http.HandlerFunc(eh.ImportTestCases))
	mux.perm("GET /v1/eval-suites/{suite_id}/test-cases/{case_id}", "eval:read", http.HandlerFunc(eh.GetCase))
	mux.perm("PUT /v1/eval-suites/{suite_id}/test-cases/{case_id}", "eval:write", http.HandlerFunc(eh.UpdateCase))
	mux.perm("DELETE /v1/eval-suites/{suite_id}/test-cases/{case_id}", "eval:write", http.HandlerFunc(eh.DeleteCase))
	mux.perm("POST /v1/eval-suites/{suite_id}/runs", "eval:run", http.HandlerFunc(eh.TriggerRun))
	mux.perm("GET /v1/eval-suites/{suite_id}/runs", "eval:read", http.HandlerFunc(eh.ListRuns))
	mux.perm("GET /v1/eval-runs/{eval_run_id}", "eval:read", http.HandlerFunc(eh.GetRun))
	mux.perm("GET /v1/eval-runs/{eval_run_id}/events", "eval:read", http.HandlerFunc(eh.StreamEvalRunEvents))
	mux.perm("GET /v1/eval-runs/{eval_run_id}/compare", "eval:read", http.HandlerFunc(eh.CompareRuns))
	mux.perm("GET /v1/eval-runs/{eval_run_id}/test-case-results/{result_id}", "eval:read", http.HandlerFunc(eh.GetTestCaseResult))
	// Eval webhook is public but validates HMAC signature inside the handler.
	mux.public("POST /v1/eval-webhooks/{suite_id}", http.HandlerFunc(eh.WebhookTrigger))

	// Org-admin routes (org_admin or system_admin).
	uah := &userAdminHandler{store: st, jwtSecret: jwtSecret, jwtTTL: jwtTTL, frontendURL: frontendURL}
	esH := &orgEmailSettingsHandler{store: st}
	mux.role("GET /v1/org/users", http.HandlerFunc(uah.listOrgUsers), "org_admin", "system_admin")
	mux.role("POST /v1/org/users/invite", http.HandlerFunc(uah.inviteUser), "org_admin", "system_admin")
	mux.role("PUT /v1/org/users/{id}/role", http.HandlerFunc(uah.updateOrgUserRole), "org_admin", "system_admin")
	mux.role("PUT /v1/org/users/{id}/permissions", http.HandlerFunc(uah.updateOrgUserPermissions), "org_admin", "system_admin")
	mux.role("DELETE /v1/org/users/{id}", http.HandlerFunc(uah.removeOrgUser), "org_admin", "system_admin")
	mux.role("GET /v1/org/email-settings", http.HandlerFunc(esH.getOrgEmailSettings), "org_admin", "system_admin")
	mux.role("PUT /v1/org/email-settings", http.HandlerFunc(esH.upsertOrgEmailSettings), "org_admin", "system_admin")
	mux.role("DELETE /v1/org/email-settings", http.HandlerFunc(esH.deleteOrgEmailSettings), "org_admin", "system_admin")

	// System-admin routes.
	mux.role("PUT /v1/admin/orgs/{org_id}/email-settings", http.HandlerFunc(esH.upsertOrgEmailSettingsAdmin), "system_admin")
	pah := &pluginAdminHandler{store: st, registry: registry, registerFn: nodeplugin.RegisterOne}
	mux.role("GET /v1/admin/orgs", http.HandlerFunc(uah.listOrgs), "system_admin")
	mux.role("POST /v1/admin/orgs", http.HandlerFunc(uah.createOrg), "system_admin")
	mux.role("DELETE /v1/admin/orgs/{id}", http.HandlerFunc(uah.deleteOrg), "system_admin")
	mux.role("GET /v1/admin/users", http.HandlerFunc(uah.listAllUsers), "system_admin")
	mux.role("DELETE /v1/admin/users/{id}", http.HandlerFunc(uah.deleteUser), "system_admin")
	mux.role("PUT /v1/admin/users/{id}/role", http.HandlerFunc(uah.updateOrgUserRole), "system_admin")
	mux.role("PUT /v1/admin/users/{id}/permissions", http.HandlerFunc(uah.updateOrgUserPermissions), "system_admin")
	mux.role("GET /v1/admin/plugins", http.HandlerFunc(pah.list), "system_admin")
	mux.role("POST /v1/admin/plugins", http.HandlerFunc(pah.register), "system_admin")
	mux.role("PUT /v1/admin/plugins/{type_id}", http.HandlerFunc(pah.update), "system_admin")
	mux.role("DELETE /v1/admin/plugins/{type_id}", http.HandlerFunc(pah.deregister), "system_admin")

	gpah := &graderPluginAdminHandler{store: st, registry: graderRegistry, registerFn: grader_plugin.RegisterOne}
	mux.role("GET /v1/admin/grader-plugins", http.HandlerFunc(gpah.list), "system_admin")
	mux.role("POST /v1/admin/grader-plugins", http.HandlerFunc(gpah.register), "system_admin")
	mux.role("PUT /v1/admin/grader-plugins/{type_id}", http.HandlerFunc(gpah.update), "system_admin")
	mux.role("DELETE /v1/admin/grader-plugins/{type_id}", http.HandlerFunc(gpah.deregister), "system_admin")

	return cors(requestID(logRequests(mux.mux)))
}

// authMux wraps http.ServeMux with helpers that automatically apply
// Authenticate + RequirePermission / RequireRole middleware per route.
type authMux struct {
	mux       *http.ServeMux
	jwtSecret []byte
}

func newAuthMux(jwtSecret []byte) *authMux {
	return &authMux{mux: http.NewServeMux(), jwtSecret: jwtSecret}
}

// public registers a route with no authentication.
func (m *authMux) public(pattern string, h http.Handler) {
	m.mux.Handle(pattern, h)
}

// authed requires a valid JWT but no specific role or permission.
func (m *authMux) authed(pattern string, h http.Handler) {
	m.mux.Handle(pattern, auth.Authenticate(m.jwtSecret)(h))
}

// perm requires a valid JWT and the named permission scope.
// The org_id from the JWT is also injected into ctx via store.WithOrgID so that
// MySQL store methods can scope queries without extra parameters.
func (m *authMux) perm(pattern string, scope string, h http.Handler) {
	wrapped := auth.Authenticate(m.jwtSecret)(
		injectOrgID(
			auth.RequirePermission(scope)(h),
		),
	)
	m.mux.Handle(pattern, wrapped)
}

// role requires a valid JWT and one of the named roles.
func (m *authMux) role(pattern string, h http.Handler, roles ...string) {
	wrapped := auth.Authenticate(m.jwtSecret)(
		injectOrgID(
			auth.RequireRole(roles...)(h),
		),
	)
	m.mux.Handle(pattern, wrapped)
}

// injectOrgID is middleware that reads the org_id from JWT claims and stores
// it via store.WithOrgID so MySQL store methods scope queries automatically.
func injectOrgID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := auth.OrgIDFrom(r.Context())
		if orgID != "" {
			r = r.WithContext(store.WithOrgID(r.Context(), orgID))
		}
		next.ServeHTTP(w, r)
	})
}

// newLLMFactory constructs an eval.LLMFactory backed by singleton OpenAI and Anthropic clients.
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
