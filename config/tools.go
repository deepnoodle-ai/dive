package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/firecrawl"
	"github.com/diveagents/dive/toolkit/google"
)

func convertToolConfig(config map[string]interface{}, options interface{}) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal tool config: %w", err)
	}
	if err := json.Unmarshal(configJSON, &options); err != nil {
		return fmt.Errorf("failed to unmarshal tool config: %w", err)
	}
	return nil
}

// initializeTools initializes tools with custom configurations
func initializeTools(tools []Tool) (map[string]dive.Tool, error) {

	toolsMap := make(map[string]dive.Tool)

	configsByName := make(map[string]map[string]interface{})
	for _, tool := range tools {
		name := tool.Name
		if name == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		configsByName[name] = tool.Parameters
	}

	if _, ok := configsByName["Web.Search"]; ok {
		key := os.Getenv("GOOGLE_SEARCH_CX")
		if key == "" {
			return nil, fmt.Errorf("google search requested but GOOGLE_SEARCH_CX not set")
		}
		googleClient, err := google.New()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Google Search: %w", err)
		}
		toolsMap["Web.Search"] = toolkit.NewSearchTool(googleClient)
	}

	if _, ok := configsByName["Web.Fetch"]; ok {
		key := os.Getenv("FIRECRAWL_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("firecrawl requested but FIRECRAWL_API_KEY not set")
		}
		client, err := firecrawl.New(firecrawl.WithAPIKey(key))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Firecrawl: %w", err)
		}
		toolsMap["Web.Fetch"] = toolkit.NewFetchTool(client)
	}

	if _, ok := configsByName["File.Read"]; ok {
		var options toolkit.FileReadToolOptions
		if config, ok := configsByName["File.Read"]; ok {
			if err := convertToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate file_read tool config: %w", err)
			}
		}
		toolsMap["File.Read"] = toolkit.NewFileReadTool(options)
	}

	if _, ok := configsByName["File.Write"]; ok {
		var options toolkit.FileWriteToolOptions
		if config, ok := configsByName["File.Write"]; ok {
			if err := convertToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate file_write tool config: %w", err)
			}
		}
		toolsMap["File.Write"] = toolkit.NewFileWriteTool(options)
	}

	if _, ok := configsByName["Directory.List"]; ok {
		var options toolkit.DirectoryListToolOptions
		if config, ok := configsByName["Directory.List"]; ok {
			if err := convertToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate directory_list tool config: %w", err)
			}
		}
		toolsMap["Directory.List"] = toolkit.NewDirectoryListTool(options)
	}

	if _, ok := configsByName["Command"]; ok {
		var options toolkit.CommandToolOptions
		if config, ok := configsByName["Command"]; ok {
			if err := convertToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate command tool config: %w", err)
			}
		}
		toolsMap["Command"] = toolkit.NewCommandTool(options)
	}

	// Add more tools here as needed

	return toolsMap, nil
}
