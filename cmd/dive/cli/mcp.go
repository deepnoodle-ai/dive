package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/mcp"
)

// loadConfigFromPath loads configuration from a file or directory path
func loadConfigFromPath(path string) (*config.DiveConfig, error) {
	// Check if path is a directory or file
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("error accessing path: %v", err)
	}

	var configEnv *config.DiveConfig

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

			var env *config.DiveConfig
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

// mcpCmd represents the mcp command for managing MCP server connections
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP (Model Context Protocol) server connections",
	Long: `Commands for managing MCP server connections, including OAuth authentication,
token management, and server status checking.`,
}

// mcpAuthCmd handles OAuth authentication flow for MCP servers
var mcpAuthCmd = &cobra.Command{
	Use:   "auth [file or directory] [server-name]",
	Short: "Authenticate with an MCP server using OAuth",
	Long: `Start the OAuth authentication flow for the specified MCP server.
This will open a browser window for user authorization and store the resulting tokens.
The first argument should be a workflow YAML file or directory containing workflow configuration.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := args[0]
		serverName := args[1]

		// Load configuration from the specified path
		env, err := loadConfigFromPath(configPath)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Find the server configuration
		var serverConfig config.MCPServer
		var found bool

		for _, server := range env.MCPServers {
			if server.Name == serverName {
				serverConfig = server
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("MCP server '%s' not found in configuration", serverName)
		}

		if !serverConfig.IsOAuthEnabled() {
			return fmt.Errorf("OAuth is not configured for server '%s'", serverName)
		}

		// Create MCP client and trigger OAuth flow
		client, err := mcp.NewClient(serverConfig.ToMCPConfig())
		if err != nil {
			return fmt.Errorf("failed to create MCP client: %w", err)
		}

		fmt.Printf("Starting OAuth authentication for server '%s'...\n", serverName)

		ctx := context.Background()
		if err := client.Connect(ctx); err != nil {
			return fmt.Errorf("OAuth authentication failed: %w", err)
		}

		fmt.Printf("✓ OAuth authentication successful for server '%s'\n", serverName)

		// Clean up
		client.Close()
		return nil
	},
}

// mcpTokenCmd manages OAuth tokens for MCP servers
var mcpTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage OAuth tokens for MCP servers",
	Long:  `Commands for managing OAuth tokens including refresh, revoke, and status checking.`,
}

// mcpTokenRefreshCmd refreshes OAuth tokens for MCP servers
var mcpTokenRefreshCmd = &cobra.Command{
	Use:   "refresh [file or directory] [server-name]",
	Short: "Refresh OAuth token for an MCP server",
	Long: `Refresh the OAuth token for the specified MCP server.
The first argument should be a workflow YAML file or directory containing workflow configuration.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := args[0]
		serverName := args[1]
		fmt.Printf("Token refresh for server '%s' is not yet implemented\n", serverName)
		// TODO: Implement token refresh logic using configPath
		_ = configPath
		return nil
	},
}

// mcpTokenStatusCmd shows OAuth token status for MCP servers
var mcpTokenStatusCmd = &cobra.Command{
	Use:   "status [file or directory] [server-name]",
	Short: "Show OAuth token status for MCP servers",
	Long: `Display the OAuth token status for the specified server or all servers.
The first argument should be a workflow YAML file or directory containing workflow configuration.
If server-name is omitted, status for all configured servers will be shown.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := args[0]

		if len(args) == 2 {
			serverName := args[1]
			fmt.Printf("Token status for server '%s' is not yet implemented\n", serverName)
		} else {
			fmt.Println("Token status for all servers is not yet implemented")
		}
		// TODO: Implement token status logic using configPath
		_ = configPath
		return nil
	},
}

// mcpTokenClearCmd clears stored OAuth tokens
var mcpTokenClearCmd = &cobra.Command{
	Use:   "clear [server-name]",
	Short: "Clear stored OAuth tokens",
	Long:  `Clear stored OAuth tokens for the specified server or all servers.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			serverName := args[0]
			return clearTokensForServer(serverName)
		} else {
			return clearAllTokens()
		}
	},
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

	fmt.Printf("✓ Cleared all OAuth tokens from %s\n", tokenPath)
	return nil
}

func init() {
	// Add mcp command to root
	rootCmd.AddCommand(mcpCmd)

	// Add subcommands to mcp
	mcpCmd.AddCommand(mcpAuthCmd)
	mcpCmd.AddCommand(mcpTokenCmd)

	// Add token subcommands
	mcpTokenCmd.AddCommand(mcpTokenRefreshCmd)
	mcpTokenCmd.AddCommand(mcpTokenStatusCmd)
	mcpTokenCmd.AddCommand(mcpTokenClearCmd)
}
