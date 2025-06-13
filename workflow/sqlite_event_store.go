package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteExecutionEventStore implements ExecutionEventStore using SQLite database
type SQLiteExecutionEventStore struct {
	db      *sql.DB
	dbPath  string
	mutex   sync.RWMutex
	options SQLiteStoreOptions
}

// SQLiteStoreOptions configures the SQLite event store
type SQLiteStoreOptions struct {
	BatchSize         int           // Number of events to batch in a single transaction
	QueryTimeout      time.Duration // Timeout for database queries
	PragmaJournalMode string        // WAL mode for better concurrent performance
	PragmaSyncMode    string        // Synchronization mode
	MaxConnections    int           // Maximum number of connections in pool
}

// DefaultSQLiteStoreOptions returns sensible defaults
func DefaultSQLiteStoreOptions() SQLiteStoreOptions {
	return SQLiteStoreOptions{
		BatchSize:         100,
		QueryTimeout:      30 * time.Second,
		PragmaJournalMode: "WAL",
		PragmaSyncMode:    "NORMAL",
		MaxConnections:    10,
	}
}

// NewSQLiteExecutionEventStore creates a new SQLite-based event store
func NewSQLiteExecutionEventStore(dbPath string, options SQLiteStoreOptions) (*SQLiteExecutionEventStore, error) {
	if options.BatchSize == 0 {
		options = DefaultSQLiteStoreOptions()
	}

	store := &SQLiteExecutionEventStore{
		dbPath:  dbPath,
		options: options,
	}

	if err := store.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}

	return store, nil
}

// initialize sets up the database connection and schema
func (s *SQLiteExecutionEventStore) initialize() error {
	var err error

	// Open database connection with pragmas for performance
	dsn := fmt.Sprintf("%s?_journal_mode=%s&_sync=%s&_foreign_keys=1&_timeout=5000",
		s.dbPath, s.options.PragmaJournalMode, s.options.PragmaSyncMode)

	s.db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	s.db.SetMaxOpenConns(s.options.MaxConnections)
	s.db.SetMaxIdleConns(s.options.MaxConnections / 2)
	s.db.SetConnMaxLifetime(time.Hour)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), s.options.QueryTimeout)
	defer cancel()

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create schema
	if err := s.createSchema(); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// createSchema creates the necessary tables and indexes
func (s *SQLiteExecutionEventStore) createSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.options.QueryTimeout)
	defer cancel()

	// Create execution_events table
	eventsSchema := `
	CREATE TABLE IF NOT EXISTS execution_events (
		id TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL,
		path_id TEXT,
		sequence INTEGER NOT NULL,
		timestamp DATETIME NOT NULL,
		event_type TEXT NOT NULL,
		step_name TEXT,
		data JSON,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(execution_id, sequence)
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_execution_events_execution_id ON execution_events(execution_id);
	CREATE INDEX IF NOT EXISTS idx_execution_events_sequence ON execution_events(execution_id, sequence);
	CREATE INDEX IF NOT EXISTS idx_execution_events_type ON execution_events(event_type);
	CREATE INDEX IF NOT EXISTS idx_execution_events_timestamp ON execution_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_execution_events_path ON execution_events(path_id) WHERE path_id IS NOT NULL;
	`

	if _, err := s.db.ExecContext(ctx, eventsSchema); err != nil {
		return fmt.Errorf("failed to create events table: %w", err)
	}

	// Create execution_snapshots table
	snapshotsSchema := `
	CREATE TABLE IF NOT EXISTS execution_snapshots (
		execution_id TEXT PRIMARY KEY,
		workflow_name TEXT NOT NULL,
		workflow_hash TEXT NOT NULL,
		inputs_hash TEXT NOT NULL,
		status TEXT NOT NULL,
		start_time DATETIME,
		end_time DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		last_event_seq INTEGER NOT NULL,
		workflow_data BLOB,
		inputs JSON,
		outputs JSON,
		error TEXT
	);

	-- Indexes for querying
	CREATE INDEX IF NOT EXISTS idx_execution_snapshots_workflow ON execution_snapshots(workflow_name);
	CREATE INDEX IF NOT EXISTS idx_execution_snapshots_status ON execution_snapshots(status);
	CREATE INDEX IF NOT EXISTS idx_execution_snapshots_created ON execution_snapshots(created_at);
	CREATE INDEX IF NOT EXISTS idx_execution_snapshots_updated ON execution_snapshots(updated_at);
	`

	if _, err := s.db.ExecContext(ctx, snapshotsSchema); err != nil {
		return fmt.Errorf("failed to create snapshots table: %w", err)
	}

	return nil
}

