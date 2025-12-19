// ask_cli is a standalone program for asking an agent a single question.
//
// Usage:
//
//	go run ./examples/programs/ask_cli "What is the capital of France?"
//	go run ./examples/programs/ask_cli --system "You are a helpful assistant" "Tell me about Go"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/internal/random"
	divelogs "github.com/deepnoodle-ai/dive/log"
	"github.com/deepnoodle-ai/dive/threads"
)

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	systemPrompt := flag.String("system", "", "System prompt for the agent")
	goal := flag.String("goal", "", "Goal for the agent")
	instructions := flag.String("instructions", "", "Instructions for the agent")
	thread := flag.String("thread", "", "Thread ID to use")
	flag.Parse()

	message := strings.Join(flag.Args(), " ")
	if message == "" {
		log.Fatal("no message provided")
	}

	if err := runAsk(*provider, *model, *systemPrompt, *goal, *instructions, message, *thread); err != nil {
		log.Fatal(err)
	}
}

func runAsk(providerName, modelName, systemPrompt, goal, instructions, message, threadID string) error {
	ctx := context.Background()

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	logger := divelogs.New(divelogs.LevelWarn)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting home directory: %v", err)
	}
	threadsDir := filepath.Join(homeDir, ".dive", "threads")
	threadRepo := threads.NewDiskRepository(threadsDir)

	interactor := dive.NewTerminalInteractor(dive.TerminalInteractorOptions{
		Mode: dive.InteractIfNotReadOnly,
	})

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:             "Assistant",
		SystemPrompt:     systemPrompt,
		Goal:             goal,
		Instructions:     instructions,
		Model:            model,
		Logger:           logger,
		ThreadRepository: threadRepo,
		ModelSettings:    &dive.ModelSettings{},
		Interactor:       interactor,
	})
	if err != nil {
		return fmt.Errorf("error creating agent: %v", err)
	}

	if threadID == "" {
		threadID = random.Integer()
	}

	_, err = agent.CreateResponse(ctx,
		dive.WithInput(message),
		dive.WithThreadID(threadID),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeModelEvent {
				payload := item.Event
				if payload.Delta != nil {
					delta := payload.Delta
					if delta.Text != "" {
						fmt.Print(delta.Text)
					} else if delta.Thinking != "" {
						fmt.Print(delta.Thinking)
					}
				}
			}
			return nil
		}),
	)

	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}

	fmt.Println()
	return nil
}
