package api

import (
	"net/http"

	"github.com/jmoiron/sqlx"
)

func NewRouter(db *sqlx.DB) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health", newHealthHandler(db))

	return mux
}
