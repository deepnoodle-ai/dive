package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// readStdin reads all content from standard input
func readStdin() (string, error) {
	var content strings.Builder
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					content.WriteString(line)
				}
				break
			}
			return "", fmt.Errorf("error reading from stdin: %v", err)
		}
		content.WriteString(line)
	}

	return strings.TrimSpace(content.String()), nil
}
