// Package storage manages the local SQLite database for event and alert history.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// EventRecord represents a historical row in the database.
type EventRecord struct {
	ID        int
	Timestamp time.Time
	Type      string // "event" or "alert"
	Action    string // e.g. "start", "stop", "critical"
	Container string
	Message   string
}

// SQLiteStore wraps the relational database client with an asynchronous write queue.
type SQLiteStore struct {
	db        *sql.DB
	saveCh    chan saveOptions
	done      chan struct{}
	closeOnce sync.Once
}

// NewSQLiteStore initializes or creates the database at the specified file path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	var dsn string
	if dbPath != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	} else {
		// modernc.org/sqlite requires a unique memory URI or a shared connection
		// to allow concurrent access to the same in-memory database.
		dsn = fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to open sqlite: %w", err)
	}

	// Single writer connection avoids 'database is locked' errors in SQLite
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: failed to connect to database: %w", err)
	}

	query := `
	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT,
		action TEXT,
		container TEXT,
		message TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	`
	_, err = db.Exec(query)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: failed to initialize tables: %w", err)
	}

	store := &SQLiteStore{
		db:     db,
		saveCh: make(chan saveOptions, 500),
		done:   make(chan struct{}),
	}
	go store.writeWorker()

	return store, nil
}

// saveOptions holds the parameters for the save function.
type saveOptions struct {
	RecordType       string
	ActionOrSeverity string
	Container        string
	Message          string
}

// writeWorker sequentially persists queued records into SQLite, eliminating lock contention.
func (s *SQLiteStore) writeWorker() {
	query := `INSERT INTO events (timestamp, type, action, container, message) VALUES (?, ?, ?, ?, ?)`
	for opts := range s.saveCh {
		_, _ = s.db.Exec(
			query,
			time.Now().UTC(),
			opts.RecordType,
			opts.ActionOrSeverity,
			opts.Container,
			opts.Message,
		)
	}
	close(s.done)
}

// save queues an event or alert to be saved asynchronously by the write worker.
func (s *SQLiteStore) save(ctx context.Context, opts saveOptions) {
	select {
	case s.saveCh <- opts:
	default:
		// Queue saturated: avoid blocking caller
	}
}

// SaveEvent persists a Docker event (start, stop, etc.) in the local history.
func (s *SQLiteStore) SaveEvent(ctx context.Context, action, container, message string) {
	s.save(ctx, saveOptions{
		RecordType:       "event",
		ActionOrSeverity: action,
		Container:        container,
		Message:          message,
	})
}

// GetRecent retrieves the last N events for the TUI history.
func (s *SQLiteStore) GetRecent(limit int) ([]EventRecord, error) {
	query := `SELECT id, timestamp, type, action, container, message FROM events ORDER BY timestamp DESC LIMIT ?`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EventRecord
	for rows.Next() {
		var r EventRecord
		var tsStr string
		if err := rows.Scan(&r.ID, &tsStr, &r.Type, &r.Action, &r.Container, &r.Message); err != nil {
			return nil, err
		}

		if t, err := time.Parse("2006-01-02 15:04:05", tsStr); err == nil {
			r.Timestamp = t.Local()
		} else {
			r.Timestamp = time.Now()
		}

		results = append(results, r)
	}
	return results, nil
}

// Close gracefully closes the save queue, waits for pending writes to flush, and closes the database.
func (s *SQLiteStore) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.saveCh)
		<-s.done
		err = s.db.Close()
	})
	return err
}