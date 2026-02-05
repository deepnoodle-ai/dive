# Ollama Provider

The Ollama provider enables integration with [Ollama](https://ollama.ai/), a tool for running large language models locally. This implementation provides a seamless bridge between Dive's LLM interface and Ollama's local model execution environment.

## Architecture & Design Philosophy

This provider takes a **wrapper-based approach**, leveraging Ollama's OpenAI-compatible API endpoint (`/v1/chat/completions`) rather than implementing a custom client from scratch. Here's why:

- **Zero Maintenance Overhead**: By wrapping the battle-tested OpenAI provider, we inherit all its features, bug fixes, and optimizations automatically
- **API Consistency**: Provides identical interface and behavior to other Dive LLM providers
- **Future-Proof**: Automatically gains support for new OpenAI API features as they're added to the base provider
- **Local Privacy**: Keeps model execution entirely local while maintaining cloud-provider API compatibility

The provider works by configuring the OpenAI client with Ollama's local endpoint and handling the minimal differences (like API key requirements).

## Features

- **Local Model Execution**: Run models entirely on your machine without external API calls
- **OpenAI API Compatibility**: Full feature parity with cloud providers through Ollama's compatible endpoint
- **Streaming Support**: Real-time response streaming for interactive applications
- **Tool Calling**: Function calling support (when available in the model)
- **Model Flexibility**: Support for various model families and sizes from the Ollama ecosystem
- **Zero Configuration**: Works out-of-the-box with sensible defaults

## Setup

1. **Install Ollama**: Follow the [official installation guide](https://ollama.ai/)

2. **Pull a Model**: Download a model to use locally

   ```bash
   ollama pull llama3.2
   ```

3. **Start Ollama**: Ensure Ollama is running (usually starts automatically)
   ```bash
   ollama serve
   ```

## Usage

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive/llm"
    "github.com/deepnoodle-ai/dive/llm/providers/ollama"
)

func main() {
    provider := ollama.New(
        ollama.WithModel(ollama.ModelLlama32_3B),
        ollama.WithMaxTokens(2048),
    )

    response, err := provider.Generate(context.Background(),
        llm.WithUserTextMessage("Hello, how are you?"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.Message().Text())
}
```

### Configuration Options

The provider supports all standard configuration options:

```go
provider := ollama.New(
    ollama.WithModel("llama3.2:3b"),                                 // Model name with optional size
    ollama.WithEndpoint("http://localhost:11434/v1/chat/completions"), // Custom endpoint
    ollama.WithAPIKey("custom-key"),                                 // API key (defaults to "ollama")
    ollama.WithMaxTokens(4096),                                      // Max response tokens
    ollama.WithClient(customHTTPClient),                             // Custom HTTP client
)
```

### Available Models

The provider includes constants for popular model families. Models are specified with size variants:

```go
// Llama 3.2 family
ollama.ModelLlama32_1B   // "llama3.2:1b"
ollama.ModelLlama32_3B   // "llama3.2:3b"
ollama.ModelLlama32_11B  // "llama3.2:11b"
ollama.ModelLlama32_90B  // "llama3.2:90b"

// Llama 3.1 family
ollama.ModelLlama31_8B   // "llama3.1:8b"
ollama.ModelLlama31_70B  // "llama3.1:70b"

// Other popular models
ollama.ModelCodeLlama_7B  // "codellama:7b"
ollama.ModelMistral_7B    // "mistral:7b"
ollama.ModelGemma2_2B     // "gemma2:2b"
ollama.ModelQwen_7B       // "qwen:7b"
ollama.ModelPhi3_Mini     // "phi3:mini"
```

You can also use any model string directly that matches your locally pulled models.

## Environment Variables

- `OLLAMA_API_KEY`: Optional API key (defaults to "ollama" for local instances)

## Implementation Details

This provider is implemented as a **thin wrapper around the OpenAI provider**, taking advantage of Ollama's OpenAI-compatible API. The architecture:

1. **Provider Creation**: Accepts Ollama-specific options and converts them to OpenAI provider configuration
2. **Endpoint Mapping**: Routes requests to Ollama's local `/v1/chat/completions` endpoint
3. **API Key Handling**: Provides a default "ollama" key since local instances don't require authentication
4. **Feature Inheritance**: Automatically supports all OpenAI provider features (streaming, tools, etc.)

This approach provides several key benefits:

- **Minimal Code Surface**: Less code to maintain and debug
- **Automatic Updates**: Inherits improvements made to the OpenAI provider
- **Behavioral Consistency**: Identical response handling across all providers
- **Testing Leverage**: Benefits from extensive OpenAI provider test coverage

## Troubleshooting

### Connection Issues

- Ensure Ollama is running: `ollama serve`
- Check the endpoint URL (default: `http://localhost:11434/v1/chat/completions`)
- Verify Ollama is listening on the expected port: `curl http://localhost:11434/api/version`

### Model Not Found

- Pull the model: `ollama pull <model-name>`
- List available models: `ollama list`
- Use exact model names including size variants (e.g., `llama3.2:3b` not `llama3.2`)

### Performance Optimization

- Choose appropriate model sizes for your hardware (smaller models = faster responses)
- Adjust `max_tokens` based on response length needs
- Use streaming for long responses to improve perceived performance
- Consider model quantization options available in Ollama
