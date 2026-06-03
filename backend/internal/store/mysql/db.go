package mysql

import (
	"embed"
	"errors"
	"fmt"

	drvmysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens a MySQL connection and runs pending schema migrations.
func Open(dsn string) (*sqlx.DB, error) {
	// Ensure clientFoundRows=true so that UPDATE statements report matched rows
	// rather than only rows whose values changed. Without this, updating a row
	// with identical values returns RowsAffected=0, which the store incorrectly
	// treats as ErrNotFound on an idempotent PUT.
	cfg, err := drvmysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: parse DSN: %w", err)
	}
	cfg.ClientFoundRows = true
	dsn = cfg.FormatDSN()

	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func runMigrations(db *sqlx.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	driver, err := migratemysql.WithInstance(db.DB, &migratemysql.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
