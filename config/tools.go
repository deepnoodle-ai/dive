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

func initializeWebSearchTool(config map[string]interface{}) (dive.Tool, error) {
	key := os.Getenv("GOOGLE_SEARCH_CX")
	if key == "" {
		return nil, fmt.Errorf("google search requested but GOOGLE_SEARCH_CX not set")
	}
	googleClient, err := google.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize google search client: %w", err)
	}
	var options toolkit.WebSearchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid web_search tool configuration: %w", err)
		}
	}
	options.Searcher = googleClient
	return toolkit.NewWebSearchTool(options), nil
}

func initializeFetchTool(config map[string]interface{}) (dive.Tool, error) {
	key := os.Getenv("FIRECRAWL_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("firecrawl requested but FIRECRAWL_API_KEY not set")
	}
	client, err := firecrawl.New(firecrawl.WithAPIKey(key))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firecrawl: %w", err)
	}
	var options toolkit.FetchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid fetch tool configuration: %w", err)
		}
	}
	options.Fetcher = client
	return toolkit.NewFetchTool(options), nil
}

func initializeReadFileTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.ReadFileToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid read_file tool configuration: %w", err)
		}
	}
	return toolkit.NewReadFileTool(options), nil
}

func initializeWriteFileTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.WriteFileToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid write_file tool configuration: %w", err)
		}
	}
	return toolkit.NewWriteFileTool(options), nil
}

func initializeListDirectoryTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.ListDirectoryToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid list_directory tool configuration: %w", err)
		}
	}
	return toolkit.NewListDirectoryTool(options), nil
}

func initializeCommandTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.CommandToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid command tool configuration: %w", err)
		}
	}
	return toolkit.NewCommandTool(options), nil
}

func initializeAnthropicCodeExecutionTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.CodeExecutionToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid anthropic code execution tool configuration: %w", err)
		}
	}
	return anthropic.NewCodeExecutionTool(options), nil
}

func initializeAnthropicComputerTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.ComputerToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid anthropic computer tool configuration: %w", err)
		}
	}
	return anthropic.NewComputerTool(options), nil
}

func initializeAnthropicWebSearchTool(config map[string]interface{}) (dive.Tool, error) {
	var options anthropic.WebSearchToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid anthropic web search tool configuration: %w", err)
		}
	}
	return anthropic.NewWebSearchTool(options), nil
}

func initializeGenerateImageTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.ImageGenerationToolOptions
	client := openaisdk.NewClient()
	options.Client = &client
	return toolkit.NewImageGenerationTool(options), nil
}

func initializeTextEditorTool(config map[string]interface{}) (dive.Tool, error) {
	var options toolkit.TextEditorToolOptions
	if config != nil {
		if err := convertToolConfig(config, &options); err != nil {
			return nil, fmt.Errorf("invalid text_editor tool configuration: %w", err)
		}
	}
	return toolkit.NewTextEditorTool(options), nil
}

// ToolInitializers maps tool names to their initialization functions
var ToolInitializers = map[string]ToolInitializer{
	"web_search":               initializeWebSearchTool,
	"fetch":                    initializeFetchTool,
	"read_file":                initializeReadFileTool,
	"write_file":               initializeWriteFileTool,
	"list_directory":           initializeListDirectoryTool,
	"command":                  initializeCommandTool,
	"generate_image":           initializeGenerateImageTool,
	"anthropic_code_execution": initializeAnthropicCodeExecutionTool,
	"anthropic_computer":       initializeAnthropicComputerTool,
	"anthropic_web_search":     initializeAnthropicWebSearchTool,
	"text_editor":              initializeTextEditorTool,
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
		if tool.Enabled != nil && !*tool.Enabled {
			continue
		}
		initializedTool, err := InitializeToolByName(tool.Name, tool.Parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tool %s: %w", tool.Name, err)
		}
		toolsMap[tool.Name] = initializedTool
	}
	return toolsMap, nil
}
