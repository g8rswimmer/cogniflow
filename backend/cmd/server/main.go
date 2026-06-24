package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/api"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/anthropic"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/ollama"
	"github.com/g8rswimmer/cogniflow/internal/aiprovider/openai"
	"github.com/g8rswimmer/cogniflow/internal/auth"
	"github.com/g8rswimmer/cogniflow/internal/crypto"
	"github.com/g8rswimmer/cogniflow/internal/engine"
	"github.com/g8rswimmer/cogniflow/internal/eval/grader_plugin"
	"github.com/g8rswimmer/cogniflow/internal/node"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/conditional"
	datatransform "github.com/g8rswimmer/cogniflow/internal/node/builtin/data_transform"
	loopcontroller "github.com/g8rswimmer/cogniflow/internal/node/builtin/loop_controller"
	dbquery "github.com/g8rswimmer/cogniflow/internal/node/builtin/db_query"
	dbwrite "github.com/g8rswimmer/cogniflow/internal/node/builtin/db_write"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/embedding"
	httprequest "github.com/g8rswimmer/cogniflow/internal/node/builtin/http_request"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/llm"
	"github.com/g8rswimmer/cogniflow/internal/node/builtin/merge"
	ragingest "github.com/g8rswimmer/cogniflow/internal/node/builtin/rag_ingest"
	ragretrieve "github.com/g8rswimmer/cogniflow/internal/node/builtin/rag_retrieve"
	nodeplugin "github.com/g8rswimmer/cogniflow/internal/node/plugin"
	"github.com/g8rswimmer/cogniflow/internal/store"
	mysqlstore "github.com/g8rswimmer/cogniflow/internal/store/mysql"
	"github.com/g8rswimmer/cogniflow/internal/trigger"
)

func main() {
	var logLevel slog.LevelVar
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "warn":
		logLevel.Set(slog.LevelWarn)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &logLevel})))
	slog.Info("log level set", "level", logLevel.Level().String())

	if err := run(&logLevel); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// run contains all server startup and lifecycle logic. Using a separate
