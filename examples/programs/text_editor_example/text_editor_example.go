package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
)

const DefaultPrompt = `Create a file test.txt in the current directory with the
following content: 'Hello, world!'. First determine the current directory using
the command tool. Then use the text editor tool to create the file. If the file
already exists, do not overwrite it. The str_replace_based_edit_tool uses
absolute paths only.
`

func main() {
	var prompt string
	flag.StringVar(&prompt, "prompt", DefaultPrompt, "prompt to use")
	flag.Parse()

	logger := slogger.New(slogger.LevelFromString("debug"))

	textEditor := toolkit.NewTextEditorTool(toolkit.TextEditorToolOptions{})
	commandTool := toolkit.NewCommandTool(toolkit.CommandToolOptions{
		DenyList: []string{"rm"},
	})

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: dive.ConfirmIfNotReadOnly,
	})

	theAgent, err := agent.New(agent.Options{
		Name:         "Text Editor Agent",
		Instructions: "You are a helpful assistant that can edit files.",
		Tools:        []dive.Tool{textEditor, commandTool},
		Model:        anthropic.New(),
		Logger:       logger,
		Confirmer:    confirmer,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	response, err := theAgent.CreateResponse(ctx, dive.WithInput(prompt))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.OutputText())
}
