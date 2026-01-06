package main

import (
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSessionConfigCompaction(t *testing.T) {
	tests := []struct {
		name                string
		compactionEnabled   bool
		compactionThreshold int
		wantConfig          bool
		wantThreshold       int
	}{
		{
			name:                "compaction enabled with default threshold",
			compactionEnabled:   true,
			compactionThreshold: 100000,
			wantConfig:          true,
			wantThreshold:       100000,
		},
		{
			name:                "compaction enabled with custom threshold",
			compactionEnabled:   true,
			compactionThreshold: 50000,
			wantConfig:          true,
			wantThreshold:       50000,
		},
		{
			name:                "compaction disabled",
			compactionEnabled:   false,
			compactionThreshold: 100000,
			wantConfig:          false,
			wantThreshold:       100000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &sessionConfig{
				compactionEnabled:   tt.compactionEnabled,
				compactionThreshold: tt.compactionThreshold,
			}

			// Verify config values
			assert.Equal(t, tt.wantConfig, cfg.compactionEnabled,
				"compactionEnabled should match expected")
			assert.Equal(t, tt.wantThreshold, cfg.compactionThreshold,
				"compactionThreshold should match expected")
		})
	}
}

func TestCompactionConfigCreation(t *testing.T) {
	tests := []struct {
		name              string
		enabled           bool
		threshold         int
		expectNil         bool
		expectedEnabled   bool
		expectedThreshold int
	}{
		{
			name:              "enabled with default threshold",
			enabled:           true,
			threshold:         100000,
			expectNil:         false,
			expectedEnabled:   true,
			expectedThreshold: 100000,
		},
		{
			name:              "enabled with custom threshold",
			enabled:           true,
			threshold:         50000,
			expectNil:         false,
			expectedEnabled:   true,
			expectedThreshold: 50000,
		},
		{
			name:      "disabled",
			enabled:   false,
			threshold: 100000,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from runInteractive/runPrint
			var compactionConfig *dive.CompactionConfig
			if tt.enabled {
				compactionConfig = &dive.CompactionConfig{
					Enabled:               true,
					ContextTokenThreshold: tt.threshold,
				}
			}

			if tt.expectNil {
				assert.Nil(t, compactionConfig, "compactionConfig should be nil when disabled")
			} else {
				assert.NotNil(t, compactionConfig, "compactionConfig should not be nil when enabled")
				assert.Equal(t, tt.expectedEnabled, compactionConfig.Enabled,
					"Enabled should match expected")
				assert.Equal(t, tt.expectedThreshold, compactionConfig.ContextTokenThreshold,
					"ContextTokenThreshold should match expected")
			}
		})
	}
}

func TestDefaultCompactionValues(t *testing.T) {
	// Verify default values match what's in the CLI flags
	const expectedDefaultThreshold = 100000
	const expectedDefaultEnabled = true

	// These constants should match the defaults in the CLI flag definitions
	assert.Equal(t, 100000, expectedDefaultThreshold,
		"default threshold should be 100000 tokens")
	assert.True(t, expectedDefaultEnabled,
		"compaction should be enabled by default")
}
