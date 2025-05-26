# OpenAI Responses API Provider

This package provides a Dive LLM provider for the OpenAI Responses API, which offers advanced features like web search, image generation, and MCP server integration.

## Key Improvements

The provider has been refactored to properly separate **provider-creation-time options** from **generation-time options**, following the same pattern as other Dive LLM providers.

### Provider-Creation-Time Options

These options are set once when creating the provider and cannot be changed per request:

```go
provider := openairesponses.New(
    openairesponses.WithAPIKey("your-api-key"),
    openairesponses.WithModel("gpt-4.1"),
    openairesponses.WithEndpoint("https://api.openai.com/v1/responses"),
    openairesponses.WithClient(customHTTPClient),
)
```

### Generation-Time Options

These options can be configured differently for each request using `llm.Option` functions:

#### Basic Options
```go
response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Your message"),
    llm.WithTemperature(0.7),
    llm.WithMaxTokens(1000),
    // OpenAI Responses specific options:
    openairesponses.LLMWithStore(true),
    openairesponses.LLMWithBackground(false),
    openairesponses.LLMWithInstructions("Custom instructions"),
    openairesponses.LLMWithServiceTier("premium"),
    openairesponses.LLMWithUser("user-123"),
)
```

#### Web Search
```go
response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("What are the latest AI developments?"),
    // Enable web search with default options
    openairesponses.LLMWithWebSearch(),
    // Or with custom options
    openairesponses.LLMWithWebSearchOptions(openairesponses.WebSearchOptions{
        Domains:           []string{"arxiv.org", "openai.com"},
        SearchContextSize: "medium",
        UserLocation: &openairesponses.UserLocation{
            Type:    "approximate",
            Country: "US",
        },
    }),
)
```

#### Image Generation
```go
response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Generate an image of a sunset"),
    // Enable image generation with default options
    openairesponses.LLMWithImageGeneration(),
    // Or with custom options
    openairesponses.LLMWithImageGenerationOptions(openairesponses.ImageGenerationOptions{
        Size:       "1024x1024",
        Quality:    "high",
        Background: "auto",
    }),
)
```

#### JSON Schema Output
```go
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "name": map[string]interface{}{"type": "string"},
        "age":  map[string]interface{}{"type": "integer"},
    },
    "required": []string{"name", "age"},
}

response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Generate person data"),
    openairesponses.LLMWithJSONSchema(schema),
)
```

#### MCP Server Integration
```go
response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Query the database"),
    openairesponses.LLMWithMCPServer("database", "http://localhost:8080/mcp", map[string]string{
        "Authorization": "Bearer token123",
    }),
)
```

#### Advanced Options
```go
response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Complex reasoning task"),
    openairesponses.LLMWithReasoningEffort("high"),
    openairesponses.LLMWithTopP(0.9),
    openairesponses.LLMWithTruncation("auto"),
)
```

## Benefits of This Approach

1. **Flexibility**: Tools and options can be configured differently for each request
2. **Consistency**: Follows the same pattern as other Dive LLM providers
3. **Composability**: Multiple tools can be enabled for a single request
4. **Maintainability**: Clear separation between provider-level and request-level configuration

## Migration from Previous Version

If you were using the old provider-creation-time tool options:

```go
// OLD - Don't do this anymore
provider := openairesponses.New(
    openairesponses.WithWebSearch(true),
    openairesponses.WithImageGeneration(true),
)
```

Change to generation-time options:

```go
// NEW - Do this instead
provider := openairesponses.New(
    openairesponses.WithAPIKey("your-api-key"),
    openairesponses.WithModel("gpt-4.1"),
)

response, err := provider.Generate(ctx,
    llm.WithUserTextMessage("Your message"),
    openairesponses.LLMWithWebSearch(),
    openairesponses.LLMWithImageGeneration(),
)
```

## Complete Example

See `examples/programs/openai_responses_example/main.go` for a comprehensive example demonstrating all features.

## Features

- **Text Generation**: Standard chat completions with the latest OpenAI models
- **Web Search**: Built-in web search capabilities with configurable options
- **Image Generation**: Generate images using the GPT Image model
- **MCP Integration**: Connect to remote MCP servers for extended functionality
- **Streaming**: Full support for streaming responses
- **Tool Calling**: Support for custom function tools alongside built-in tools

## Usage

### Basic Text Generation

```go
import openairesponses "github.com/diveagents/dive/llm/providers/openai-responses"

provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
)

response, err := provider.Generate(context.Background(),
    llm.WithUserTextMessage("Hello, world!"),
)
```

### Web Search

Enable web search to allow the model to search the internet for current information:

```go
provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
    openairesponses.WithWebSearchOptions(openairesponses.WebSearchOptions{
        SearchContextSize: "medium", // "low", "medium", "high"
        UserLocation: &openairesponses.UserLocation{
            Type:    "approximate",
            Country: "US",
        },
    }),
)

response, err := provider.Generate(context.Background(),
    llm.WithUserTextMessage("What are the latest AI developments?"),
)
```

