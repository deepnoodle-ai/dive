package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/settings"
	"github.com/spf13/cobra"
)

// MCPScope represents the scope of MCP server configuration
type MCPScope string

const (
	LocalScope   MCPScope = "local"   // Project-specific, user only
	ProjectScope MCPScope = "project" // Shared via .mcp.json
	UserScope    MCPScope = "user"    // User-global
)

// mcpAddCmd adds a new MCP server
var mcpAddCmd = &cobra.Command{
	Use:   "add [name] [command/url] [args...]",
	Short: "Add a new MCP server",
	Long: `Add a new MCP server configuration.

Examples:
  # Add a local stdio server
  dive mcp add github -- npx -y github-mcp-server

  # Add with environment variable
  dive mcp add airtable --env AIRTABLE_API_KEY=your_key -- npx -y airtable-mcp-server

  # Add SSE server
  dive mcp add --transport sse linear https://mcp.linear.app/sse

  # Add HTTP server with auth header
  dive mcp add --transport http notion https://mcp.notion.com/mcp \
    --header "Authorization: Bearer token"

  # Add to user scope (global)
  dive mcp add --scope user myserver -- /usr/local/bin/myserver

  # Add to project scope (shared)
  dive mcp add --scope project shared-server -- npx server`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// Parse flags
		transport, _ := cmd.Flags().GetString("transport")
		scope, _ := cmd.Flags().GetString("scope")
		envVars, _ := cmd.Flags().GetStringSlice("env")
		headers, _ := cmd.Flags().GetStringSlice("header")

		// Create server config
		serverConfig := &settings.MCPServerSettings{
			Type: "stdio", // default
		}

		// Handle different transport types
		switch transport {
		case "sse", "http":
			serverConfig.Type = transport
			if len(args) < 2 {
				return fmt.Errorf("URL required for %s transport", transport)
			}
			serverConfig.URL = args[1]
		case "", "stdio":
			// Command with args after --
			dashIndex := -1
			for i, arg := range args {
				if arg == "--" {
					dashIndex = i
					break
				}
			}

			if dashIndex > 0 && dashIndex < len(args)-1 {
				serverConfig.Command = args[dashIndex+1]
				if dashIndex < len(args)-2 {
					serverConfig.Args = args[dashIndex+2:]
				}
			} else if len(args) > 1 {
				serverConfig.Command = args[1]
				if len(args) > 2 {
					serverConfig.Args = args[2:]
				}
			}
		default:
			return fmt.Errorf("unknown transport: %s", transport)
		}

		// Parse environment variables
		if len(envVars) > 0 {
			serverConfig.Env = make(map[string]string)
			for _, env := range envVars {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid env format: %s", env)
				}
				serverConfig.Env[parts[0]] = parts[1]
			}
		}

		// Parse headers
		if len(headers) > 0 {
			serverConfig.Headers = make(map[string]string)
			for _, header := range headers {
				parts := strings.SplitN(header, ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid header format: %s", header)
				}
				serverConfig.Headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}

		// Save to appropriate scope
		return saveMCPServer(name, serverConfig, MCPScope(scope))
	},
}

// mcpListCmd lists configured MCP servers
var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load settings from all scopes
		userServers := loadMCPServers(UserScope)
		projectServers := loadMCPServers(ProjectScope)
		localServers := loadMCPServers(LocalScope)

		// Display servers by scope
		fmt.Println(boldStyle.Sprint("MCP Servers:"))
		fmt.Println()

		if len(userServers) > 0 {
			fmt.Println(yellowStyle.Sprint("User Scope (global):"))
			for name, server := range userServers {
				printMCPServer(name, server, "  ")
			}
			fmt.Println()
		}

		if len(projectServers) > 0 {
			fmt.Println(greenStyle.Sprint("Project Scope (shared):"))
			for name, server := range projectServers {
				printMCPServer(name, server, "  ")
			}
			fmt.Println()
		}

		if len(localServers) > 0 {
			fmt.Println(successStyle.Sprint("Local Scope (current project):"))
			for name, server := range localServers {
				printMCPServer(name, server, "  ")
			}
		}

		if len(userServers)+len(projectServers)+len(localServers) == 0 {
			fmt.Println("No MCP servers configured")
		}

		return nil
	},
}

// mcpRemoveCmd removes an MCP server
var mcpRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		scope, _ := cmd.Flags().GetString("scope")

		if err := removeMCPServer(name, MCPScope(scope)); err != nil {
			return err
		}

		fmt.Printf("✓ Removed MCP server '%s'\n", name)
		return nil
	},
}

// mcpGetCmd gets details for a specific server
var mcpGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Get details for an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// Search in all scopes
		for _, scope := range []MCPScope{LocalScope, ProjectScope, UserScope} {
			servers := loadMCPServers(scope)
			if server, ok := servers[name]; ok {
				fmt.Printf("Server: %s\n", boldStyle.Sprint(name))
				fmt.Printf("Scope: %s\n", scope)
				printMCPServer(name, server, "")
				return nil
			}
		}

		return fmt.Errorf("server '%s' not found", name)
	},
}

