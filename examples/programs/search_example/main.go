package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/diveagents/dive/toolkit/kagi"
	"github.com/diveagents/dive/web"
)

func main() {
	searchProvider := flag.String("provider", "google", "Search provider to use: 'google' or 'kagi'")
	modelProvider := flag.String("model", "anthropic", "LLM model provider to use: 'anthropic', 'openai', or 'azure'")
	logLevel := flag.String("log", "", "Log level (default is 'debug' or value of LOG_LEVEL env var)")
	prompt := flag.String("prompt", "Research the history of computing. Respond with a brief markdown-formatted report.", "Research prompt")
	flag.Parse()

	if *logLevel == "" {
		if os.Getenv("LOG_LEVEL") != "" {
			*logLevel = os.Getenv("LOG_LEVEL")
		} else {
			*logLevel = "debug"
		}
	}

	ctx := context.Background()
	
	var searchClient web.Searcher
	var model llm.LLM
	var err error

	// Initialize search client
	switch *searchProvider {
	case "google":
		// Google requires:
		// - GOOGLE_SEARCH_CX
		// - GOOGLE_SEARCH_API_KEY
		// https://developers.google.com/custom-search/v1/introduction
		searchClient, err = google.New()
		if err != nil {
			log.Fatalf("Failed to initialize Google search client: %v", err)
		}
	case "kagi":
		// Kagi requires:
		// - KAGI_API_KEY
		// Note: Kagi search API is currently in private beta
		// See https://help.kagi.com/kagi/api/search.html to request an invite
		searchClient, err = kagi.New()
		if err != nil {
			log.Fatalf("Failed to initialize Kagi search client: %v", err)
		}
	default:
		log.Fatalf("Unknown search provider: %s. Use 'google' or 'kagi'", *searchProvider)
	}
	
	// Initialize LLM model
	switch *modelProvider {
	case "anthropic":
		// Anthropic requires:
		// - ANTHROPIC_API_KEY (set as environment variable)
		model = anthropic.New()
	case "openai":
		// OpenAI requires:
		// - OPENAI_API_KEY (set as environment variable)
		model = openai.New()
	case "azure":
		// Azure OpenAI requires:
		// - OPENAI_ENDPOINT
		// - OPENAI_API_KEY
		model = openai.New(
			openai.WithEndpoint(os.Getenv("OPENAI_ENDPOINT")),
		)
	default:
		log.Fatalf("Unknown model provider: %s. Use 'anthropic', 'openai', or 'azure'", *modelProvider)
	}

	researcher, err := agent.New(agent.Options{
		Name:   "Research Assistant",
		Goal:   "Use " + *searchProvider + " search with " + *modelProvider + " to research assigned topics",
		Model:  model,
		Logger: slogger.New(slogger.LevelFromString(*logLevel)),
		Tools:  []llm.Tool{toolkit.NewSearchTool(searchClient)},
	})
	if err != nil {
		log.Fatalf("Failed to create research agent: %v", err)
	}

	response, err := researcher.CreateResponse(ctx, dive.WithInput(*prompt))
	if err != nil {
		log.Fatalf("Failed to create response: %v", err)
	}
	fmt.Println(response.OutputText())
}