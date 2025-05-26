package openairesponses

import (
	"net/http"
	"time"
)

// Provider creation options (set once when creating the provider)
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

// WithMaxRetries sets the maximum number of retries for failed requests
func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.maxRetries = maxRetries
	}
}

// WithBaseWait sets the base wait time for retry backoff
func WithBaseWait(baseWait time.Duration) Option {
	return func(p *Provider) {
		p.baseWait = baseWait
	}
}

// Generation-time options (can be set per request via llm.Option)

// WithStore sets whether to store the response for future reference
func WithStore(store bool) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Store = &store
	}
}

// WithBackground sets whether to process the request in the background
func WithBackground(background bool) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Background = &background
	}
}

// WithWebSearch enables web search with default options
func WithWebSearch() func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.EnabledTools = append(rc.EnabledTools, "web_search_preview")
	}
}

// WithWebSearchOptions enables web search with specific configuration
func WithWebSearchOptions(options WebSearchOptions) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.EnabledTools = append(rc.EnabledTools, "web_search_preview")
		rc.WebSearchOptions = &options
	}
}

// WithImageGeneration enables image generation with default options
func WithImageGeneration() func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.EnabledTools = append(rc.EnabledTools, "image_generation")
	}
}

// WithImageGenerationOptions enables image generation with specific configuration
func WithImageGenerationOptions(options ImageGenerationOptions) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.EnabledTools = append(rc.EnabledTools, "image_generation")
		rc.ImageGenerationOptions = &options
	}
}

// WithMCPServer adds an MCP server configuration
func WithMCPServer(label, serverURL string, headers map[string]string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		if rc.MCPServers == nil {
			rc.MCPServers = make(map[string]MCPServerConfig)
		}
		rc.MCPServers[label] = MCPServerConfig{
			ServerURL: serverURL,
			Headers:   headers,
		}
	}
}

// WithMCPServerOptions adds an MCP server with full configuration
func WithMCPServerOptions(label string, config MCPServerConfig) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		if rc.MCPServers == nil {
			rc.MCPServers = make(map[string]MCPServerConfig)
		}
		rc.MCPServers[label] = config
	}
}

// WithInstructions sets custom instructions for the request
func WithInstructions(instructions string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Instructions = &instructions
	}
}

// WithMaxOutputTokens sets the maximum number of output tokens
func WithMaxOutputTokens(maxTokens int) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.MaxOutputTokens = &maxTokens
	}
}

// WithMetadata sets metadata for the request
func WithMetadata(metadata map[string]string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Metadata = metadata
	}
}

// WithServiceTier sets the service tier for the request
func WithServiceTier(tier string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.ServiceTier = &tier
	}
}

// WithReasoningEffort sets the reasoning effort for o-series models
func WithReasoningEffort(effort string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		if rc.Reasoning == nil {
			rc.Reasoning = &ReasoningConfig{}
		}
		rc.Reasoning.Effort = &effort
	}
}

// WithTextFormat sets the text output format
func WithTextFormat(format TextFormat) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Text = &TextConfig{Format: format}
	}
}

// WithJSONSchema sets JSON schema output format
func WithJSONSchema(schema interface{}) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Text = &TextConfig{
			Format: TextFormat{
				Type:   "json_schema",
				Schema: schema,
			},
		}
	}
}

// WithTopP sets the top-p sampling parameter
func WithTopP(topP float64) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.TopP = &topP
	}
}

// WithTruncation sets the truncation strategy
func WithTruncation(truncation string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.Truncation = &truncation
	}
}

// WithUser sets the user identifier for the request
func WithUser(user string) func(*RequestConfig) {
	return func(rc *RequestConfig) {
		rc.User = &user
	}
}

// RequestConfig holds generation-time configuration
type RequestConfig struct {
	Store                  *bool
	Background             *bool
	EnabledTools           []string
	WebSearchOptions       *WebSearchOptions
	ImageGenerationOptions *ImageGenerationOptions
	MCPServers             map[string]MCPServerConfig
	Instructions           *string
	MaxOutputTokens        *int
	Metadata               map[string]string
	ServiceTier            *string
	Reasoning              *ReasoningConfig
	Text                   *TextConfig
	TopP                   *float64
	Truncation             *string
	User                   *string
}

// WebSearchOptions configures web search behavior
type WebSearchOptions struct {
	Domains           []string
	SearchContextSize string // "low", "medium", "high"
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
