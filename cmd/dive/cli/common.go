package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diveagents/dive/environment"
)

// getDiveConfigDir returns the dive configuration directory, creating it if it doesn't exist
func getDiveConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	diveDir := filepath.Join(homeDir, ".dive")
	if err := os.MkdirAll(diveDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create dive config directory: %w", err)
	}

	return diveDir, nil
}

// getDatabasePath returns the database path, using the provided override or defaulting to ~/.dive/executions.db
func getDatabasePath(databaseFlag string) (string, error) {
	if databaseFlag != "" {
		return databaseFlag, nil
	}

	diveDir, err := getDiveConfigDir()
	if err != nil {
		return "", fmt.Errorf("error getting dive config directory: %v", err)
	}

	return filepath.Join(diveDir, "executions.db"), nil
}

// getEventStore creates an event store instance for the given database path
func getEventStore(databaseFlag string) (environment.ExecutionEventStore, error) {
	dbPath, err := getDatabasePath(databaseFlag)
	if err != nil {
		return nil, err
	}

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not found: %s\nRun a workflow with 'dive run' to create an execution history", dbPath)
	}

	eventStore, err := environment.NewSQLiteExecutionEventStore(dbPath, environment.DefaultSQLiteStoreOptions())
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	return eventStore, nil
}
