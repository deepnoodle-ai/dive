package teamconf

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
	"github.com/mendableai/firecrawl-go"
)

func populateToolConfig(config map[string]interface{}, options interface{}) error {
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
func initializeTools(enabledTools []string, toolConfigs map[string]map[string]interface{}) (map[string]llm.Tool, error) {
	toolsMap := make(map[string]llm.Tool)

	// Create a set of enabled tools for quick lookup
	enabledToolsSet := make(map[string]bool)
	for _, tool := range enabledTools {
		enabledToolsSet[tool] = true
	}

	if enabledToolsSet["google_search"] {
		key := os.Getenv("GOOGLE_SEARCH_CX")
		if key == "" {
			return nil, fmt.Errorf("google search requested but GOOGLE_SEARCH_CX not set")
		}
		googleClient, err := google.New()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Google Search: %w", err)
		}
		toolsMap["google_search"] = tools.NewGoogleSearch(googleClient)
	}

	if enabledToolsSet["firecrawl"] {
		key := os.Getenv("FIRECRAWL_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("firecrawl requested but FIRECRAWL_API_KEY not set")
		}
		app, err := firecrawl.NewFirecrawlApp(key, "")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Firecrawl: %w", err)
		}
		var options tools.FirecrawlScrapeToolOptions
		if config, ok := toolConfigs["firecrawl"]; ok {
			if err := populateToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate firecrawl tool config: %w", err)
			}
		}
		options.App = app
		toolsMap["firecrawl"] = tools.NewFirecrawlScrapeTool(options)
	}

	if enabledToolsSet["file_read"] {
		var options tools.FileReadToolOptions
		if config, ok := toolConfigs["file_read"]; ok {
			if err := populateToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate file_read tool config: %w", err)
			}
		}
		toolsMap["file_read"] = tools.NewFileReadTool(options)
	}

	if enabledToolsSet["file_write"] {
		var options tools.FileWriteToolOptions
		if config, ok := toolConfigs["file_write"]; ok {
			if err := populateToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate file_write tool config: %w", err)
			}
		}
		toolsMap["file_write"] = tools.NewFileWriteTool(options)
	}

	if enabledToolsSet["directory_list"] {
		var options tools.DirectoryListToolOptions
		if config, ok := toolConfigs["directory_list"]; ok {
			if err := populateToolConfig(config, &options); err != nil {
				return nil, fmt.Errorf("failed to populate directory_list tool config: %w", err)
			}
		}
		toolsMap["directory_list"] = tools.NewDirectoryListTool(options)
	}

	// Add more tools here as needed

	return toolsMap, nil
}
