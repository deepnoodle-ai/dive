package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/getstingrai/agents"
	"github.com/getstingrai/agents/llm"
	"github.com/getstingrai/agents/providers/anthropic"
	"github.com/getstingrai/agents/tools"
	"github.com/getstingrai/agents/tools/google"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	a := agents.NewStandardAgent(agents.StandardAgentSpec{
		Name:         "test",
		Role:         &agents.Role{Name: "test"},
		LLM:          anthropic.New(),
		Tools:        []llm.Tool{tools.NewGoogleSearch(googleClient)},
		CacheControl: "ephemeral",
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

		task := agents.NewTask(agents.TaskSpec{Description: description})

		promise, err := a.Work(ctx, task)
		if err != nil {
			log.Fatal(err)
		}
		result, err := promise.Get(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(result.Output.Content)
	}
}
