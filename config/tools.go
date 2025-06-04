package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/firecrawl"
	"github.com/diveagents/dive/toolkit/google"
	openaisdk "github.com/openai/openai-go"
)

type ToolInitializer func(config map[string]interface{}) (dive.Tool, error)

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

// InitializeWebSearchTool initializes the Web.Search tool with the given configuration
func InitializeWebSearchTool(config map[string]interface{}) (dive.Tool, error) {
	key := os.Getenv("GOOGLE_SEARCH_CX")
	if key == "" {
		return nil, fmt.Errorf("google search requested but GOOGLE_SEARCH_CX not set")
	}
	googleClient, err := google.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Google Search: %w", err)
	}
	var options toolkit.SearchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate web_search tool config: %w", err)
		}
	}
	options.Searcher = googleClient
	return toolkit.NewSearchTool(options), nil
}

// InitializeWebFetchTool initializes the Web.Fetch tool with the given configuration
func InitializeWebFetchTool(config map[string]interface{}) (dive.Tool, error) {
	key := os.Getenv("FIRECRAWL_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("firecrawl requested but FIRECRAWL_API_KEY not set")
	}
	client, err := firecrawl.New(firecrawl.WithAPIKey(key))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firecrawl: %w", err)
	}
	var options toolkit.FetchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate web_fetch tool config: %w", err)
		}
	}
	options.Fetcher = client
	return toolkit.NewFetchTool(options), nil
}

// InitializeFileReadTool initializes the File.Read tool with the given configuration
func InitializeFileReadTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.FileReadToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate file_read tool config: %w", err)
		}
	}
	return toolkit.NewFileReadTool(options), nil
}

// InitializeFileWriteTool initializes the File.Write tool with the given configuration
func InitializeFileWriteTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.FileWriteToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate file_write tool config: %w", err)
		}
	}
	return toolkit.NewFileWriteTool(options), nil
}

// InitializeDirectoryListTool initializes the Directory.List tool with the given configuration
func InitializeDirectoryListTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.DirectoryListToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate directory_list tool config: %w", err)
		}
	}
	return toolkit.NewDirectoryListTool(options), nil
}

// InitializeCommandTool initializes the Command tool with the given configuration
func InitializeCommandTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.CommandToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate command tool config: %w", err)
		}
	}
	return toolkit.NewCommandTool(options), nil
}

// InitializeAnthropicCodeExecutionTool initializes the Anthropic Code Execution tool with the given configuration
func InitializeAnthropicCodeExecutionTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.CodeExecutionToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate Anthropic code execution tool config: %w", err)
		}
	}
	return anthropic.NewCodeExecutionTool(options), nil
}

// InitializeAnthropicComputerTool initializes the Anthropic Computer tool with the given configuration
func InitializeAnthropicComputerTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.ComputerToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate Anthropic computer tool config: %w", err)
		}
	}
	return anthropic.NewComputerTool(options), nil
}

// InitializeAnthropicWebSearchTool initializes the Anthropic Web Search tool with the given configuration
func InitializeAnthropicWebSearchTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.WebSearchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("failed to populate Anthropic web search tool config: %w", err)
		}
	}
	return anthropic.NewWebSearchTool(options), nil
}

func InitializeImageGenerationTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.ImageGenerationToolOptions
	client := openaisdk.NewClient()
	options.Client = &client
	return toolkit.NewImageGenerationTool(options), nil
}

// ToolInitializers maps tool names to their initialization functions
var ToolInitializers = map[string]ToolInitializer{
	"Web.Search":              InitializeWebSearchTool,
	"Web.Fetch":               InitializeWebFetchTool,
	"File.Read":               InitializeFileReadTool,
	"File.Write":              InitializeFileWriteTool,
	"Directory.List":          InitializeDirectoryListTool,
	"Command":                 InitializeCommandTool,
	"Image.Generate":          InitializeImageGenerationTool,
	"Anthropic.CodeExecution": InitializeAnthropicCodeExecutionTool,
	"Anthropic.Computer":      InitializeAnthropicComputerTool,
	"Anthropic.WebSearch":     InitializeAnthropicWebSearchTool,
}

// InitializeToolByName initializes a tool by its name with the given configuration
func InitializeToolByName(toolName string, config map[string]interface{}) (dive.Tool, error) {
	initializer, exists := ToolInitializers[toolName]
	if !exists {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
	return initializer(config)
}

// GetAvailableToolNames returns a list of all available tool names
func GetAvailableToolNames() []string {
	names := make([]string, 0, len(ToolInitializers))
	for name := range ToolInitializers {
		names = append(names, name)
	}
	return names
}

// initializeTools initializes tools with custom configurations
func initializeTools(tools []Tool) (map[string]dive.Tool, error) {
	toolsMap := make(map[string]dive.Tool)
	for _, tool := range tools {
		if tool.Name == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		initializedTool, err := InitializeToolByName(tool.Name, tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tool %s: %w", tool.Name, err)
		}
		toolsMap[tool.Name] = initializedTool
	}
	return toolsMap, nil
}
