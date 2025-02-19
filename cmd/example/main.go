package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/getstingrai/agents"
	"github.com/getstingrai/agents/providers/anthropic"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a := agents.NewStandardAgent(agents.StandardAgentSpec{
		Name: "test",
		Role: &agents.Role{Name: "test"},
		LLM:  anthropic.New(),
	})

	err := a.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

	task := agents.NewTask(agents.TaskSpec{
		Name:        "Echo",
		Description: "Say each letter of the word 'echo', responding with one letter at a time.",
	})

	promise, err := a.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	result, err := promise.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Output.Content)

	task = agents.NewTask(agents.TaskSpec{
		Name:        "Jokes",
		Description: "Share two jokes. Ideally related to previous tasks. Respond with one at a time.",
	})

	promise, err = a.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	result, err = promise.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Output.Content)
}
