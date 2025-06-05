package main

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/diveagents/dive/mcp"
	"github.com/fatih/color"
)

func main() {
	ctx := context.Background()

	// You will need `npm` and `uvx` installed to run this example.
	// https://github.com/astral-sh/uv

	// Run two MCP servers. Examples borrowed from:
	// https://github.com/modelcontextprotocol/servers
	configs := []*mcp.ServerConfig{
		{
			Type: "stdio",
			Name: "filesystem-server",
			URL:  "npx",
			Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		},
		{
			Type: "stdio",
			Name: "git-server",
			URL:  "uvx",
			Args: []string{"mcp-server-git"},
		},
	}

	manager := mcp.NewManager()
	defer cleanup(manager)

	// Initialize servers
	fmt.Println("🚀 Initializing MCP servers...")
	if err := manager.InitializeServers(ctx, configs); err != nil {
		log.Printf("Warning: Some servers failed to initialize: %v\n", err)
	} else {
		fmt.Println("✅ All servers initialized successfully!")
	}

	// Display server status and demonstrate capabilities
	displayServerStatus(manager)
	displayAvailableTools(manager)
}

func displayServerStatus(manager *mcp.Manager) {
	fmt.Println("\n📊 Server Connection Status:")
	serverStatus := manager.GetServerStatus()

	for serverName, isConnected := range serverStatus {
		status := "❌ Failed to connect"
		if isConnected {
			status = "✅ Connected successfully"
		}
		fmt.Printf("  %s: %s\n", serverName, status)
	}
}

func displayAvailableTools(manager *mcp.Manager) {
	bold := color.New(color.Bold).SprintFunc()

	fmt.Println("\n🔧 Available Tools:")
	allTools := manager.GetAllTools()

	// Sort tools by name for consistent output
	var toolKeys []string
	for toolKey := range allTools {
		toolKeys = append(toolKeys, toolKey)
	}
	sort.Strings(toolKeys)

	for _, toolKey := range toolKeys {
		tool := allTools[toolKey]
		fmt.Printf("  - %s: %s", bold(toolKey), tool.Description())

		if annotations := tool.Annotations(); annotations != nil && annotations.Title != "" {
			fmt.Printf(" (%s)", annotations.Title)
		}
		fmt.Println()
	}
}

func cleanup(manager *mcp.Manager) {
	fmt.Println("\n🧹 Cleaning up...")
	if err := manager.Close(); err != nil {
		log.Printf("Error during cleanup: %v\n", err)
	}
}
