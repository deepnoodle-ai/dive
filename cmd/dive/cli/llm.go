package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	wontoncli "github.com/deepnoodle-ai/wonton/cli"
)

func streamLLM(ctx context.Context, model llm.StreamingLLM, opts ...llm.Option) (*llm.Response, error) {
	stream, err := model.Stream(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating response: %v", err)
	}
	defer stream.Close()

	accum := llm.NewResponseAccumulator()
	printNewline := false

	for stream.Next() {
		event := stream.Event()
		accum.AddEvent(event)
		if printNewline {
			fmt.Println()
			printNewline = false
		}
		switch event.Type {
		case llm.EventTypeContentBlockStart:
			if event.ContentBlock.Type == llm.ContentTypeToolUse {
				fmt.Print(yellowStyle.Sprint(event.ContentBlock.ID + ": " + event.ContentBlock.Name + " "))
			}
		case llm.EventTypeContentBlockDelta:
			switch event.Delta.Type {
			case llm.EventDeltaTypeText:
				fmt.Print(successStyle.Sprint(event.Delta.Text))
			case llm.EventDeltaTypeInputJSON:
				fmt.Print(yellowStyle.Sprint(event.Delta.PartialJSON))
			case llm.EventDeltaTypeSignature:
				fmt.Print(thinkingStyle.Sprint(event.Delta.Signature))
			case llm.EventDeltaTypeThinking:
				fmt.Print(thinkingStyle.Sprint(event.Delta.Thinking))
			}
		case llm.EventTypeContentBlockStop:
			printNewline = true
		case llm.EventTypeMessageStop:
			printNewline = true
		}
	}

	if stream.Err() != nil {
		return nil, fmt.Errorf("error streaming response: %v", stream.Err())
	}
	return accum.Response(), nil
}

func generateLLM(ctx context.Context, model llm.LLM, opts ...llm.Option) (*llm.Response, error) {
	response, err := model.Generate(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating response: %v", err)
	}
	return response, nil
}

