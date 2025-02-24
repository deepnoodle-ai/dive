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
	var providerName, modelName string
	flag.StringVar(&providerName, "provider", "anthropic", "provider to use")
	flag.StringVar(&modelName, "model", "", "model to use")
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
		Name:         "test",
		Role:         dive.Role{Description: "test"},
		LLM:          provider,
		Tools:        []llm.Tool{tools.NewGoogleSearch(googleClient)},
		CacheControl: "ephemeral",
		LogLevel:     "info",
		Hooks: llm.Hooks{
			llm.AfterGenerate: func(ctx context.Context, hookCtx *llm.HookContext) {
				fmt.Println("----")
				fmt.Println("INPUT")
				fmt.Println(dive.FormatMessages(hookCtx.Messages))
				fmt.Println("----")
				fmt.Println("OUTPUT")
				fmt.Println(dive.FormatMessages([]*llm.Message{hookCtx.Response.Message()}))
				fmt.Println("----")
			},
		},
	})

	if err := a.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter a task description: ")
		description, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		description = strings.TrimSpace(description)
		if description == "exit" {
			break
		}
		if description == "" {
			continue
		}

		// task := dive.NewTask(dive.TaskOptions{
		// 	Description: description,
		// })
		// promise, err := a.Work(ctx, task)
		// if err != nil {
		// 	log.Fatal(err)
		// }
		// result, err := promise.Get(ctx)
		// if err != nil {
		// 	log.Fatal(err)
		// }

		response, err := a.Chat(ctx, llm.NewUserMessage(description))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(response.Message().Text())
	}
}
