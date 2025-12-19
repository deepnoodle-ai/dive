# Dive CLI Example Programs

This directory contains standalone example programs that demonstrate various AI capabilities using the Dive library. These were previously subcommands of the main `dive` CLI but have been moved to examples to keep the main CLI focused on interactive chat.

## Available Examples

### Core LLM Interaction

- **ask_cli** - Ask an agent a single question
  ```bash
  go run ./examples/programs/ask_cli "What is the capital of France?"
  ```

- **llm_cli** - Direct LLM interaction with streaming support
  ```bash
  go run ./examples/programs/llm_cli "Tell me a story" --stream
  ```

### Text Processing

- **summarize_cli** - Summarize text from stdin
  ```bash
  cat document.txt | go run ./examples/programs/summarize_cli --length short
  ```

- **classify_cli** - Classify text into categories
  ```bash
  go run ./examples/programs/classify_cli --text "This is great!" --labels "positive,negative,neutral"
  ```

- **embed_cli** - Generate text embeddings
  ```bash
  echo "Hello, world!" | go run ./examples/programs/embed_cli
  ```

### Data Extraction

- **extract_cli** - Extract structured data from text
  ```bash
  echo "John is 25 years old" | go run ./examples/programs/extract_cli --fields "name,age:int"
  ```

### Utilities

- **diff_cli** - AI-powered semantic diff between files
  ```bash
  go run ./examples/programs/diff_cli file1.txt file2.txt
  ```

- **compare_cli** - Compare responses from multiple LLM providers
  ```bash
  go run ./examples/programs/compare_cli --prompt "Hello" --providers "anthropic,openai"
  ```

- **watch_cli** - Monitor files and trigger AI actions on changes
  ```bash
  go run ./examples/programs/watch_cli --path . --on-change "Summarize the changes"
  ```

### Generation

- **image_cli** - Generate images using DALL-E
  ```bash
  go run ./examples/programs/image_cli --prompt "A sunset over mountains" --output image.png
  ```

- **video_cli** - Generate videos using Google Veo
  ```bash
  go run ./examples/programs/video_cli --prompt "A cat playing piano"
  ```

## Building Examples

You can build any example as a standalone binary:

```bash
go build -o my-tool ./examples/programs/llm_cli
./my-tool "What is 2+2?"
```

## Main Dive CLI

The main `dive` CLI is now focused on interactive chat:

```bash
# Start an interactive chat session
dive

# Or explicitly use the chat command
dive chat

# Use a specific configuration
dive --config ./dive.yaml --agent "my-agent"

# Manage threads
dive threads list
dive threads show <thread-id>

# Work with MCP servers
dive mcp auth
dive mcp token-status

# Validate configuration
dive config check
```

Run `dive --help` for more information about the main CLI.