// mcpAddJSONCmd adds an MCP server from JSON
var mcpAddJSONCmd = &cobra.Command{
	Use:   "add-json [name] [json]",
	Short: "Add an MCP server from JSON configuration",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		jsonStr := args[1]
		scope, _ := cmd.Flags().GetString("scope")

		var server settings.MCPServerSettings
		if err := json.Unmarshal([]byte(jsonStr), &server); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}

		if err := saveMCPServer(name, &server, MCPScope(scope)); err != nil {
			return err
		}

		fmt.Printf("✓ Added MCP server '%s' from JSON\n", name)
		return nil
	},
}

// mcpImportDesktopCmd imports servers from Claude Desktop
var mcpImportDesktopCmd = &cobra.Command{
	Use:   "add-from-claude-desktop",
	Short: "Import MCP servers from Claude Desktop configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find Claude Desktop config
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		var configPath string
		switch {
		case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
			// Windows/WSL
			configPath = filepath.Join(homeDir, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
		default:
			// macOS
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		}

		// Read config file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read Claude Desktop config: %w", err)
		}

		// Parse config
		var desktopConfig struct {
			MCPServers map[string]struct {
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}

		if err := json.Unmarshal(data, &desktopConfig); err != nil {
			return fmt.Errorf("failed to parse Claude Desktop config: %w", err)
		}

		// Import servers
		scope, _ := cmd.Flags().GetString("scope")
		imported := 0

		for name, server := range desktopConfig.MCPServers {
			diveServer := &settings.MCPServerSettings{
				Type:    "stdio",
				Command: server.Command,
				Args:    server.Args,
				Env:     server.Env,
			}

			// Check if name already exists
			existingName := name
			counter := 1
			for serverExists(existingName) {
				existingName = fmt.Sprintf("%s_%d", name, counter)
				counter++
			}

			if err := saveMCPServer(existingName, diveServer, MCPScope(scope)); err != nil {
				fmt.Printf("Failed to import '%s': %v\n", name, err)
				continue
			}

			imported++
			if existingName != name {
				fmt.Printf("✓ Imported '%s' as '%s' (name conflict)\n", name, existingName)
			} else {
				fmt.Printf("✓ Imported '%s'\n", name)
			}
		}

		fmt.Printf("\n%d server(s) imported successfully\n", imported)
		return nil
	},
}

// mcpResetChoicesCmd resets project server approval choices
var mcpResetChoicesCmd = &cobra.Command{
	Use:   "reset-project-choices",
	Short: "Reset approval choices for project MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		choicesFile := filepath.Join(homeDir, ".dive", "mcp-project-choices.json")
		if err := os.Remove(choicesFile); err != nil && !os.IsNotExist(err) {
			return err
		}

		fmt.Println("✓ Reset all project MCP server approval choices")
		return nil
	},
}

