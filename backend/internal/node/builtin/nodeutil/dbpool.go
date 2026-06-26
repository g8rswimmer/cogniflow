package nodeutil

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// DBPool maintains a per-(driver, DSN) connection pool shared across Execute
// calls for the same node handler. Using a shared pool avoids opening a new
// *sql.DB on every workflow run when the driver/DSN pair is the same.
//
// When pools is nil (Handler constructed directly in tests without New()),
// Get opens a fresh connection per call and signals the caller to close it.
type DBPool struct {
	openDB  func(driver, dsn string) (*sql.DB, error)
	poolsMu sync.Mutex
	pools   map[string]*sql.DB // keyed by "driver\x00dsn"
}

// NewDBPool returns a production-ready DBPool backed by sql.Open.
func NewDBPool() *DBPool {
	return &DBPool{openDB: sql.Open, pools: make(map[string]*sql.DB)}
}

// NewTestPool returns a DBPool in test mode: each Get call opens a fresh
// connection via the provided openDB function and signals the caller to close
// it. pools is nil so no caching occurs between calls.
func NewTestPool(openDB func(driver, dsn string) (*sql.DB, error)) *DBPool {
	return &DBPool{openDB: openDB}
}

// Get returns a pooled *sql.DB for the given driver/DSN pair, creating it on
// first use. closeWhenDone is true only when pools is nil (test mode), which
// means the caller must close the returned *sql.DB after use.
func (p *DBPool) Get(driver, dsn string) (db *sql.DB, closeWhenDone bool, err error) {
	if p.pools == nil {
		db, err = p.openDB(driver, dsn)
		return db, true, err
	}
	key := driver + "\x00" + dsn
	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()
	if db, ok := p.pools[key]; ok {
		return db, false, nil
	}
	db, err = p.openDB(driver, dsn)
	if err != nil {
		return nil, false, fmt.Errorf("open db (%s): %w", driver, err)
	}
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	p.pools[key] = db
	return db, false, nil
}
