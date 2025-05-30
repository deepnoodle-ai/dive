package llm

import (
	"os"
	"strings"
)

// ReadEventsFile reads a file and returns a slice of strings, each corresponding
// to a line in an SSE event stream.
func ReadEventsFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(content), "\n"), nil
}
