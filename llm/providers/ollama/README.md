# Ollama Provider

The Ollama provider enables integration with [Ollama](https://ollama.ai/), a tool for running large language models locally. This implementation leverages Ollama's OpenAI-compatible API for seamless integration.

## Features

- **OpenAI-Compatible API**: Uses Ollama's `/v1/chat/completions` endpoint
- **Streaming Support**: Full streaming capabilities for real-time responses
- **Tool Calling**: Support for function calling (when supported by the model)
- **Local Deployment**: Run models locally without external API dependencies
- **Model Flexibility**: Support for various model sizes and types

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

    "github.com/diveagents/dive/llm"
    "github.com/diveagents/dive/llm/providers/ollama"
)

func main() {
    provider := ollama.New(
        ollama.WithModel(ollama.ModelLlama32),
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

```go
provider := ollama.New(
    ollama.WithModel("llama3.2"),                                    // Model name
    ollama.WithEndpoint("http://localhost:11434/v1/chat/completions"), // Custom endpoint
    ollama.WithAPIKey("ollama"),                                     // API key (usually not needed)
    ollama.WithMaxTokens(4096),                                      // Max tokens
)
```

### Available Models

The provider includes constants for common models:

```go
ollama.ModelLlama32     // "llama3.2"
ollama.ModelLlama31     // "llama3.1" 
ollama.ModelLlama3      // "llama3"
ollama.ModelCodeLlama   // "codellama"
ollama.ModelMistral     // "mistral"
ollama.ModelGemma       // "gemma"
ollama.ModelLlava       // "llava" (vision)
ollama.ModelDeepSeek    // "deepseek-coder"
```

Specific size variants are also available (e.g., `ollama.ModelLlama32_3B`, `ollama.ModelMistral_7B`).

## Environment Variables

- `OLLAMA_API_KEY`: API key (optional for local instances)

## Implementation Details

This provider is built on top of the OpenAI provider, leveraging Ollama's OpenAI-compatible API. This approach provides:

- **Consistency**: Same interface as other providers
- **Reliability**: Leverages well-tested OpenAI provider code
- **Maintainability**: Minimal custom code to maintain
- **Feature Parity**: Automatic support for new OpenAI API features

## Troubleshooting

### Connection Issues
- Ensure Ollama is running: `ollama serve`
- Check the endpoint URL (default: `http://localhost:11434/v1/chat/completions`)
- Verify the model is pulled: `ollama list`

### Model Not Found
- Pull the model: `ollama pull <model-name>`
- Check available models: `ollama list`

### Performance
- Consider using smaller models for faster responses
- Adjust `max_tokens` based on your needs
- Use streaming for long responses 