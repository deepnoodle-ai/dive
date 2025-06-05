package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/openai"
)

const DefaultPrompt = "What are a few open tickets in Linear?"

func main() {
	var prompt, authToken, provider string
	flag.StringVar(&prompt, "p", DefaultPrompt, "prompt to use")
	flag.StringVar(&authToken, "t", "", "authorization token for MCP server")
	flag.StringVar(&provider, "provider", "anthropic", "provider to use (anthropic or openai-responses)")
	flag.Parse()

	// Create the MCP server configuration (same for both providers)
	mcpConfig := llm.MCPServerConfig{
		Type:               "url",
		Name:               "linear",
		URL:                "https://mcp.linear.app/sse",
		AuthorizationToken: authToken,
		ToolConfiguration: &llm.MCPToolConfiguration{
			Enabled: true,
			// AllowedTools: []string{"searchIssues", "createIssue"}, // Optional: restrict to specific tools
		},
	}

	var response *llm.Response
	var err error

	switch provider {
	case "anthropic":
		fmt.Println("Using Anthropic provider...")
		response, err = anthropic.New().Generate(
			context.Background(),
			llm.WithMessages(llm.NewUserTextMessage(prompt)),
			llm.WithMCPServers(mcpConfig),
		)
	case "openai-responses":
		fmt.Println("Using OpenAI Responses API provider...")
		response, err = openai.New(
			openai.WithModel("gpt-4o"),
		).Generate(
			context.Background(),
			llm.WithMessages(llm.NewUserTextMessage(prompt)),
			llm.WithMCPServers(mcpConfig),
		)
	default:
		log.Fatalf("Unknown provider: %s. Use 'anthropic' or 'openai-responses'", provider)
	}

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n=== Response ===\n%s\n", response.Message().Text())
}
