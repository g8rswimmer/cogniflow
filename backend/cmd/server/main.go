package main

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/g8rswimmer/cogniflow/internal/api"
	"github.com/g8rswimmer/cogniflow/internal/crypto"
	httprequest "github.com/g8rswimmer/cogniflow/internal/node/builtin/http_request"
	"github.com/g8rswimmer/cogniflow/internal/node"
	mysqlstore "github.com/g8rswimmer/cogniflow/internal/store/mysql"
)

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		slog.Error("DB_DSN environment variable is required")
		os.Exit(1)
	}

	encKeyB64 := os.Getenv("COGNIFLOW_ENCRYPTION_KEY")
	if encKeyB64 == "" {
		slog.Error("COGNIFLOW_ENCRYPTION_KEY environment variable is required")
		os.Exit(1)
	}
	encKey, err := base64.StdEncoding.DecodeString(encKeyB64)
	if err != nil {
		slog.Error("COGNIFLOW_ENCRYPTION_KEY must be base64-encoded", "error", err)
		os.Exit(1)
	}
	cipher, err := crypto.NewCipher(encKey)
	if err != nil {
		slog.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	db, err := mysqlstore.Open(dsn)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database connected and migrations applied")

	registry := node.NewRegistry()
	registry.Register(httprequest.New())

	rawStore := mysqlstore.NewWorkflowStore(db)
	vault := crypto.NewConfigVault(rawStore, cipher, registry)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := api.NewRouter(db, vault, registry)

	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
