package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
)

func main() {
	var verbose bool
	var providerName, modelName string
	flag.StringVar(&providerName, "provider", "anthropic", "provider to use")
	flag.StringVar(&modelName, "model", "", "model to use")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.Parse()

	ctx := context.Background()

	var provider llm.LLM
	switch providerName {
	case "anthropic":
		provider = anthropic.New()
	case "openai":
		provider = openai.New()
	case "groq":
		provider = groq.New(groq.WithModel("deepseek-r1-distill-llama-70b"))
	}

	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	a := dive.NewAgent(dive.AgentOptions{
		Name: "Dr. Smith",
		Role: dive.Role{
			Description: `
You are a virtual doctor for role-playing purposes only. You can discuss general
medical topics, symptoms, and health advice, but always clarify that you're not
a real doctor and cannot provide actual medical diagnosis or treatment. Refuse
to answer non-medical questions. Use maximum medical jargon.`,
		},
		LLM:          provider,
		Tools:        []llm.Tool{tools.NewGoogleSearch(googleClient)},
		CacheControl: "ephemeral",
		LogLevel:     "info",
		Hooks: llm.Hooks{
			llm.AfterGenerate: func(ctx context.Context, hookCtx *llm.HookContext) {
				if verbose {
					fmt.Println("----")
					fmt.Println("INPUT")
					fmt.Println(dive.FormatMessages(hookCtx.Messages))
					fmt.Println("OUTPUT")
					fmt.Println(dive.FormatMessages([]*llm.Message{hookCtx.Response.Message()}))
					fmt.Println("----")
				}
			},
		},
	})

	if err := a.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter a chat message about a medical topic: ")
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		message = strings.TrimSpace(message)
		if message == "exit" {
			break
		}
		if message == "" {
			continue
		}
		response, err := a.Chat(ctx, llm.NewUserMessage(message))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(response.Message().Text())
	}
}
