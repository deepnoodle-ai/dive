package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/llm"
)

// This example demonstrates how to use the FileThreadRepository
// to persist chat conversations across sessions
func main() {
	ctx := context.Background()

	// Create a temporary session file for this demo
	tmpDir, err := os.MkdirTemp("", "dive-session-demo")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sessionFile := filepath.Join(tmpDir, "demo-conversation.json")
	fmt.Printf("Demo session file: %s\n\n", sessionFile)

	// Create a file-based thread repository
	repo := agent.NewFileThreadRepository(sessionFile)
	if err := repo.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Simulate a conversation by creating some messages
	thread := &dive.Thread{
		ID:     "cli-chat",
		UserID: "demo-user",
		Messages: []*llm.Message{
			{
				ID:   "msg1",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello! I'm testing the session persistence feature."},
				},
			},
			{
				ID:   "msg2",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "Hello! Great to see you testing the session persistence. This conversation will be saved to a JSON file and can be resumed later."},
				},
			},
			{
				ID:   "msg3",
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "How does the persistence work exactly?"},
				},
			},
			{
				ID:   "msg4",
				Role: llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "The FileThreadRepository saves the entire conversation thread as JSON. It includes all messages, their roles, content, timestamps, and metadata. When you restart a chat with the same session file, all this history is loaded back into memory."},
				},
			},
		},
	}

	// Save the thread (this would happen automatically during chat)
	if err := repo.PutThread(ctx, thread); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Saved conversation to session file")

	// Simulate loading the session in a new instance (like restarting the CLI)
	newRepo := agent.NewFileThreadRepository(sessionFile)
	if err := newRepo.Load(ctx); err != nil {
		log.Fatal(err)
	}

	// Retrieve the conversation
	loadedThread, err := newRepo.GetThread(ctx, "cli-chat")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Loaded conversation from session file")
	fmt.Printf("✓ Found %d messages in the conversation\n\n", len(loadedThread.Messages))

	// Display the conversation
	fmt.Println("Conversation History:")
	fmt.Println("====================")
	for i, msg := range loadedThread.Messages {
		role := string(msg.Role)
		if role == "user" {
			role = "You"
		} else {
			role = "Assistant"
		}
		fmt.Printf("%d. %s: %s\n\n", i+1, role, msg.Text())
	}

	// Show file content
	fmt.Println("Session File Content:")
	fmt.Println("====================")
	content, err := os.ReadFile(sessionFile)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(content))
}