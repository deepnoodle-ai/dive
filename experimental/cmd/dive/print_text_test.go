package main

import (
	"bytes"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func textDelta(s string) *dive.ResponseItem {
	return &dive.ResponseItem{
		Type:  dive.ResponseItemTypeModelEvent,
		Event: &llm.Event{Type: llm.EventTypeContentBlockDelta, Delta: &llm.EventDelta{Text: s}},
	}
}

func thinkingDelta(s string) *dive.ResponseItem {
	return &dive.ResponseItem{
		Type:  dive.ResponseItemTypeModelEvent,
		Event: &llm.Event{Type: llm.EventTypeContentBlockDelta, Delta: &llm.EventDelta{Thinking: s}},
	}
}

func messageItem() *dive.ResponseItem {
	return &dive.ResponseItem{Type: dive.ResponseItemTypeMessage, Message: &llm.Message{Role: llm.Assistant}}
}

func toolCallItem(id string) *dive.ResponseItem {
	return &dive.ResponseItem{Type: dive.ResponseItemTypeToolCall, ToolCall: toolUse(id, "Read", `{}`)}
}

func printItems(showThinking bool, fallback string, items ...*dive.ResponseItem) string {
	var buf bytes.Buffer
	p := newTextPrinter(&buf, showThinking)
	for _, item := range items {
		p.handle(item)
	}
	p.finish(fallback)
	return buf.String()
}

func TestTextPrinterSeparatesTextAcrossTurns(t *testing.T) {
	out := printItems(false, "",
		textDelta("Finding your note"), textDelta(" — one moment."),
		messageItem(), toolCallItem("t1"),
		textDelta("note.txt contains \"hello\"."),
		messageItem(),
	)
	assert.Equal(t, "Finding your note — one moment.\n\nnote.txt contains \"hello\".\n", out)
}

func TestTextPrinterKeepsOneTurnTogether(t *testing.T) {
	out := printItems(false, "", textDelta("Hello"), textDelta(", world.\n"), messageItem())
	assert.Equal(t, "Hello, world.\n", out)
}

func TestTextPrinterDoesNotDoubleTrailingNewline(t *testing.T) {
	out := printItems(false, "",
		textDelta("Reading.\n"), messageItem(), toolCallItem("t1"),
		textDelta("Done.\n"), messageItem(),
	)
	assert.Equal(t, "Reading.\n\nDone.\n", out)
}

func TestTextPrinterNormalizesNewlinesAtPassageEdges(t *testing.T) {
	out := printItems(true, "",
		thinkingDelta("\nPlan.\n\n"), textDelta("\n"), textDelta("\nFirst para.\n"), textDelta("\nSecond.\n\n\n"),
		messageItem(),
	)
	assert.Equal(t, "Thinking:\nPlan.\n\nResponse:\nFirst para.\n\nSecond.\n", out)
}

func TestTextPrinterBoundaryWithoutFollowingTextAddsNothing(t *testing.T) {
	out := printItems(false, "", textDelta("Answer."), messageItem(), toolCallItem("t1"), messageItem())
	assert.Equal(t, "Answer.\n", out)
}

func TestTextPrinterHidesThinkingWhenDisabled(t *testing.T) {
	out := printItems(false, "", thinkingDelta("hmm"), textDelta("Answer."), messageItem())
	assert.Equal(t, "Answer.\n", out)
}

func TestTextPrinterSeparatesThinkingAcrossTurns(t *testing.T) {
	out := printItems(true, "",
		thinkingDelta("I should read the file."), messageItem(), toolCallItem("t1"),
		thinkingDelta("It says hello."), textDelta("The file says hello."), messageItem(),
	)
	assert.Equal(t, "Thinking:\nI should read the file.\n\nIt says hello.\n\nResponse:\nThe file says hello.\n", out)
}

func TestTextPrinterHeadersFollowInterleavedPassages(t *testing.T) {
	out := printItems(true, "",
		thinkingDelta("Plan."), textDelta("Reading the file."), messageItem(), toolCallItem("t1"),
		thinkingDelta("Got it."), textDelta("It says hello."), messageItem(),
	)
	want := "Thinking:\nPlan.\n\nResponse:\nReading the file.\n\n" +
		"Thinking:\nGot it.\n\nResponse:\nIt says hello.\n"
	assert.Equal(t, want, out)
}

func TestTextPrinterNoResponseHeaderWithoutThinking(t *testing.T) {
	out := printItems(true, "", textDelta("Answer."), messageItem())
	assert.Equal(t, "Answer.\n", out)
}

func TestTextPrinterFallsBackToOutputText(t *testing.T) {
	assert.Equal(t, "Final.\n", printItems(false, "Final."))
	assert.Equal(t, "\n", printItems(false, ""))
	assert.Equal(t, "Thinking:\nHmm.\n\nResponse:\nFinal.\n",
		printItems(true, "Final.", thinkingDelta("Hmm."), messageItem()))
	assert.Equal(t, "Thinking:\nHmm.\n", printItems(true, "", thinkingDelta("Hmm."), messageItem()))
}
