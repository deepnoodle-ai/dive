package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/slogger"
	"github.com/getstingrai/dive/teamconf"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
)

// runTeam runs a team defined in an HCL file
func runTeam(filePath string, variables []string, logLevel string) error {

	vars := teamconf.VariableValues{}
	if len(variables) > 0 {
		for _, pair := range variables {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid variable format: %s", pair)
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vars[key] = cty.StringVal(value)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	team, err := teamconf.LoadConfFile(filePath)
	if err != nil {
		return fmt.Errorf("error loading team: %v", err)
	}

	logger := slogger.New(slogger.LevelFromString(logLevel))

	fmt.Println(titleStyle.Render(" Running Team "))
	fmt.Println(infoStyle.Render("File:"), filePath)
	if len(vars) > 0 {
		fmt.Println(infoStyle.Render("Variables:"), formatVars(vars))
	}
	fmt.Println()

	if err := team.Start(ctx); err != nil {
		return fmt.Errorf("error starting team: %v", err)
	}

	fmt.Printf("Running team %s with %d tasks...\n",
		infoStyle.Render(team.Name()),
		len(tasks))
	fmt.Println()

	// Set up event and result channels
	eventCh := make(chan dive.Event)
	resultCh := make(chan *dive.TaskResult)

	// Start a goroutine to print events and results as they come in
	go func() {
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				if verbose {
					fmt.Printf("%s %s\n",
						infoStyle.Render(time.Now().Format(time.RFC3339)), event.Name)
				}
			case result, ok := <-resultCh:
				if !ok {
					return
				}
				// printTaskResult(result, verbose)
				print(result)
			case <-ctx.Done():
				return
			}
		}
	}()

	agent := team.Agents()[0]
	response, err := agent.Generate(ctx, llm.NewUserMessage("Hello, how are you?"))
	if err != nil {
		return fmt.Errorf("error generating response: %v", err)
	}
	fmt.Println(response.Message().Text())

	// Close channels
	close(eventCh)
	close(resultCh)

	// Print final results
	fmt.Println()
	fmt.Println(titleStyle.Render(" Results "))

	if err != nil {
		fmt.Println(errorStyle.Render(fmt.Sprintf("Error: %v", err)))
	}
	return nil
}

var runCmd = &cobra.Command{
	Use:   "run [file]",
	Short: "Run a team defined in a YAML or HCL file",
	Long:  `Run a team defined in a YAML or HCL file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		vars, err := cmd.Flags().GetStringSlice("var")
		if err != nil {
			return fmt.Errorf("error getting variables: %v", err)
		}
		logLevel, err := cmd.Flags().GetString("log-level")
		if err != nil {
			return fmt.Errorf("error getting log level: %v", err)
		}
		return runTeam(filePath, vars, logLevel)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("var", "", "", "Variable to pass to the team in 'key=value' format")
	runCmd.Flags().StringP("log-level", "", "info", "Log level to use (debug, info, warn, error)")
}