// AppendEvents adds events to the store in a batch transaction
func (s *SQLiteExecutionEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	if len(events) == 0 {
		return nil
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Start transaction for batch insert
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare insert statement
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO execution_events 
		(id, execution_id, path_id, sequence, timestamp, event_type, step_name, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert events in batches
	for i, event := range events {
		// Validate event
		if err := event.Validate(); err != nil {
			return fmt.Errorf("invalid event at index %d: %w", i, err)
		}

		// Serialize event data
		var dataJSON []byte
		if event.Data != nil {
			dataJSON, err = json.Marshal(event.Data)
			if err != nil {
				return fmt.Errorf("failed to marshal event data at index %d: %w", i, err)
			}
		}

		// Execute insert
		_, err := stmt.ExecContext(ctx,
			event.ID,
			event.ExecutionID,
			nullableString(event.PathID),
			event.Sequence,
			event.Timestamp,
			event.EventType,
			nullableString(event.StepName),
			nullableBytes(dataJSON),
		)
		if err != nil {
			return fmt.Errorf("failed to insert event at index %d: %w", i, err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetEvents retrieves events for an execution starting from a sequence number
func (s *SQLiteExecutionEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	query := `
		SELECT id, execution_id, path_id, sequence, timestamp, event_type, step_name, data
		FROM execution_events
		WHERE execution_id = ? AND sequence >= ?
		ORDER BY sequence ASC
	`

	rows, err := s.db.QueryContext(ctx, query, executionID, fromSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*ExecutionEvent
	for rows.Next() {
		event, err := s.scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return events, nil
}

// GetEventHistory retrieves all events for an execution
func (s *SQLiteExecutionEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return s.GetEvents(ctx, executionID, 0)
}

// SaveSnapshot stores an execution snapshot with upsert behavior
func (s *SQLiteExecutionEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Serialize JSON fields
	inputsJSON, err := json.Marshal(snapshot.Inputs)
	if err != nil {
		return fmt.Errorf("failed to marshal inputs: %w", err)
	}

	outputsJSON, err := json.Marshal(snapshot.Outputs)
	if err != nil {
		return fmt.Errorf("failed to marshal outputs: %w", err)
	}

	// Upsert query
	query := `
		INSERT INTO execution_snapshots 
		(execution_id, workflow_name, workflow_hash, inputs_hash, status, 
		 start_time, end_time, created_at, updated_at, last_event_seq,
		 workflow_data, inputs, outputs, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(execution_id) DO UPDATE SET
			workflow_name = excluded.workflow_name,
			workflow_hash = excluded.workflow_hash,
			inputs_hash = excluded.inputs_hash,
			status = excluded.status,
			start_time = excluded.start_time,
			end_time = excluded.end_time,
			updated_at = excluded.updated_at,
			last_event_seq = excluded.last_event_seq,
			workflow_data = excluded.workflow_data,
			inputs = excluded.inputs,
			outputs = excluded.outputs,
			error = excluded.error
	`

	now := time.Now()
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	updatedAt := snapshot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err = s.db.ExecContext(ctx, query,
		snapshot.ID,
		snapshot.WorkflowName,
		snapshot.WorkflowHash,
		snapshot.InputsHash,
		snapshot.Status,
		nullableTime(snapshot.StartTime),
		nullableTime(snapshot.EndTime),
		createdAt,
		updatedAt,
		snapshot.LastEventSeq,
		nullableBytes(snapshot.WorkflowData),
		inputsJSON,
		outputsJSON,
		nullableString(snapshot.Error),
	)

	if err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	return nil
}

// GetSnapshot retrieves an execution snapshot
func (s *SQLiteExecutionEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	query := `
		SELECT execution_id, workflow_name, workflow_hash, inputs_hash, status,
			   start_time, end_time, created_at, updated_at, last_event_seq,
			   workflow_data, inputs, outputs, error
		FROM execution_snapshots
		WHERE execution_id = ?
	`

	row := s.db.QueryRowContext(ctx, query, executionID)
	return s.scanSnapshotRow(row)
}

// ListExecutions retrieves executions based on filter criteria
func (s *SQLiteExecutionEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Validate filter
	if err := filter.Validate(); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	// Build query with conditions
	query := `
		SELECT execution_id, workflow_name, workflow_hash, inputs_hash, status,
			   start_time, end_time, created_at, updated_at, last_event_seq,
			   workflow_data, inputs, outputs, error
		FROM execution_snapshots
	`

	var conditions []string
	var args []interface{}

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.WorkflowName != "" {
		conditions = append(conditions, "workflow_name = ?")
		args = append(args, filter.WorkflowName)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query executions: %w", err)
	}
	defer rows.Close()

	var snapshots []*ExecutionSnapshot
	for rows.Next() {
		snapshot, err := s.scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return snapshots, nil
}

// DeleteExecution removes an execution and its events
func (s *SQLiteExecutionEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete events first
	_, err = tx.ExecContext(ctx, "DELETE FROM execution_events WHERE execution_id = ?", executionID)
	if err != nil {
		return fmt.Errorf("failed to delete events: %w", err)
	}
	// Delete snapshot
	_, err = tx.ExecContext(ctx, "DELETE FROM execution_snapshots WHERE execution_id = ?", executionID)
	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit deletion: %w", err)
	}
	return nil
}

// CleanupCompletedExecutions removes old completed/failed executions
func (s *SQLiteExecutionEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find executions to delete
	rows, err := tx.QueryContext(ctx, `
		SELECT execution_id FROM execution_snapshots 
		WHERE status IN ('completed', 'failed', 'canceled') 
		AND updated_at < ?
	`, olderThan)
	if err != nil {
		return fmt.Errorf("failed to query old executions: %w", err)
	}
	defer rows.Close()

	var executionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan execution ID: %w", err)
		}
		executionIDs = append(executionIDs, id)
	}

	// Delete events and snapshots for each execution
	deleteEventStmt, err := tx.PrepareContext(ctx, "DELETE FROM execution_events WHERE execution_id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare event deletion: %w", err)
	}
	defer deleteEventStmt.Close()

	deleteSnapshotStmt, err := tx.PrepareContext(ctx, "DELETE FROM execution_snapshots WHERE execution_id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare snapshot deletion: %w", err)
	}
	defer deleteSnapshotStmt.Close()

	for _, executionID := range executionIDs {
		if _, err := deleteEventStmt.ExecContext(ctx, executionID); err != nil {
			return fmt.Errorf("failed to delete events for %s: %w", executionID, err)
		}
		if _, err := deleteSnapshotStmt.ExecContext(ctx, executionID); err != nil {
			return fmt.Errorf("failed to delete snapshot for %s: %w", executionID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cleanup: %w", err)
	}
	return nil
}

// Close closes the database connection
func (s *SQLiteExecutionEventStore) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Helper functions for scanning

func (s *SQLiteExecutionEventStore) scanEvent(rows *sql.Rows) (*ExecutionEvent, error) {
	event := &ExecutionEvent{}
	var pathID, stepName sql.NullString
	var dataJSON sql.NullString

	err := rows.Scan(
		&event.ID,
		&event.ExecutionID,
		&pathID,
		&event.Sequence,
		&event.Timestamp,
		&event.EventType,
		&stepName,
		&dataJSON,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if pathID.Valid {
		event.PathID = pathID.String
	}
	if stepName.Valid {
		event.StepName = stepName.String
	}
	if dataJSON.Valid && dataJSON.String != "" {
		if err := json.Unmarshal([]byte(dataJSON.String), &event.Data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
		}
	}

	return event, nil
}

func (s *SQLiteExecutionEventStore) scanSnapshot(rows *sql.Rows) (*ExecutionSnapshot, error) {
	snapshot := &ExecutionSnapshot{}
	var startTime, endTime sql.NullTime
	var workflowData sql.NullString
	var inputsJSON, outputsJSON []byte
	var errorMsg sql.NullString

	err := rows.Scan(
		&snapshot.ID,
		&snapshot.WorkflowName,
		&snapshot.WorkflowHash,
		&snapshot.InputsHash,
		&snapshot.Status,
		&startTime,
		&endTime,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
		&snapshot.LastEventSeq,
		&workflowData,
		&inputsJSON,
		&outputsJSON,
		&errorMsg,
	)
	if err != nil {
		return nil, err
	}

	// Convert nullable fields
	if startTime.Valid {
		snapshot.StartTime = startTime.Time
	}
	if endTime.Valid {
		snapshot.EndTime = endTime.Time
	}
	if workflowData.Valid {
		snapshot.WorkflowData = []byte(workflowData.String)
	}
	if errorMsg.Valid {
		snapshot.Error = errorMsg.String
	}

	// Deserialize JSON fields
	if err := json.Unmarshal(inputsJSON, &snapshot.Inputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
	}
	if err := json.Unmarshal(outputsJSON, &snapshot.Outputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal outputs: %w", err)
	}

	return snapshot, nil
}

func (s *SQLiteExecutionEventStore) scanSnapshotRow(row *sql.Row) (*ExecutionSnapshot, error) {
	snapshot := &ExecutionSnapshot{}
	var startTime, endTime sql.NullTime
	var workflowData sql.NullString
	var inputsJSON, outputsJSON []byte
	var errorMsg sql.NullString

	err := row.Scan(
		&snapshot.ID,
		&snapshot.WorkflowName,
		&snapshot.WorkflowHash,
		&snapshot.InputsHash,
		&snapshot.Status,
		&startTime,
		&endTime,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
		&snapshot.LastEventSeq,
		&workflowData,
		&inputsJSON,
		&outputsJSON,
		&errorMsg,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("execution snapshot not found")
		}
		return nil, fmt.Errorf("failed to scan snapshot: %w", err)
	}

	// Convert nullable fields
	if startTime.Valid {
		snapshot.StartTime = startTime.Time
	}
	if endTime.Valid {
		snapshot.EndTime = endTime.Time
	}
	if workflowData.Valid {
		snapshot.WorkflowData = []byte(workflowData.String)
	}
	if errorMsg.Valid {
		snapshot.Error = errorMsg.String
	}

	// Deserialize JSON fields
	if err := json.Unmarshal(inputsJSON, &snapshot.Inputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inputs: %w", err)
	}
	if err := json.Unmarshal(outputsJSON, &snapshot.Outputs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal outputs: %w", err)
	}

	return snapshot, nil
}

// Helper functions for nullable database values

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableBytes(b []byte) sql.NullString {
	if len(b) == 0 {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: t, Valid: true}
}
