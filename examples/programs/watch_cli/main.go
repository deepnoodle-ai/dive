// watch_cli is a standalone program for monitoring files and triggering AI actions on changes.
//
// This is a simplified example demonstrating file watching with AI-driven responses.
// For the full implementation from cmd/dive/cli/watch.go with batching, filtering,
// and advanced options, see the dive repository.
//
// Usage:
//
//	go run ./examples/programs/watch_cli --path . --on-change "Summarize the changes"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/fsnotify/fsnotify"
)

func main() {
	path := flag.String("path", ".", "Path to watch for changes")
	onChange := flag.String("on-change", "", "Action to perform when changes are detected (prompt to LLM)")
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	recursive := flag.Bool("recursive", false, "Watch directories recursively")
	flag.Parse()

	if *onChange == "" {
		log.Fatal("--on-change is required")
	}

	if err := runWatch(*path, *onChange, *provider, *model, *recursive); err != nil {
		log.Fatal(err)
	}
}

func runWatch(path, onChange, providerName, modelName string, recursive bool) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating watcher: %v", err)
	}
	defer watcher.Close()

	// Add initial path
	if err := watcher.Add(path); err != nil {
		return fmt.Errorf("error adding path to watcher: %v", err)
	}

	// Add subdirectories if recursive
	if recursive {
		err := filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return watcher.Add(walkPath)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking path: %v", err)
		}
	}

	fmt.Printf("Watching %s for changes...\n", path)
	fmt.Printf("Action on change: %s\n\n", onChange)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Debounce changes
	debounceTimer := time.NewTimer(0)
	<-debounceTimer.C // Drain the initial timer

	var pendingChanges []string

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				fmt.Printf("Detected change: %s %s\n", event.Op, event.Name)
				pendingChanges = append(pendingChanges, fmt.Sprintf("%s: %s", event.Op, event.Name))

				// Reset debounce timer
				debounceTimer.Reset(1 * time.Second)
			}

		case <-debounceTimer.C:
			if len(pendingChanges) > 0 {
				fmt.Printf("\nProcessing %d changes...\n", len(pendingChanges))
				if err := processChanges(onChange, pendingChanges, providerName, modelName); err != nil {
					fmt.Printf("Error processing changes: %v\n", err)
				}
				pendingChanges = nil
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("Watcher error: %v\n", err)

		case <-sigChan:
			fmt.Println("\nShutting down...")
			return nil
		}
	}
}

func processChanges(prompt string, changes []string, providerName, modelName string) error {
	ctx := context.Background()

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	changesSummary := ""
	for _, change := range changes {
		changesSummary += change + "\n"
	}

	fullPrompt := fmt.Sprintf("%s\n\nChanges detected:\n%s", prompt, changesSummary)

	response, err := model.Generate(ctx, llm.WithUserTextMessage(fullPrompt))
	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}

	fmt.Println("\n--- AI Response ---")
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}
	fmt.Println("-------------------\n")

	return nil
}
