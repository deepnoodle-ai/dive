package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const maxInputHistoryEntries = 1000

type inputHistoryFile struct {
	Entries []string `json:"entries"`
}

// inputHistoryStore persists interactive input independently of conversation
// sessions, so recalled prompts remain available after restarting the CLI.
type inputHistoryStore struct {
	path string
}

func defaultInputHistoryStore() (*inputHistoryStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	return &inputHistoryStore{path: filepath.Join(home, ".dive", "history.json")}, nil
}

func (s *inputHistoryStore) Load() ([]string, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read input history: %w", err)
	}

	var file inputHistoryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse input history: %w", err)
	}
	return limitInputHistory(file.Entries), nil
}

func (s *inputHistoryStore) Save(entries []string) error {
	data, err := json.MarshalIndent(inputHistoryFile{Entries: limitInputHistory(entries)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode input history: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create input history directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create input history file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write input history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close input history: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("save input history: %w", err)
	}
	committed = true
	return nil
}

func limitInputHistory(entries []string) []string {
	if len(entries) <= maxInputHistoryEntries {
		return append([]string(nil), entries...)
	}
	return append([]string(nil), entries[len(entries)-maxInputHistoryEntries:]...)
}