func registerLLMCommand(app *wontoncli.App) {
	app.Command("llm").
		Description("Execute an LLM with various configuration options").
		Args("message?").
		Flags(
			// Basic options
			wontoncli.String("message", "m").Help("Message to send to the LLM (alternative to positional argument)"),
			wontoncli.String("system-prompt", "").Help("System prompt for the chat agent"),
			wontoncli.Bool("stream", "s").Help("Stream the response"),

			// Content options
			wontoncli.String("pdfs", "").Help("Comma-separated list of PDF URLs or file paths to include as document content"),
			wontoncli.String("images", "").Help("Comma-separated list of image URLs or file paths to include as image content"),

			// LLM configuration options
			wontoncli.Int("reasoning-budget", "").Help("Reasoning budget for the chat agent"),
			wontoncli.String("reasoning-effort", "").Help("Reasoning effort level (low, medium, high)"),
			wontoncli.Int("max-tokens", "").Help("Maximum number of tokens to generate"),
			wontoncli.Float("temperature", "").Default(-1).Help("Temperature for randomness (0.0 to 2.0)"),
			wontoncli.Float("presence-penalty", "").Help("Presence penalty (-2.0 to 2.0)"),
			wontoncli.Float("frequency-penalty", "").Help("Frequency penalty (-2.0 to 2.0)"),
			wontoncli.String("tools", "").Help("Tools to use (comma separated list of tool names)"),

			// Provider options
			wontoncli.String("api-key", "").Help("API key for the LLM provider"),
			wontoncli.String("endpoint", "").Help("Custom endpoint URL for the LLM provider"),
			wontoncli.String("service-tier", "").Help("Service tier for the request"),

			// Advanced options
			wontoncli.String("prefill", "").Help("Prefill text for assistant response"),
			wontoncli.String("previous-response-id", "").Help("Previous response ID for continuation"),
			wontoncli.String("write-events", "").Help("Write events to a file"),
		).
		Run(func(ctx *wontoncli.Context) error {
			parseGlobalFlags(ctx)
			goCtx := context.Background()

			// Get message from args or flag
			var message string
			if ctx.NArg() > 0 {
				message = ctx.Arg(0)
			} else {
				message = ctx.String("message")
			}

			if message == "" {
				return wontoncli.Errorf("no message provided. Use argument or --message flag")
			}

			systemPrompt := ctx.String("system-prompt")
			reasoningBudget := ctx.Int("reasoning-budget")
			reasoningEffort := ctx.String("reasoning-effort")
			streaming := ctx.Bool("stream")
			maxTokens := ctx.Int("max-tokens")
			temperature := ctx.Float64("temperature")
			presencePenalty := ctx.Float64("presence-penalty")
			frequencyPenalty := ctx.Float64("frequency-penalty")
			apiKey := ctx.String("api-key")
			endpoint := ctx.String("endpoint")
			prefill := ctx.String("prefill")
			serviceTier := ctx.String("service-tier")
			previousResponseID := ctx.String("previous-response-id")
			writeEvents := ctx.String("write-events")
			pdfURLsStr := ctx.String("pdfs")
			imageURLsStr := ctx.String("images")

			model, err := config.GetModel(llmProvider, llmModel)
			if err != nil {
				return wontoncli.Errorf("%v", err)
			}

			var tools []llm.Tool
			toolsStr := ctx.String("tools")
			if toolsStr != "" {
				toolNames := strings.Split(toolsStr, ",")
				for _, toolName := range toolNames {
					tool, err := config.InitializeToolByName(toolName, nil)
					if err != nil {
						return wontoncli.Errorf("failed to initialize tool: %s", err)
					}
					tools = append(tools, tool)
				}
			}

			// Build content blocks for the user message
			var contentBlocks []llm.Content

			// Add text content block
			contentBlocks = append(contentBlocks, llm.NewTextContent(message))

			// Add PDF content blocks
			if pdfURLsStr != "" {
				pdfURLs := strings.Split(pdfURLsStr, ",")
				for _, pdfPath := range pdfURLs {
					pdfPath = strings.TrimSpace(pdfPath)
					if pdfPath != "" {
						if strings.HasPrefix(pdfPath, "http") {
							contentBlocks = append(contentBlocks, llm.NewDocumentContent(llm.ContentURL(pdfPath)))
						} else {
							data, err := os.ReadFile(pdfPath)
							if err != nil {
								return wontoncli.Errorf("failed to read PDF file %s: %v", pdfPath, err)
							}
							base64Data := base64.StdEncoding.EncodeToString(data)
							filename := filepath.Base(pdfPath)

							contentBlocks = append(contentBlocks, llm.NewDocumentContent(&llm.ContentSource{
								Type:      llm.ContentSourceTypeBase64,
								MediaType: "application/pdf",
								Data:      base64Data,
							}))
							if len(contentBlocks) > 0 {
								if docContent, ok := contentBlocks[len(contentBlocks)-1].(*llm.DocumentContent); ok {
									docContent.Title = filename
								}
							}
						}
					}
				}
			}

			// Add image content blocks
			if imageURLsStr != "" {
				imageURLs := strings.Split(imageURLsStr, ",")
				for _, imagePath := range imageURLs {
					imagePath = strings.TrimSpace(imagePath)
					if imagePath != "" {
						if strings.HasPrefix(imagePath, "http") {
							contentBlocks = append(contentBlocks, &llm.ImageContent{
								Source: llm.ContentURL(imagePath),
							})
						} else {
							data, err := os.ReadFile(imagePath)
							if err != nil {
								return wontoncli.Errorf("failed to read image file %s: %v", imagePath, err)
							}
							base64Data := base64.StdEncoding.EncodeToString(data)
							mediaType := detectImageMediaType(imagePath)

							contentBlocks = append(contentBlocks, &llm.ImageContent{
								Source: &llm.ContentSource{
									Type:      llm.ContentSourceTypeBase64,
									MediaType: mediaType,
									Data:      base64Data,
								},
							})
						}
					}
				}
			}

			var options []llm.Option

			// Add user message with content blocks
			userMessage := llm.NewUserMessage(contentBlocks...)
			options = append(options, llm.WithMessages(userMessage))

			// Add conditional options
			if len(tools) > 0 {
				options = append(options, llm.WithTools(tools...))
			}
			if systemPrompt != "" {
				options = append(options, llm.WithSystemPrompt(systemPrompt))
			}
			if reasoningBudget > 0 {
				options = append(options, llm.WithReasoningBudget(reasoningBudget))
			}
			if reasoningEffort != "" {
				effort := llm.ReasoningEffort(reasoningEffort)
				if !effort.IsValid() {
					return wontoncli.Errorf("invalid reasoning effort: %s", reasoningEffort)
				}
				options = append(options, llm.WithReasoningEffort(effort))
			}
			if maxTokens > 0 {
				options = append(options, llm.WithMaxTokens(maxTokens))
			}
			if temperature >= 0 {
				options = append(options, llm.WithTemperature(temperature))
			}
			if presencePenalty != 0 {
				options = append(options, llm.WithPresencePenalty(presencePenalty))
			}
			if frequencyPenalty != 0 {
				options = append(options, llm.WithFrequencyPenalty(frequencyPenalty))
			}
			if apiKey != "" {
				options = append(options, llm.WithAPIKey(apiKey))
			}
			if endpoint != "" {
				options = append(options, llm.WithEndpoint(endpoint))
			}
			if prefill != "" {
				options = append(options, llm.WithPrefill(prefill, ""))
			}
			if serviceTier != "" {
				options = append(options, llm.WithServiceTier(serviceTier))
			}
			if previousResponseID != "" {
				options = append(options, llm.WithPreviousResponseID(previousResponseID))
			}
			if writeEvents != "" {
				f, err := os.Create(writeEvents)
				if err != nil {
					return wontoncli.Errorf("%v", err)
				}
				defer f.Close()
				options = append(options, llm.WithServerSentEventsCallback(func(line string) error {
					if _, err := f.WriteString(line); err != nil {
						return err
					}
					return nil
				}))
			}

			if streaming {
				streamingModel, ok := model.(llm.StreamingLLM)
				if !ok {
					return wontoncli.Errorf("model does not support streaming")
				}
				if _, err := streamLLM(goCtx, streamingModel, options...); err != nil {
					return wontoncli.Errorf("%v", err)
				}
			} else {
				response, err := generateLLM(goCtx, model, options...)
				if err != nil {
					return wontoncli.Errorf("%v", err)
				}
				for _, content := range response.Content {
					switch content := content.(type) {
					case *llm.TextContent:
						fmt.Println(content.Text)
					case *llm.ToolUseContent:
						fmt.Println(content.Name)
						fmt.Println(string(content.Input))
					}
				}
			}
			return nil
		})
}

// detectImageMediaType returns the media type based on file extension
func detectImageMediaType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	default:
		return "image/jpeg" // Default fallback
	}
}
