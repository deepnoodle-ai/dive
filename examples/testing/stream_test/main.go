package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/experimental/toolkit/google"
	"github.com/fatih/color"
)

var (
	errorStyle = color.New(color.FgRed)
)

func fatal(msg string, args ...interface{}) {
	fmt.Printf(errorStyle.Sprint(msg)+"\n", args...)
	os.Exit(1)
}

const ComparisonPrompt = `
You are comparing two event streams and responses from different LLMs to verify
if the OpenAI transformation was successful.

**REFERENCE (CORRECT)**: Anthropic events and response 
**UNDER TEST**: OpenAI events and response (should match Anthropic after transformation)

## Analysis Guidelines

### ✅ IGNORE (Expected Differences):
- Exact number of content_block_delta events (chunking varies)
- Presence of "ping" events 
- Message IDs and model names
- The number of input and output tokens used may differ signficantly
- OpenAI's stream usually has zero tokens used in the message_start event and that's fine
- The response content may differ, but the meaning should be the same
- The response formatting may differ (commas/newlines/etc) but that's fine
- When a response has tool calls, Anthropic usually has a text content block before the tool call
  and OpenAI often does not. Ignore this. This will make the content block indexes differ by one.

### 🔍 ANALYZE (Critical Matches):
- Event sequence order (message_start → content_block_start → content_block_delta(s) → content_block_stop → message_delta → message_stop)
- Key-value pairs in corresponding events
- stop_reason values
- >10x differences in final token usage values
- Event structure consistency

## Required Output Format

### 📊 SUMMARY
Provide 1-2 sentence overview

### 🔍 EVENT SEQUENCE ANALYSIS
✅ CORRECT: List what matches correctly
❌ ISSUES: List problems found, or "None" if all good

### 📋 DETAILED FINDINGS

For each issue found, use this format:
❌ Issue Name
- Expected (Anthropic): exact value/structure in backticks
- Actual (OpenAI): exact value/structure in backticks  

For correct aspects, use:
✅ Aspect Name: Brief confirmation

### 🎯 FINAL VERDICT

TRANSFORMATION STATUS: 
- 🟢 PASS - Transformation successful, minor/acceptable differences only
- 🟡 PARTIAL - Mostly works but has non-critical issues  
- 🔴 FAIL - Critical differences that break functionality

KEY ISSUES COUNT: X critical, Y minor, Z total

Use clear, scannable formatting with emojis and make differences jump out visually.
`

const DefaultPrompt = "Count to 5, responding with the numbers only"

func main() {
	var logLevel, prompt string
	flag.StringVar(&logLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	flag.StringVar(&prompt, "prompt", DefaultPrompt, "Prompt to send to the LLM")
	flag.Parse()

	messages := llm.Messages{llm.NewUserTextMessage(prompt)}

	var sb1, sb2 strings.Builder

	// Capture Anthropic LLM events and response
	events, response := stream(anthropic.New(), messages)
	sb1.WriteString("---\n# Anthropic\n\n")
	sb1.WriteString("## Events:\n")
	sb1.WriteString(events)
	sb1.WriteString("\n## Response:\n")
	sb1.WriteString(response)

	fmt.Println(sb1.String())
	fmt.Println("")

	// Capture OpenAI LLM events and response
	events, response = stream(openaic.New(), messages)
	sb2.WriteString("---\n# OpenAI\n\n")
	sb2.WriteString("## Events:\n")
	sb2.WriteString(events)
	sb2.WriteString("\n## Response:\n")
	sb2.WriteString(response)

	fmt.Println(sb2.String())
	fmt.Println("")

	// Use Claude to compare the two responses
	model := anthropic.New()
	comparison := llm.NewUserMessage(
		&llm.TextContent{Text: ComparisonPrompt},
		&llm.TextContent{Text: sb1.String()},
		&llm.TextContent{Text: sb2.String()},
	)

	evaluation, err := model.Generate(
		context.Background(),
		llm.WithMessages(comparison),
		llm.WithSystemPrompt("You are a helpful assistant."),
	)
	if err != nil {
		fatal("error: %s", err)
	}
	fmt.Println(evaluation.Message().Text())
}

func eventToString(event *llm.Event) string {
	data, err := json.Marshal(event)
	if err != nil {
		fatal("error: %s", err)
	}
	return string(data)
}

func responseToString(response *llm.Response) string {
	data, err := json.Marshal(response)
	if err != nil {
		fatal("error: %s", err)
	}
	return string(data)
}

func stream(model llm.StreamingLLM, messages llm.Messages) (string, string) {
	var modelTools []llm.Tool
	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			fatal("failed to initialize Google Search: %s", err)
		}
		modelTools = append(modelTools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{Searcher: googleClient}))
	}

	stream, err := model.Stream(
		context.Background(),
		llm.WithMessages(messages...),
		llm.WithTools(modelTools...),
		llm.WithTemperature(0.1),
		llm.WithSystemPrompt("You are a helpful assistant."),
	)
	if err != nil {
		fatal("error: %s", err)
	}
	defer stream.Close()

	accumulator := llm.NewResponseAccumulator()
	var sb strings.Builder

	for stream.Next() {
		event := stream.Event()
		if err := accumulator.AddEvent(event); err != nil {
			fatal("error: %s", err)
		}
		sb.WriteString(eventToString(event) + "\n")
	}

	if err := stream.Err(); err != nil {
		fatal("error: %s", err)
	}
	if !accumulator.IsComplete() {
		fatal("incomplete response")
	}
	return sb.String(), responseToString(accumulator.Response())
}
