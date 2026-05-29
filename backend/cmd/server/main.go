package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/g8rswimmer/cogniflow/internal/api"
	"github.com/g8rswimmer/cogniflow/internal/store/mysql"
)

func main() {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		slog.Error("DB_DSN environment variable is required")
		os.Exit(1)
	}

	db, err := mysql.Open(dsn)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("database connected and migrations applied")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := api.NewRouter(db)

	addr := fmt.Sprintf(":%s", port)
	slog.Info("server starting", "addr", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
