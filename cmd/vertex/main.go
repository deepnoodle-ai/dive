package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"google.golang.org/genai"
)

func main() {
	var project, location string
	flag.StringVar(&project, "project", "", "Project ID")
	flag.StringVar(&location, "location", "", "Location")
	flag.Parse()

	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	if client.ClientConfig().Backend == genai.BackendVertexAI {
		fmt.Println("Calling VertexAI Backend...")
	} else {
		fmt.Println("Calling GeminiAPI Backend...")
	}
	var config *genai.GenerateContentConfig = &genai.GenerateContentConfig{Temperature: genai.Ptr[float32](0.5)}

	// Create a new Chat.
	chat, err := client.Chats.Create(ctx, "gemini-2.0-flash-001", config, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Send first chat message.
	result, err := chat.SendMessage(ctx, genai.Part{Text: "What's the weather in San Francisco?"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text())

	// Send second chat message.
	result, err = chat.SendMessage(ctx, genai.Part{Text: "How about New York?"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text())

	// Call the GenerateContent method.
	result, err = client.Models.GenerateContent(ctx, "gemini-2.0-flash-001", genai.Text("What is your name?"), config)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Text())
}