// mcpServeCmd starts Dive as an MCP server
var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run Dive as an MCP server",
	Long: `Start Dive as a stdio MCP server that other applications can connect to.

This exposes Dive's tools (View, Edit, LS, etc.) via MCP protocol.

To use in Claude Desktop, add this to claude_desktop_config.json:
{
  "mcpServers": {
    "dive": {
      "command": "dive",
      "args": ["mcp", "serve"],
      "env": {}
    }
  }
}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement MCP server mode
		fmt.Println("Starting Dive MCP server...")
		fmt.Println("This feature is coming soon!")
		return nil
	},
}

// Helper functions

func saveMCPServer(name string, server *settings.MCPServerSettings, scope MCPScope) error {
	switch scope {
	case UserScope:
		return saveUserMCPServer(name, server)
	case ProjectScope:
		return saveProjectMCPServer(name, server)
	default: // LocalScope
		return saveLocalMCPServer(name, server)
	}
}

func saveUserMCPServer(name string, server *settings.MCPServerSettings) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(homeDir, ".dive", "settings.json")

	// Load existing settings
	var s settings.Settings
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &s)
	}

	// Add server
	if s.MCPServers == nil {
		s.MCPServers = make(map[string]*settings.MCPServerSettings)
	}
	s.MCPServers[name] = server

	// Save settings
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

func saveProjectMCPServer(name string, server *settings.MCPServerSettings) error {
	mcpPath := ".mcp.json"

	// Load existing config
	var mcpConfig struct {
		MCPServers map[string]*settings.MCPServerSettings `json:"mcpServers"`
	}

	if data, err := os.ReadFile(mcpPath); err == nil {
		json.Unmarshal(data, &mcpConfig)
	}

	// Add server
	if mcpConfig.MCPServers == nil {
		mcpConfig.MCPServers = make(map[string]*settings.MCPServerSettings)
	}
	mcpConfig.MCPServers[name] = server

	// Save config
	data, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mcpPath, data, 0644)
}

func saveLocalMCPServer(name string, server *settings.MCPServerSettings) error {
	settingsPath := ".dive/settings.local.json"

	// Load existing settings
	var s settings.Settings
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &s)
	}

	// Add server
	if s.MCPServers == nil {
		s.MCPServers = make(map[string]*settings.MCPServerSettings)
	}
	s.MCPServers[name] = server

	// Save settings
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

func loadMCPServers(scope MCPScope) map[string]*settings.MCPServerSettings {
	var path string

	switch scope {
	case UserScope:
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".dive", "settings.json")
	case ProjectScope:
		// Check both .mcp.json and .dive/settings.json
		if _, err := os.Stat(".mcp.json"); err == nil {
			// Load from .mcp.json
			var mcpConfig struct {
				MCPServers map[string]*settings.MCPServerSettings `json:"mcpServers"`
			}
			if data, err := os.ReadFile(".mcp.json"); err == nil {
				json.Unmarshal(data, &mcpConfig)
				return mcpConfig.MCPServers
			}
		}
		path = ".dive/settings.json"
	default: // LocalScope
		path = ".dive/settings.local.json"
	}

	// Load settings file
	var s settings.Settings
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &s)
		return s.MCPServers
	}

	return nil
}

func removeMCPServer(name string, scope MCPScope) error {
	servers := loadMCPServers(scope)
	if servers == nil {
		return fmt.Errorf("server '%s' not found", name)
	}

	delete(servers, name)

	// Save updated config
	switch scope {
	case ProjectScope:
		if _, err := os.Stat(".mcp.json"); err == nil {
			// Save to .mcp.json
			mcpConfig := struct {
				MCPServers map[string]*settings.MCPServerSettings `json:"mcpServers"`
			}{
				MCPServers: servers,
			}
			data, _ := json.MarshalIndent(mcpConfig, "", "  ")
			return os.WriteFile(".mcp.json", data, 0644)
		}
	}

	// Save to settings file
	var s settings.Settings
	s.MCPServers = servers

	// Determine path
	var path string
	switch scope {
	case UserScope:
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".dive", "settings.json")
	case ProjectScope:
		path = ".dive/settings.json"
	default:
		path = ".dive/settings.local.json"
	}

	data, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func serverExists(name string) bool {
	for _, scope := range []MCPScope{LocalScope, ProjectScope, UserScope} {
		servers := loadMCPServers(scope)
		if _, ok := servers[name]; ok {
			return true
		}
	}
	return false
}

func printMCPServer(name string, server *settings.MCPServerSettings, indent string) {
	fmt.Printf("%s%s (%s)\n", indent, boldStyle.Sprint(name), server.Type)

	switch server.Type {
	case "stdio":
		if server.Command != "" {
			fmt.Printf("%s  Command: %s", indent, server.Command)
			if len(server.Args) > 0 {
				fmt.Printf(" %s", strings.Join(server.Args, " "))
			}
			fmt.Println()
		}
	case "sse", "http":
		if server.URL != "" {
			fmt.Printf("%s  URL: %s\n", indent, server.URL)
		}
	}

	if len(server.Env) > 0 {
		fmt.Printf("%s  Environment:\n", indent)
		for k, v := range server.Env {
			// Mask sensitive values
			displayValue := v
			if strings.Contains(strings.ToLower(k), "key") || strings.Contains(strings.ToLower(k), "token") {
				displayValue = "***"
			}
			fmt.Printf("%s    %s=%s\n", indent, k, displayValue)
		}
	}

	if len(server.Headers) > 0 {
		fmt.Printf("%s  Headers:\n", indent)
		for k, v := range server.Headers {
			// Mask auth headers
			displayValue := v
			if strings.EqualFold(k, "authorization") {
				displayValue = "***"
			}
			fmt.Printf("%s    %s: %s\n", indent, k, displayValue)
		}
	}
}

func init() {
	// Add commands to existing mcp command
	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpGetCmd)
	mcpCmd.AddCommand(mcpAddJSONCmd)
	mcpCmd.AddCommand(mcpImportDesktopCmd)
	mcpCmd.AddCommand(mcpResetChoicesCmd)
	mcpCmd.AddCommand(mcpServeCmd)

	// Add flags
	mcpAddCmd.Flags().String("transport", "stdio", "Transport type: stdio, sse, or http")
	mcpAddCmd.Flags().String("scope", "local", "Configuration scope: local, project, or user")
	mcpAddCmd.Flags().StringSlice("env", nil, "Environment variables (format: KEY=value)")
	mcpAddCmd.Flags().StringSlice("header", nil, "HTTP headers (format: Name: Value)")

	mcpRemoveCmd.Flags().String("scope", "local", "Configuration scope: local, project, or user")
	mcpAddJSONCmd.Flags().String("scope", "local", "Configuration scope: local, project, or user")
	mcpImportDesktopCmd.Flags().String("scope", "user", "Configuration scope: local, project, or user")
}