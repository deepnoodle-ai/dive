package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/getstingrai/dive/slogger"
	"github.com/getstingrai/dive/teamconf"
	"github.com/zclconf/go-cty/cty"
)

var (
	boldStyle = color.New(color.Bold)

	successStyle = color.New(color.FgGreen)

	errorStyle = color.New(color.FgRed)

	infoStyle = color.New(color.FgBlue)
)

func fatal(msg string, args ...interface{}) {
	fmt.Printf(msg+"\n", args...)
	os.Exit(1)
}

func main() {
	var logLevel, varsFlag, outDir string
	flag.StringVar(&logLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	flag.StringVar(&varsFlag, "vars", "", "Comma-separated list of variables in format key=value")
	flag.StringVar(&outDir, "output", "", "Output directory for task results")
	flag.Parse()

	if flag.NArg() == 0 {
		fatal("Error: file path is required")
	}

	filePath := flag.Arg(0)

	vars := teamconf.VariableValues{}
	if varsFlag != "" {
		varPairs := strings.Split(varsFlag, ",")
		for _, pair := range varPairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				fatal("Error: invalid variable format: %s", pair)
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vars[key] = cty.StringVal(value)
		}
	}

	ctx := context.Background()

	logger := slogger.New(slogger.LevelFromString(logLevel))

	team, err := teamconf.TeamFromFile(filePath, teamconf.WithLogger(logger))
	if err != nil {
		fatal(err.Error())
	}
	if err := team.Start(ctx); err != nil {
		fatal(err.Error())
	}
	defer team.Stop(ctx)

	fmt.Println("Starting", boldStyle.Sprint(team.Name()))

	stream, err := team.Work(ctx)
	if err != nil {
		fatal(err.Error())
	}

	if outDir != "" {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fatal("Error: failed to create output directory: %s", err)
		}
	}

	saveOutput := func(name string, result string) {
		if outDir == "" {
			return
		}
		dstPath := filepath.Join(outDir, name+".txt")
		if err := os.WriteFile(dstPath, []byte(result), 0644); err != nil {
			fatal("Error: failed to save output for %s: %s", name, err)
		}
		fmt.Println(infoStyle.Sprint("Saved output to " + dstPath))
	}

	for event := range stream.Channel() {
		switch event.Type {
		case "task.result":
			resultText := "\n" + boldStyle.Sprint(event.TaskName+":") + "\n" +
				successStyle.Sprint(event.TaskResult.Content) + "\n"
			fmt.Println(resultText)
			saveOutput(event.TaskName, event.TaskResult.Content)

		case "task.error":
			fmt.Println(boldStyle.Sprint(event.TaskName+":") + "\n")
			fmt.Println(errorStyle.Sprint(event.Error) + "\n")
			fatal(event.Error)
		}
	}
}
