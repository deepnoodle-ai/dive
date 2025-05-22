# Combined Search Example

This example demonstrates how to use the Dive framework to create a research agent that can use either Google Search or Kagi as a search provider, with a configurable LLM model (Anthropic, OpenAI, or Azure OpenAI).

## Prerequisites

Depending on which search provider you want to use, you'll need to set different environment variables:

### For Search Providers
#### Google Search
- `GOOGLE_SEARCH_CX`: Your Google Custom Search Engine ID
- `GOOGLE_SEARCH_API_KEY`: Your Google API Key
  - See [Google Custom Search JSON API](https://developers.google.com/custom-search/v1/introduction) for more information

#### Kagi Search
- `KAGI_API_KEY`: Your Kagi API Key
  - Note: Kagi search API is currently in private beta
  - See [Kagi API documentation](https://help.kagi.com/kagi/api/search.html) to request an invite

### For LLM Models
#### Anthropic
- `ANTHROPIC_API_KEY`: Your Anthropic API key

#### OpenAI
- `OPENAI_API_KEY`: Your OpenAI API key

#### Azure OpenAI
- `OPENAI_ENDPOINT`: Your Azure OpenAI endpoint
  - Format: `https://YOUR_RESOURCE_NAME.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT_NAME/chat/completions?api-version=2024-06-01`
  - See [Azure OpenAI Reference](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference)
- `OPENAI_API_KEY`: Your Azure OpenAI API key

## Usage

Run the example with the desired search provider:

```bash
# Use Google Search with Anthropic (default)
go run main.go

# Use Kagi Search
go run main.go -provider kagi

# Use Google Search with OpenAI
go run main.go -model openai

# Use Kagi Search with Azure OpenAI
go run main.go -provider kagi -model azure

# Specify a custom prompt
go run main.go -prompt "Research quantum computing advances in 2023"

# Change log level
go run main.go -log info
```

## Command-line Arguments

- `-provider`: Search provider to use (`google` or `kagi`, default: `google`)
- `-model`: LLM model provider to use (`anthropic`, `openai`, or `azure`, default: `anthropic`)
- `-log`: Log level (default: value of `LOG_LEVEL` env var or `debug`)
- `-prompt`: Research prompt (default: "Research the history of computing. Respond with a brief markdown-formatted report.")

## Example Output

The agent will research the given topic using the specified search engine and generate a markdown-formatted report.