package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RunMetadata struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	Villagers          int    `json:"villagers"`
	Parallelism        int    `json:"parallelism"`
	TickMinutes        int    `json:"tick_minutes"`
	ReflectionInterval int    `json:"reflection_interval"`
	SeedParty          bool   `json:"seed_party"`
	SessionDir         string `json:"session_dir"`
}

type RunRecord struct {
	Type            string        `json:"type"`
	CreatedAt       time.Time     `json:"created_at"`
	Metadata        RunMetadata   `json:"metadata"`
	InitialSnapshot WorldSnapshot `json:"initial_snapshot"`
}

type TickRecord struct {
	Type   string      `json:"type"`
	Report *TickReport `json:"report"`
}

type RunRecorder struct {
	file    *os.File
	encoder *json.Encoder
}

func NewRunRecorder(path string, metadata RunMetadata, initial WorldSnapshot) (*RunRecorder, error) {
	if path == "" {
		return nil, fmt.Errorf("record path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	recorder := &RunRecorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}
	if err := recorder.encoder.Encode(RunRecord{
		Type:            "run",
		CreatedAt:       time.Now().UTC(),
		Metadata:        metadata,
		InitialSnapshot: initial,
	}); err != nil {
		_ = file.Close()
		return nil, err
	}
	return recorder, nil
}

func (r *RunRecorder) RecordTick(report *TickReport) error {
	if r == nil {
		return nil
	}
	return r.encoder.Encode(TickRecord{
		Type:   "tick",
		Report: report,
	})
}

func (r *RunRecorder) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}
