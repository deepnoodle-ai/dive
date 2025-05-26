package openairesponses

import (
	"fmt"
	"net/http"

	"github.com/diveagents/dive/llm"
)

// LLM Option functions that can be used with llm.Generate() or llm.Stream()

// LLMWithStore sets whether to store the response for future reference
func LLMWithStore(store bool) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		if store {
			config.Features = append(config.Features, "openai-responses:store")
		}
	}
}

// LLMWithBackground sets whether to process the request in the background
func LLMWithBackground(background bool) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		if background {
			config.Features = append(config.Features, "openai-responses:background")
		}
	}
}

// LLMWithWebSearch enables web search with default options
func LLMWithWebSearch() llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:web_search")
	}
}

// LLMWithWebSearchOptions enables web search with specific configuration
func LLMWithWebSearchOptions(options WebSearchOptions) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:web_search")
		// Store options in request headers for now - in a real implementation you might
		// want to add a proper extension mechanism to llm.Config
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		// This is a simplified approach - in practice you'd want a better serialization
		if len(options.Domains) > 0 {
			config.RequestHeaders.Set("X-OpenAI-Responses-Web-Search-Domains", options.Domains[0]) // simplified
		}
		if options.SearchContextSize != "" {
			config.RequestHeaders.Set("X-OpenAI-Responses-Web-Search-Context-Size", options.SearchContextSize)
		}
	}
}

// LLMWithImageGeneration enables image generation with default options
func LLMWithImageGeneration() llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:image_generation")
	}
}

// LLMWithImageGenerationOptions enables image generation with specific configuration
func LLMWithImageGenerationOptions(options ImageGenerationOptions) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:image_generation")
		// Store options in request headers
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		if options.Size != "" {
			config.RequestHeaders.Set("X-OpenAI-Responses-Image-Size", options.Size)
		}
		if options.Quality != "" {
			config.RequestHeaders.Set("X-OpenAI-Responses-Image-Quality", options.Quality)
		}
		if options.Background != "" {
			config.RequestHeaders.Set("X-OpenAI-Responses-Image-Background", options.Background)
		}
	}
}

// LLMWithMCPServer adds an MCP server configuration
func LLMWithMCPServer(label, serverURL string, headers map[string]string) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:mcp:"+label)
		// Store MCP config in request headers
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-MCP-"+label+"-URL", serverURL)
		// Note: headers would need more sophisticated serialization in practice
	}
}

// LLMWithInstructions sets custom instructions for the request
func LLMWithInstructions(instructions string) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-Instructions", instructions)
	}
}

// LLMWithServiceTier sets the service tier for the request
func LLMWithServiceTier(tier string) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-Service-Tier", tier)
	}
}

// LLMWithReasoningEffort sets the reasoning effort for o-series models
func LLMWithReasoningEffort(effort string) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-Reasoning-Effort", effort)
	}
}

// LLMWithJSONSchema sets JSON schema output format
func LLMWithJSONSchema(schema interface{}) llm.Option {
	return func(config *llm.Config) {
		if config.Features == nil {
			config.Features = []string{}
		}
		config.Features = append(config.Features, "openai-responses:json_schema")
		// In practice, you'd want to serialize the schema properly
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-JSON-Schema", "enabled")
	}
}

// LLMWithTopP sets the top-p sampling parameter
func LLMWithTopP(topP float64) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		// Convert float to string for header storage
		config.RequestHeaders.Set("X-OpenAI-Responses-Top-P", fmt.Sprintf("%.3f", topP))
	}
}

// LLMWithTruncation sets the truncation strategy
func LLMWithTruncation(truncation string) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-Truncation", truncation)
	}
}

// LLMWithUser sets the user identifier for the request
func LLMWithUser(user string) llm.Option {
	return func(config *llm.Config) {
		if config.RequestHeaders == nil {
			config.RequestHeaders = make(http.Header)
		}
		config.RequestHeaders.Set("X-OpenAI-Responses-User", user)
	}
}
