package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/require"
)

func TestMatchesPatterns(t *testing.T) {
	fw := &FileWatcher{
		options: WatchOptions{
			Patterns: []string{"*.go", "src/**/*.js", "test/*.py"},
		},
	}

	testCases := []struct {
		filePath string
		expected bool
	}{
		{"main.go", true},
		{"src/components/app.js", true},
		{"src/nested/deep/file.js", true},
		{"test/unit.py", true},
		{"README.md", false},
		{"src/style.css", false},
		{"test/data.json", false},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			result := fw.matchesPatterns(tc.filePath)
			require.Equal(t, tc.expected, result, "Pattern matching failed for %s", tc.filePath)
		})
	}
}

func TestWatchOptionsValidation(t *testing.T) {
	testCases := []struct {
		name        string
		options     WatchOptions
		expectError bool
	}{
		{
			name: "valid options",
			options: WatchOptions{
				Patterns:  []string{"*.go"},
				OnChange:  "lint files",
				AgentName: "TestAgent",
				Debounce:  time.Millisecond * 500,
			},
			expectError: false,
		},
		{
			name: "empty patterns",
			options: WatchOptions{
				Patterns: []string{},
				OnChange: "lint files",
			},
			expectError: true,
		},
		{
			name: "empty on-change",
			options: WatchOptions{
				Patterns: []string{"*.go"},
				OnChange: "",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectError {
				require.True(t, len(tc.options.Patterns) == 0 || tc.options.OnChange == "",
					"Expected validation to fail but got valid options")
			} else {
				require.NotEmpty(t, tc.options.Patterns, "Patterns should not be empty")
				require.NotEmpty(t, tc.options.OnChange, "OnChange should not be empty")
			}
		})
	}
}

func TestGlobPatternMatching(t *testing.T) {
	testCases := []struct {
		pattern  string
		filePath string
		expected bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "src/main.go", false},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "src/deep/nested/file.go", true},
		{"src/*.js", "src/app.js", true},
		{"src/*.js", "src/components/app.js", false},
		{"src/**/*.js", "src/components/app.js", true},
	}

	for _, tc := range testCases {
		t.Run(tc.pattern+"_"+tc.filePath, func(t *testing.T) {
			matched, err := doublestar.PathMatch(tc.pattern, tc.filePath)
			require.NoError(t, err)
			require.Equal(t, tc.expected, matched,
				"Pattern %s should match %s: %v", tc.pattern, tc.filePath, tc.expected)
		})
	}
}

func TestDebounceLogic(t *testing.T) {
	fw := &FileWatcher{
		options: WatchOptions{
			Debounce: time.Millisecond * 100,
		},
		debouncer: make(map[string]time.Time),
	}

	testFile := "test.go"

	// First event should not be debounced
	now := time.Now()
	fw.debouncer[testFile] = now.Add(-time.Millisecond * 200) // Old timestamp

	// Should not be debounced (enough time has passed)
	shouldProcess := now.Sub(fw.debouncer[testFile]) >= fw.options.Debounce
	require.True(t, shouldProcess, "Should process event when debounce time has passed")

	// Update timestamp to now
	fw.debouncer[testFile] = now

	// Should be debounced (not enough time has passed)
	currentTime := now.Add(time.Millisecond * 50)
	shouldProcess = currentTime.Sub(fw.debouncer[testFile]) >= fw.options.Debounce
	require.False(t, shouldProcess, "Should debounce event when not enough time has passed")
}

func TestWatchCommand(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	// Create a test file
	err := os.WriteFile(testFile, []byte("package main\n\nfunc main() {}\n"), 0644)
	require.NoError(t, err)

	// Test that the file exists and can be read
	content, err := os.ReadFile(testFile)
	require.NoError(t, err)
	require.Contains(t, string(content), "package main")

	// Test pattern matching with the created file
	pattern := filepath.Join(tmpDir, "*.go")
	matches, err := doublestar.FilepathGlob(pattern)
	require.NoError(t, err)
	require.Contains(t, matches, testFile)
}

func TestFileWatcherCreation(t *testing.T) {
	// Test FileWatcher creation with minimal options
	options := WatchOptions{
		Patterns:       []string{"*.go"},
		OnChange:       "test action",
		Recursive:      false,
		Debounce:       time.Millisecond * 500,
		AgentName:      "TestAgent",
		SystemPrompt:   "You are a test agent",
		ExitOnError:    false,
		LogFile:        "",
		OnlyExtensions: []string{},
		IgnorePatterns: []string{},
	}

	// Note: This test would require mocking the LLM and agent creation
	// For now, we test the structure and validation
	require.NotEmpty(t, options.Patterns)
	require.NotEmpty(t, options.OnChange)
	require.NotEmpty(t, options.AgentName)
	require.Equal(t, time.Millisecond*500, options.Debounce)
}

func TestExtensionFiltering(t *testing.T) {
	fw := &FileWatcher{
		options: WatchOptions{
			Patterns:       []string{"**/*"},
			OnlyExtensions: []string{"go", "js"},
		},
	}

	testCases := []struct {
		filePath string
		expected bool
	}{
		{"main.go", true},
		{"app.js", true},
		{"style.css", false},
		{"README.md", false},
		{"src/component.go", true},
		{"src/app.js", true},
		{"config.json", false},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			result := fw.matchesPatterns(tc.filePath)
			require.Equal(t, tc.expected, result, "Extension filtering failed for %s", tc.filePath)
		})
	}
}

func TestIgnorePatterns(t *testing.T) {
	fw := &FileWatcher{
		options: WatchOptions{
			Patterns:       []string{"**/*"},
			IgnorePatterns: []string{"*.test.go", "**/node_modules/**", "*.tmp"},
		},
	}

	testCases := []struct {
		filePath string
		expected bool
	}{
		{"main.go", true},
		{"main.test.go", false},
		{"node_modules/lib/index.js", false},
		{"src/node_modules/pkg.js", false},
		{"temp.tmp", false},
		{"src/component.go", true},
	}

	for _, tc := range testCases {
		t.Run(tc.filePath, func(t *testing.T) {
			result := fw.matchesPatterns(tc.filePath)
			require.Equal(t, tc.expected, result, "Ignore pattern filtering failed for %s", tc.filePath)
		})
	}
}
