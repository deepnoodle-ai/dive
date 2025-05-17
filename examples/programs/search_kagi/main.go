package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/kagi"
)

// Use Kagi and Azure OpenAI to research the history of computing.
//
// Note Kagi search API is currently in private beta, see https://help.kagi.com/kagi/api/search.html to request an invite.
//
// You'll need to have these environment variables set:
// - KAGI_API_KEY
// - OPENAI_ENDPOINT
//   - Something like https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/chat/completions?api-version=2024-06-01
//   - See https://learn.microsoft.com/en-us/azure/ai-services/openai/reference
//
// - OPENAI_API_KEY
func main() {

	var logLevel string
	if os.Getenv("LOG_LEVEL") != "" {
		logLevel = os.Getenv("LOG_LEVEL")
	} else {
		logLevel = "debug"
	}

	ctx := context.Background()

	kagi, err := kagi.New()
	if err != nil {
		log.Fatal(err)
	}

	model := openai.New(
		openai.WithEndpoint(os.Getenv("OPENAI_ENDPOINT")),
	)
	researcher, err := agent.New(agent.Options{
		Name:   "Research Assistant",
		Goal:   "Use Kagi to research assigned topics",
		Model:  model,
		Logger: slogger.New(slogger.LevelFromString(logLevel)),
		Tools:  []llm.Tool{toolkit.NewSearchTool(kagi)},
	})
	if err != nil {
		log.Fatal(err)
	}

	prompt := "Research the history of computing. Respond with a brief markdown-formatted report."

	response, err := researcher.CreateResponse(ctx, dive.WithInput(prompt))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.OutputText())
}
