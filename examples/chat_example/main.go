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
	"github.com/getstingrai/dive/slogger"
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

	logger := slogger.New(slogger.LevelDebug)

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
		Logger:       logger,
		Hooks: llm.Hooks{
			llm.AfterGenerate: func(ctx context.Context, hookCtx *llm.HookContext) {
				inputText := dive.FormatMessages(hookCtx.Messages)
				outputText := dive.FormatMessages([]*llm.Message{hookCtx.Response.Message()})
				os.WriteFile("output/chat.txt", []byte(inputText+"\n\n"+outputText), 0644)
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

		response, err := a.Work(ctx, dive.NewTask(dive.TaskOptions{
			Name:        "chat",
			Description: "Respond to this message: " + message,
		}))
		if err != nil {
			log.Fatal(err)
		}

		// fmt.Println(response.Message().Text())
		for {
			select {
			case e := <-response.Channel():
				fmt.Println("event", e.Type, e.AgentName, e.TaskName, string(e.Data))
			}
		}
	}
}
