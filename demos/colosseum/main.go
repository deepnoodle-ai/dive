// Command colosseum runs a cross-provider Werewolf arena: each player is a
// different LLM provider's model (Claude, GPT, Gemini, Grok), all driven through
// Dive's single llm.LLM interface in one Go program.
//
//	colosseum run        --players claude,gpt,gemini,grok   # play one match
//	colosseum tournament --players claude,gpt,grok -n 10    # run a leaderboard
//	colosseum serve      --dir transcripts                  # web replay viewer
//	colosseum leaderboard transcripts/                      # print standings
//	colosseum highlights match.jsonl                        # analyze one match
//	colosseum serve-agent --provider claude                 # host an A2A challenger
//
// The whole point is the thing only Dive makes easy: many providers behind one
// interface, in a single binary. See README.md.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/provider"
)

type command struct {
	name string
	desc string
	run  func(args []string) error
}

func commands() []command {
	return []command{
		{"run", "play a single match and print it live", cmdRun},
		{"tournament", "run N matches and build an ELO leaderboard", cmdTournament},
		{"serve", "serve the web replay viewer + leaderboard", cmdServe},
		{"leaderboard", "print the leaderboard for a transcripts dir or leaderboard.json", cmdLeaderboard},
		{"highlights", "analyze one transcript: metrics + highlights", cmdHighlights},
		{"serve-agent", "host a Dive agent as an A2A challenger (bring your own agent)", cmdServeAgent},
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	name := os.Args[1]
	for _, c := range commands() {
		if c.name == name {
			if err := c.run(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
	usage()
	os.Exit(2)
}

func usage() {
	fmt.Fprintf(os.Stderr, `The Colosseum — cross-provider Werewolf arena (built with Dive)

Usage:
  colosseum <command> [flags]

Commands:
`)
	for _, c := range commands() {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", c.name, c.desc)
	}
	fmt.Fprintf(os.Stderr, "\nKnown providers: %s\n", strings.Join(provider.Keys(), ", "))
	fmt.Fprintf(os.Stderr, "Run `colosseum <command> -h` for command flags.\n")
}