### Image Generation

Generate images directly within conversations:

```go
provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
    openairesponses.WithImageGenerationOptions(openairesponses.ImageGenerationOptions{
        Size:    "1024x1024",
        Quality: "high",
        // Format field is not supported by the OpenAI Responses API
    }),
)

response, err := provider.Generate(context.Background(),
    llm.WithUserTextMessage("Generate an image of a sunset over mountains."),
)
```

### MCP Server Integration

Connect to remote MCP servers for extended functionality. The provider supports the full OpenAI Responses API MCP feature set including:

- Remote MCP server connections
- Tool filtering with `allowed_tools`
- Approval workflows for security
- Authentication via headers
- Streaming support for MCP events

#### Basic MCP Usage

```go
provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
)

response, err := provider.Generate(context.Background(),
    llm.WithUserTextMessage("What transport protocols are supported in the 2025-03-26 version of the MCP spec?"),
    llm.WithMCPServers(llm.MCPServerConfig{
        Type: "url",
        Name: "deepwiki",
        URL:  "https://mcp.deepwiki.com/mcp",
        ToolConfiguration: &llm.MCPToolConfiguration{
            Enabled:      true,
            AllowedTools: []string{"ask_question"}, // Filter to specific tools
        },
    }),
)
```

#### Authenticated MCP Servers

```go
response, err := provider.Generate(context.Background(),
    llm.WithUserTextMessage("Create a payment link for $20"),
    llm.WithMCPServers(llm.MCPServerConfig{
        Type:               "url",
        Name:               "stripe",
        URL:                "https://mcp.stripe.com",
        AuthorizationToken: os.Getenv("STRIPE_API_KEY"),
        ToolConfiguration: &llm.MCPToolConfiguration{
            Enabled: true,
        },
    }),
)
```

#### MCP Approval Workflow

By default, MCP tool calls require approval for security. You can handle approval requests:

```go
// First request that triggers an approval request
response1, err := provider.Generate(ctx, 
    llm.WithUserTextMessage("Query the database"),
    llm.WithMCPServers(/* MCP config */),
)

// Check for approval requests in the response
for _, content := range response1.Content {
    if textContent, ok := content.(*llm.TextContent); ok {
        if strings.Contains(textContent.Text, "MCP approval required") {
            // Handle approval - in practice, you'd extract the approval request ID
            approvalRequestID := "mcpr_example_id"
            
            // Approve the request
            response2, err := provider.Generate(ctx,
                openairesponses.LLMWithMCPApprovalResponse(approvalRequestID, true),
            )
        }
    }
}
```

#### Provider-Level MCP Configuration

You can also configure MCP servers at the provider level:

```go
provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
    openairesponses.WithMCPServerOptions("deepwiki", openairesponses.MCPServerConfig{
        ServerURL: "https://mcp.deepwiki.com/mcp",
        RequireApproval: "never", // Skip approvals for trusted servers
    }),
)
```

### Streaming

Stream responses for real-time interaction:

```go
stream, err := provider.Stream(context.Background(),
    llm.WithUserTextMessage("Tell me a story..."),
)
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for stream.Next() {
    event := stream.Event()
    // Process streaming events
}
```

## Configuration Options

### Provider Options

- `WithAPIKey(string)`: Set OpenAI API key (defaults to `OPENAI_API_KEY` env var)
- `WithEndpoint(string)`: Set custom endpoint (defaults to OpenAI Responses API)
- `WithModel(string)`: Set model (defaults to "gpt-4.1")
- `WithStore(bool)`: Enable/disable response storage
- `WithBackground(bool)`: Run requests in background

### Built-in Tools

#### Web Search
- `WithWebSearch(bool)`: Enable basic web search
- `WithWebSearchOptions(WebSearchOptions)`: Configure web search behavior

#### Image Generation
- `WithImageGeneration(bool)`: Enable basic image generation
- `WithImageGenerationOptions(ImageGenerationOptions)`: Configure image generation

#### MCP Servers
- `WithMCPServer(label, url, headers)`: Add an MCP server
- `WithMCPServerOptions(label, config)`: Add MCP server with full configuration

## Differences from Standard OpenAI Provider

The Responses API provider differs from the standard OpenAI Chat Completions provider in several ways:

1. **Input Format**: Uses the Responses API's input format instead of messages array
2. **Built-in Tools**: Supports OpenAI's built-in tools (web search, image generation, MCP)
3. **Response Structure**: Handles the Responses API's output format with multiple item types
4. **Advanced Features**: Supports conversation state, approval workflows, and more

## Model Support

The following models are supported:
- `gpt-4.1` (default)
- `gpt-4.1-mini`
- `gpt-4o`
- `gpt-4o-mini`
- `o3` (for reasoning tasks)

## Environment Variables

- `OPENAI_API_KEY`: Your OpenAI API key (required)

## Examples

See the [examples/programs/openai_responses_example](../../../examples/programs/openai_responses_example/) directory for complete working examples. 