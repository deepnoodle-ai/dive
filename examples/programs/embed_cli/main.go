// embed_cli is a standalone program for generating text embeddings.
//
// Usage:
//
//	echo "Hello, world!" | go run ./examples/programs/embed_cli
//	go run ./examples/programs/embed_cli --text "Some text to embed"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
)

func main() {
	text := flag.String("text", "", "Input text to embed (if not provided, reads from stdin)")
	model := flag.String("model", "", "Embedding model to use")
	output := flag.String("output", "json", "Output format: json (full response) or vector (embedding values only)")
	flag.Parse()

	inputText := *text
	if inputText == "" {
		// Try to read from stdin
		stat, err := os.Stdin.Stat()
		if err != nil {
			log.Fatal(err)
		}
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			inputText, err = readStdin()
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	inputText = strings.TrimSpace(inputText)
	if inputText == "" {
		log.Fatal("no text input provided. Use --text flag or pipe text via stdin.")
	}

	if err := runEmbedding(*model, *output, []string{inputText}); err != nil {
		log.Fatal(err)
	}
}

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

func runEmbedding(model, outputFormat string, inputs []string) error {
	ctx := context.Background()

	provider := openai.NewEmbeddingProvider()

	var opts []embedding.Option
	opts = append(opts, embedding.WithInputs(inputs))

	if model != "" {
		opts = append(opts, embedding.WithModel(model))
	}
	opts = append(opts, embedding.WithDimensions(1536))

	response, err := provider.Embed(ctx, opts...)
	if err != nil {
		return fmt.Errorf("error generating embedding: %w", err)
	}

	switch outputFormat {
	case "json":
		jsonData, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling response to JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	case "vector":
		if len(response.Floats) > 0 {
			if len(response.Floats) == 1 {
				jsonData, err := json.Marshal(response.Floats[0])
				if err != nil {
					return fmt.Errorf("error marshaling float vector to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			} else {
				jsonData, err := json.Marshal(response.Floats)
				if err != nil {
					return fmt.Errorf("error marshaling float vectors to JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			}
		} else {
			return fmt.Errorf("no embeddings returned")
		}
	default:
		return fmt.Errorf("unsupported output format: %s (supported: json, vector)", outputFormat)
	}
	return nil
}