// function ensures that deferred cleanup (registry.Shutdown, db.Close, etc.)
// runs on both normal exit and startup failures, since os.Exit skips defers.
func run(logLevel *slog.LevelVar) error {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		return fmt.Errorf("DB_DSN environment variable is required")
	}

	encKeyB64 := os.Getenv("COGNIFLOW_ENCRYPTION_KEY")
	if encKeyB64 == "" {
		return fmt.Errorf("COGNIFLOW_ENCRYPTION_KEY environment variable is required")
	}
	encKey, err := base64.StdEncoding.DecodeString(encKeyB64)
	if err != nil {
		return fmt.Errorf("COGNIFLOW_ENCRYPTION_KEY must be base64-encoded: %w", err)
	}
	cipher, err := crypto.NewCipher(encKey)
	if err != nil {
		return fmt.Errorf("invalid encryption key: %w", err)
	}

	jwtSecretStr := os.Getenv("JWT_SECRET")
	if len(jwtSecretStr) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	jwtSecret := []byte(jwtSecretStr)

	jwtTTL := 24 * time.Hour
	if ttlStr := os.Getenv("JWT_TTL"); ttlStr != "" {
		parsed, err := time.ParseDuration(ttlStr)
		if err != nil {
			return fmt.Errorf("invalid JWT_TTL %q: %w", ttlStr, err)
		}
		jwtTTL = parsed
	}

	db, err := mysqlstore.Open(dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	slog.Info("database connected and migrations applied")

	openaiClient := openai.New()
	anthropicClient := anthropic.New()

	rawStore := mysqlstore.NewWorkflowStore(db)

	registry := node.NewRegistry()
	defer registry.Shutdown()
	registry.Register(httprequest.New())
	registry.Register(llm.NewOpenAI(openaiClient))
	registry.Register(llm.NewAnthropic(anthropicClient))
	registry.Register(embedding.New(openaiClient))
	registry.Register(conditional.New())
	registry.Register(loopcontroller.New())
	registry.Register(datatransform.New())
	registry.Register(dbquery.New())
	registry.Register(dbwrite.New())
	registry.Register(merge.New())

	// Re-establish connections to plugins registered via the admin API (persisted
	// in DB). Done before PLUGIN_ADDRESSES so DB-registered plugins take
	// precedence and env-var duplicates are skipped with a warning.
	nodeplugin.LoadFromStore(context.Background(), rawStore, registry)

	// Register out-of-process gRPC plugins before built-in AI nodes so that
	// PLUGIN_ADDRESSES nodes appear in the palette alongside built-ins.
	if pluginAddrs := os.Getenv("PLUGIN_ADDRESSES"); pluginAddrs != "" {
		pluginCtx, pluginCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer pluginCancel()
		nodeplugin.Register(pluginCtx, pluginAddrs, registry)
	}

	vault := crypto.NewConfigVault(rawStore, cipher, registry)

	if err := bootstrapAdmin(context.Background(), vault); err != nil {
		return fmt.Errorf("admin bootstrap: %w", err)
	}

	if ollamaURL := os.Getenv("OLLAMA_BASE_URL"); ollamaURL != "" {
		slog.Info("using Ollama for RAG embeddings", "base_url", ollamaURL)
		ollamaClient := ollama.New(ollamaURL)
		registry.Register(ragingest.New(ollamaClient, vault))
		registry.Register(ragretrieve.New(ollamaClient, vault))
	} else {
		registry.Register(ragingest.New(openaiClient, vault))
		registry.Register(ragretrieve.New(openaiClient, vault))
	}

	bus := engine.NewEventBus()
	wfEngine := engine.NewWorkflowEngine(vault, registry, bus)

	graderRegistry := grader_plugin.NewGraderRegistry()
	defer graderRegistry.Shutdown()
	grader_plugin.LoadFromStore(context.Background(), rawStore, graderRegistry)

	triggerMgr := trigger.NewManager(vault, wfEngine)
	loadCtx, loadCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer loadCancel()
	if err := triggerMgr.LoadAll(loadCtx); err != nil {
		return fmt.Errorf("failed to load trigger configs: %w", err)
	}
	triggerMgr.Start()
	defer triggerMgr.Stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// srvCtx is cancelled when a shutdown signal is received, allowing
	// background goroutines (e.g. EvalRunner) to stop cleanly.
	srvCtx, srvCancel := context.WithCancel(context.Background())

	router := api.NewRouter(srvCtx, db, vault, registry, wfEngine, bus, wfEngine, cipher, triggerMgr, graderRegistry, logLevel, jwtSecret, jwtTTL)

	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		slog.Info("shutdown signal received")
		srvCancel() // stop background goroutines
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// bootstrapAdmin creates a "Default" org and system_admin user on first startup
// when BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD are set and no users
// exist yet. Idempotent: does nothing if users already exist.
func bootstrapAdmin(ctx context.Context, st store.Store) error {
	email := os.Getenv("BOOTSTRAP_ADMIN_EMAIL")
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return nil
	}

	users, err := st.ListUsers(ctx, "")
	if err != nil {
		return fmt.Errorf("check existing users: %w", err)
	}
	if len(users) > 0 {
		return nil // already bootstrapped
	}

	org, err := st.CreateOrganization(ctx, store.Organization{Name: "Default"})
	if err != nil {
		return fmt.Errorf("create default org: %w", err)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if _, err := st.CreateUser(ctx, store.User{
		OrgID:        org.ID,
		Email:        email,
		PasswordHash: hash,
		Role:         "system_admin",
		Permissions:  store.DefaultPermissions,
	}); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	slog.Info("bootstrapped system_admin", "email", email, "org_id", org.ID)
	return nil
}
