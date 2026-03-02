package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// EventRecord representa uma linha histórica no banco de dados
type EventRecord struct {
	ID        int
	Timestamp time.Time
	Type      string // "event" ou "alert"
	Action    string // ex: "start", "stop", "critical"
	Container string
	Message   string
}

// SQLiteStore encapsula o cliente do banco de dados relacional.
// Utiliza modernc.org/sqlite, que é 100% Go nativo e não depende de CGO.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore inicializa ou cria o banco de dados no arquivo especificado.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Cria/abre o banco de dados e adiciona pragmas de performance
	// loc=auto resolve conversões de timezone automaticamente
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: falha ao abrir sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("storage: falha de conexao com banco: %w", err)
	}

	// Criação do Schema Automático
	// A tabela engloba tanto eventos do docker run/stop (type=event)
	// quanto disparos de alertas de monitoramento (type=alert)
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
		return nil, fmt.Errorf("storage: falha ao inicializar tabelas: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// SaveEvent persiste um evento Docker no histórico local.
// Deve chamar assíncronamente para não bloquear a UI.
func (s *SQLiteStore) SaveEvent(ctx context.Context, action, container, message string) {
	go func() {
		query := `INSERT INTO events (timestamp, type, action, container, message) VALUES (?, ?, ?, ?, ?)`
		s.db.ExecContext(context.Background(), query, time.Now().UTC(), "event", action, container, message)
	}()
}

// SaveAlert persiste o disparo de um alerta de monitoramento ou segurança.
func (s *SQLiteStore) SaveAlert(ctx context.Context, severity, container, message string) {
	go func() {
		query := `INSERT INTO events (timestamp, type, action, container, message) VALUES (?, ?, ?, ?, ?)`
		s.db.ExecContext(context.Background(), query, time.Now().UTC(), "alert", severity, container, message)
	}()
}

// GetRecent recupera os últimos N eventos para histórico ou restore na UI.
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
		// Parse do timestamp gerado pelo CURRENT_TIMESTAMP
		if t, err := time.Parse("2006-01-02 15:04:05", tsStr); err == nil {
			r.Timestamp = t.Local()
		} else {
			r.Timestamp = time.Now() // fallback
		}

		results = append(results, r)
	}
	return results, nil
}

// Close garante o fechamento do banco e do WAL file.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
