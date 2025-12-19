package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/mcp"
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
)

// loadConfigFromPath loads configuration from a file or directory path
func loadConfigFromPath(path string) (*config.Config, error) {
	// Check if path is a directory or file
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error accessing path: %v", err)
	}

	var configEnv *config.Config

	if fi.IsDir() {
		// For directories, we need to read all YAML/JSON files and merge them
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		// Collect all YAML and JSON files
		var configFiles []string
		for _, entry := range entries {
			if !entry.IsDir() {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".yml" || ext == ".yaml" || ext == ".json" {
					configFiles = append(configFiles, filepath.Join(path, entry.Name()))
				}
			}
		}

		if len(configFiles) == 0 {
			return nil, fmt.Errorf("no yaml or json files found in directory: %s", path)
		}

		// Parse and merge all configuration files
		for _, file := range configFiles {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, fmt.Errorf("failed to read file %s: %w", file, err)
			}

			var env *config.Config
			ext := strings.ToLower(filepath.Ext(file))
			if ext == ".json" {
				env, err = config.ParseJSON(data)
			} else {
				env, err = config.ParseYAML(data)
			}

			if err != nil {
				return nil, fmt.Errorf("failed to parse file %s: %w", file, err)
			}

			if configEnv == nil {
				configEnv = env
			} else {
				configEnv = config.Merge(configEnv, env)
			}
		}
	} else {
		// Single file
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".json" {
			configEnv, err = config.ParseJSON(data)
		} else {
			configEnv, err = config.ParseYAML(data)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", path, err)
		}
	}

	return configEnv, nil
}

func registerMCPCommand(app *wontoncli.App) {
	mcpGroup := app.Group("mcp").
		Description("Manage MCP (Model Context Protocol) server connections")

	mcpGroup.Command("auth").
		Description("Authenticate with an MCP server using OAuth").
		Long(`Start the OAuth authentication flow for the specified MCP server.
This will open a browser window for user authorization and store the resulting tokens.
The first argument should be a workflow YAML file or directory containing workflow configuration.`).
		Args("config-path", "server-name").
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			configPath := ctx.Arg(0)
			serverName := ctx.Arg(1)

			cfg, err := loadConfigFromPath(configPath)
			if err != nil {
				return wontoncli.Errorf("failed to load configuration: %v", err)
			}

			var serverConfig config.MCPServer
			var found bool

			for _, server := range cfg.MCPServers {
				if server.Name == serverName {
					serverConfig = server
					found = true
					break
				}
			}

			if !found {
				return wontoncli.Errorf("MCP server '%s' not found in configuration", serverName)
			}

			if !serverConfig.IsOAuthEnabled() {
				return wontoncli.Errorf("OAuth is not configured for server '%s'", serverName)
			}

			client, err := mcp.NewClient(serverConfig.ToMCPConfig())
			if err != nil {
				return wontoncli.Errorf("failed to create MCP client: %v", err)
			}

			fmt.Printf("Starting OAuth authentication for server '%s'...\n", serverName)

			goCtx := context.Background()
			if err := client.Connect(goCtx); err != nil {
				return wontoncli.Errorf("OAuth authentication failed: %v", err)
			}

			fmt.Printf("OAuth authentication successful for server '%s'\n", serverName)

			client.Close()
			return nil
		})

	// Token commands as direct subcommands (flattened from token subgroup)
	mcpGroup.Command("token-refresh").
		Description("Refresh OAuth token for an MCP server").
		Args("config-path", "server-name").
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			serverName := ctx.Arg(1)
			fmt.Printf("Token refresh for server '%s' is not yet implemented\n", serverName)
			return nil
		})

	mcpGroup.Command("token-status").
		Description("Show OAuth token status for MCP servers").
		Args("config-path", "server-name?").
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			if ctx.NArg() >= 2 {
				serverName := ctx.Arg(1)
				fmt.Printf("Token status for server '%s' is not yet implemented\n", serverName)
			} else {
				fmt.Println("Token status for all servers is not yet implemented")
			}
			return nil
		})

	mcpGroup.Command("token-clear").
		Description("Clear stored OAuth tokens").
		Args("server-name?").
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)

			if ctx.NArg() >= 1 {
				serverName := ctx.Arg(0)
				return clearTokensForServer(serverName)
			}
			return clearAllTokens()
		})
}

// clearTokensForServer clears tokens for a specific server
func clearTokensForServer(serverName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	tokenPath := filepath.Join(homeDir, ".dive", "tokens", "mcp-tokens.json")

	// TODO: Implement server-specific token clearing
	fmt.Printf("Clearing tokens for server '%s' from %s (not yet implemented)\n", serverName, tokenPath)
	return nil
}

// clearAllTokens clears all stored tokens
func clearAllTokens() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	tokenPath := filepath.Join(homeDir, ".dive", "tokens", "mcp-tokens.json")

	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		fmt.Println("No token file found - nothing to clear")
		return nil
	}

	if err := os.Remove(tokenPath); err != nil {
		return fmt.Errorf("failed to remove token file: %w", err)
	}

	fmt.Printf("Cleared all OAuth tokens from %s\n", tokenPath)
	return nil
}
