# OpenAI Responses PDF Example

This example demonstrates how to use PDF files as input with the OpenAI Responses API.

## Usage

### Using a local PDF file

```bash
go run main.go -pdf path/to/your/document.pdf -prompt "What is the main topic of this document?"
```

### Using an OpenAI file ID

First, upload your file to OpenAI and get the file ID, then:

```bash
go run main.go -file-id file-abc123 -prompt "Summarize the key points in this document"
```

## Features

- Supports both base64-encoded PDF data and OpenAI file IDs
- Automatically handles file encoding and data URI formatting
- Customizable prompts for different analysis tasks

## Requirements

- OpenAI API key set in `OPENAI_API_KEY` environment variable
- Access to OpenAI's Responses API (currently in beta)

## Example Prompts

- "What is this document about? Provide a brief summary."
- "Extract the key findings from this research paper."
- "List the main sections and their topics."
- "What are the conclusions or recommendations?" 