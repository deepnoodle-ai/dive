package cli

import (
	"fmt"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/wonton/cli"
)

func registerConfigCommand(app *cli.App) {
	configGroup := app.Group("config").
		Description("Work with Dive configurations")

	configGroup.Command("check").
		Description("Validate a Dive configuration").
		Args("file").
		Run(func(ctx *cli.Context) error {
			parseGlobalFlags(ctx)

			cfg, err := config.LoadDirectory(ctx.Arg(0))
			if err != nil {
				return cli.Errorf("%v", err)
			}
			fmt.Printf("Configuration is valid with %d agent(s)\n", len(cfg.Agents))
			return nil
		})
}
