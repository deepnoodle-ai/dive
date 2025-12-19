// diff_cli is a standalone program for AI-powered semantic diff between files.
//
// Usage:
//
//	go run ./examples/programs/diff_cli file1.txt file2.txt
//	go run ./examples/programs/diff_cli --format json file1.txt file2.txt
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
)

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	format := flag.String("format", "text", "Output format: text, markdown, or json")
	contextLines := flag.Int("context", 3, "Number of context lines to show")
	flag.Parse()

	if flag.NArg() < 2 {
		log.Fatal("usage: diff_cli [flags] file1 file2")
	}

	file1 := flag.Arg(0)
	file2 := flag.Arg(1)

	if err := runDiff(*provider, *model, file1, file2, *format, *contextLines); err != nil {
		log.Fatal(err)
	}
}

func runDiff(providerName, modelName, file1, file2, format string, contextLines int) error {
	ctx := context.Background()

	content1, err := os.ReadFile(file1)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", file1, err)
	}

	content2, err := os.ReadFile(file2)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", file2, err)
	}

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	systemPrompt := fmt.Sprintf(`You are a code and document diff analyzer. Compare the two files and provide a semantic diff.
Output format: %s
Focus on meaningful changes, not just line-by-line differences.
Explain what changed and why it might matter.`, format)

	userMessage := fmt.Sprintf(`Compare these two files:

File 1 (%s):
%s

File 2 (%s):
%s

Provide a semantic analysis of the differences.`, file1, string(content1), file2, string(content2))

	response, err := model.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userMessage),
	)
	if err != nil {
		return fmt.Errorf("error generating diff: %v", err)
	}

	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}

	return nil
}
