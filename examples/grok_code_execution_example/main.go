// Command grok_code_execution_example demonstrates Grok's server-side code
// execution tool. Grok writes and runs Python in a sandbox to compute an exact
// answer, then explains it. With IncludeOutputs enabled, the executed code and
// its outputs come back in the response so we can show what Grok ran.
//
// Run with XAI_API_KEY (or GROK_API_KEY) set:
//
//	go run ./grok_code_execution_example -prompt "What is 2^64?"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/grok"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
)

const defaultPrompt = "Compute the compound interest on $10,000 at 5% annually for 10 years. Use code and show the final amount."

func main() {
	var prompt string
	flag.StringVar(&prompt, "prompt", defaultPrompt, "prompt to send to Grok")
	flag.Parse()

	// IncludeOutputs asks the server to return the code interpreter's outputs
	// (logs/files) alongside the executed code.
	codeExec := grok.NewCodeExecutionTool(grok.CodeExecutionToolOptions{
		IncludeOutputs: true,
	})

	provider := grok.New() // defaults to grok-4.3

	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithTools(codeExec),
	)
	if err != nil {
		log.Fatal(err)
	}

	// The code Grok wrote and executed arrives as a code_interpreter_call
	// content block (defined in the openai provider that Grok builds on).
	for _, content := range response.Content {
		if call, ok := content.(*openaiProvider.CodeInterpreterCallContent); ok {
			fmt.Println("Grok executed this code:")
			fmt.Println("------------------------")
			fmt.Println(call.Code)
			fmt.Println("------------------------")
			for _, result := range call.Results {
				if result.Logs != "" {
					fmt.Printf("Output: %s\n", result.Logs)
				}
			}
			fmt.Println()
		}
	}

	fmt.Println("Answer:")
	fmt.Println(response.Message().Text())

	fmt.Printf("\nUsage: input=%d output=%d reasoning=%d\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.ReasoningTokens)
}
