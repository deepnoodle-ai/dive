package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
)

const DefaultPrompt = "What are a few open tickets in Linear?"

func main() {
	var prompt, authToken string
	flag.StringVar(&prompt, "p", DefaultPrompt, "prompt to use")
	flag.StringVar(&authToken, "t", "", "authorization token for MCP server")
	flag.Parse()

	response, err := anthropic.New().Generate(
		context.Background(),
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithMCPServers(
			llm.MCPServerConfig{
				Type:               "url",
				Name:               "linear",
				URL:                "https://mcp.linear.app/sse",
				AuthorizationToken: authToken,
			},
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
