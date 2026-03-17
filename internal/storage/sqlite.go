package storage

import (
	"context"
	"database/sql"
	"fmt"
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

// SQLiteStore wraps the relational database client.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore initializes or creates the database at the specified file path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dsn := dbPath
	if dbPath != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	} else {
		// modernc.org/sqlite requires a unique memory URI or a shared connection
		// When we use :memory: each new connection gets its own private database,
		// so GetRecent() doesn't see what save() wrote in another goroutine.
		// A common way to share the memory db across multiple connections is:
		dsn = fmt.Sprintf("file:memdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
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
		return nil, fmt.Errorf("storage: failed to initialize tables: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// save persiste os dados de forma assíncrona.
func (s *SQLiteStore) save(ctx context.Context, recordType, actionOrSeverity, container, message string) {
	go func() {
		query := `INSERT INTO events (timestamp, type, action, container, message) VALUES (?, ?, ?, ?, ?)`
		
		// RESOLUÇÃO:
		// Usamos context.WithoutCancel(ctx) para garantir que o registro seja salvo 
		// mesmo se o contexto original (ex: uma requisição HTTP) for cancelado.
		// E usamos os parâmetros dinâmicos (recordType, actionOrSeverity) da main.
		s.db.ExecContext(
			context.WithoutCancel(ctx), 
			query, 
			time.Now().UTC(), 
			recordType, 
			actionOrSeverity, 
			container, 
			message,
		)
	}()
}

// SaveEvent persists a Docker event in the local history.
func (s *SQLiteStore) SaveEvent(ctx context.Context, action, container, message string) {
	s.save(ctx, "event", action, container, message)
}

// SaveAlert persists the firing of a monitoring or security alert.
func (s *SQLiteStore) SaveAlert(ctx context.Context, severity, container, message string) {
	s.save(ctx, "alert", severity, container, message)
}

// GetRecent retrieves the last N events for history or UI restoration.
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

// Close ensures proper shutdown of the database and WAL file.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}