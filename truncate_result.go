package dive

import (
	"fmt"
	"os"
)

// TruncateResult truncates s if it exceeds max chars, writes the full content
// to a temp file, and returns the truncated string and the temp file path.
// If len(s) <= max, returns s unchanged and an empty path.
func TruncateResult(s string, max int) (result string, tmpPath string, err error) {
	runes := []rune(s)
	if len(runes) <= max {
		return s, "", nil
	}
	f, err := os.CreateTemp("", "dive-tool-result-*")
	if err != nil {
		return s, "", fmt.Errorf("truncate result: create temp file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		return s, "", fmt.Errorf("truncate result: write temp file: %w", err)
	}
	truncated := string(runes[:max])
	truncated += fmt.Sprintf("\n[Output truncated at %d chars. Full output written to %s.]", max, f.Name())
	return truncated, f.Name(), nil
}
