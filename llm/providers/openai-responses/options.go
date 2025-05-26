package openairesponses

import "net/http"

type Option func(*Provider)

func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
	}
}

func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.endpoint = endpoint
	}
}

func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

func WithStore(store bool) Option {
	return func(p *Provider) {
		p.store = &store
	}
}

func WithBackground(background bool) Option {
	return func(p *Provider) {
		p.background = &background
	}
}

func WithWebSearch(enabled bool) Option {
	return func(p *Provider) {
		if enabled {
			p.enabledTools = append(p.enabledTools, "web_search_preview")
		}
	}
}

func WithWebSearchOptions(options WebSearchOptions) Option {
	return func(p *Provider) {
		p.enabledTools = append(p.enabledTools, "web_search_preview")
		p.webSearchOptions = &options
	}
}

func WithImageGeneration(enabled bool) Option {
	return func(p *Provider) {
		if enabled {
			p.enabledTools = append(p.enabledTools, "image_generation")
		}
	}
}

func WithImageGenerationOptions(options ImageGenerationOptions) Option {
	return func(p *Provider) {
		p.enabledTools = append(p.enabledTools, "image_generation")
		p.imageGenerationOptions = &options
	}
}

func WithMCPServer(label, serverURL string, headers map[string]string) Option {
	return func(p *Provider) {
		if p.mcpServers == nil {
			p.mcpServers = make(map[string]MCPServerConfig)
		}
		p.mcpServers[label] = MCPServerConfig{
			ServerURL: serverURL,
			Headers:   headers,
		}
	}
}

func WithMCPServerOptions(label string, config MCPServerConfig) Option {
	return func(p *Provider) {
		if p.mcpServers == nil {
			p.mcpServers = make(map[string]MCPServerConfig)
		}
		p.mcpServers[label] = config
	}
}

// WebSearchOptions configures web search behavior
type WebSearchOptions struct {
	Domains           []string
	SearchContextSize string // "small", "medium", "large"
	UserLocation      *UserLocation
}

// ImageGenerationOptions configures image generation behavior
type ImageGenerationOptions struct {
	Size    string // "1024x1024", "1024x1536", etc.
	Quality string // "low", "medium", "high", "auto"
	// Format field is not supported by the OpenAI Responses API
	// Format        string // "png", "jpeg", "webp"
	Compression   *int   // 0-100 for JPEG/WebP
	Background    string // "transparent", "opaque", "auto"
	PartialImages *int   // 1-3 for streaming
}

// MCPServerConfig configures an MCP server connection
type MCPServerConfig struct {
	ServerURL       string
	AllowedTools    []string
	RequireApproval interface{} // "never", "always", or object with tool-specific settings
	Headers         map[string]string
}
