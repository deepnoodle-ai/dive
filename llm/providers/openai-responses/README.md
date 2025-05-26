# OpenAI Responses API Provider

This provider implements support for OpenAI's new Responses API, which offers advanced features like built-in web search, image generation, and MCP server integration.

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
        SearchContextSize: "medium", // "small", "medium", "large"
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

Connect to remote MCP servers for extended functionality:

```go
provider := openairesponses.New(
    openairesponses.WithModel("gpt-4.1"),
    openairesponses.WithMCPServer("deepwiki", "https://mcp.deepwiki.com/mcp", nil),
    openairesponses.WithMCPServerOptions("stripe", openairesponses.MCPServerConfig{
        ServerURL: "https://mcp.stripe.com",
        Headers: map[string]string{
            "Authorization": "Bearer " + os.Getenv("STRIPE_API_KEY"),
        },
        RequireApproval: "never",
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