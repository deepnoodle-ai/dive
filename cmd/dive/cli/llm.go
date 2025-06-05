package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/llm"
	"github.com/spf13/cobra"
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

var llmCmd = &cobra.Command{
	Use:   "llm [message]",
	Short: "Execute an LLM",
	Long:  "Execute an LLM with various configuration options",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// Get message from args or flag
		var message string
		if len(args) > 0 {
			message = args[0]
		} else {
			var err error
			message, err = cmd.Flags().GetString("message")
			if err != nil {
				fmt.Println(errorStyle.Sprint(err))
				os.Exit(1)
			}
		}

		if message == "" {
			fmt.Println(errorStyle.Sprint("No message provided. Use argument or --message flag"))
			os.Exit(1)
		}

		systemPrompt, err := cmd.Flags().GetString("system-prompt")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		var reasoningBudget int
		if value, err := cmd.Flags().GetInt("reasoning-budget"); err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		} else {
			reasoningBudget = value
		}

		reasoningEffort, err := cmd.Flags().GetString("reasoning-effort")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		llmProvider, err := cmd.Flags().GetString("provider")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		llmModel, err := cmd.Flags().GetString("model")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		model, err := config.GetModel(llmProvider, llmModel)
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		streaming, err := cmd.Flags().GetBool("stream")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		maxTokens, err := cmd.Flags().GetInt("max-tokens")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		temperature, err := cmd.Flags().GetFloat64("temperature")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		presencePenalty, err := cmd.Flags().GetFloat64("presence-penalty")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		frequencyPenalty, err := cmd.Flags().GetFloat64("frequency-penalty")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		apiKey, err := cmd.Flags().GetString("api-key")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		endpoint, err := cmd.Flags().GetString("endpoint")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		prefill, err := cmd.Flags().GetString("prefill")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		serviceTier, err := cmd.Flags().GetString("service-tier")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		previousResponseID, err := cmd.Flags().GetString("previous-response-id")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		writeEvents, err := cmd.Flags().GetString("write-events")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Get PDF URLs
		pdfURLsStr, err := cmd.Flags().GetString("pdfs")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		// Get image URLs
		imageURLsStr, err := cmd.Flags().GetString("images")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}

		var tools []llm.Tool
		toolsStr, err := cmd.Flags().GetString("tools")
		if err != nil {
			fmt.Println(errorStyle.Sprint(err))
			os.Exit(1)
		}
		if toolsStr != "" {
			toolNames := strings.Split(toolsStr, ",")
			for _, toolName := range toolNames {
				tool, err := config.InitializeToolByName(toolName, nil)
				if err != nil {
					fmt.Println(errorStyle.Sprintf("Failed to initialize tool: %s", err))
					os.Exit(1)
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
						// Handle as URL
						contentBlocks = append(contentBlocks, llm.NewDocumentContent(llm.ContentURL(pdfPath)))
					} else {
						// Handle as local file path
						data, err := os.ReadFile(pdfPath)
						if err != nil {
							fmt.Println(errorStyle.Sprintf("Failed to read PDF file %s: %v", pdfPath, err))
							os.Exit(1)
						}
						base64Data := base64.StdEncoding.EncodeToString(data)
						filename := filepath.Base(pdfPath)

						contentBlocks = append(contentBlocks, llm.NewDocumentContent(&llm.ContentSource{
							Type:      llm.ContentSourceTypeBase64,
							MediaType: "application/pdf",
							Data:      base64Data,
						}))
						// Set title for the document
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
						// Handle as URL
						contentBlocks = append(contentBlocks, &llm.ImageContent{
							Source: llm.ContentURL(imagePath),
						})
					} else {
						// Handle as local file path
						data, err := os.ReadFile(imagePath)
						if err != nil {
							fmt.Println(errorStyle.Sprintf("Failed to read image file %s: %v", imagePath, err))
							os.Exit(1)
						}
						base64Data := base64.StdEncoding.EncodeToString(data)

						// Detect media type based on file extension
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
				fmt.Println(errorStyle.Sprintf("Invalid reasoning effort: %s", reasoningEffort))
				os.Exit(1)
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
				fmt.Println(errorStyle.Sprint(err))
				os.Exit(1)
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
				fmt.Println(errorStyle.Sprint("Model does not support streaming"))
				os.Exit(1)
			}
			if _, err := streamLLM(ctx, streamingModel, options...); err != nil {
				fmt.Println(errorStyle.Sprint(err))
				os.Exit(1)
			}
		} else {
			response, err := generateLLM(ctx, model, options...)
			if err != nil {
				fmt.Println(errorStyle.Sprint(err))
				os.Exit(1)
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
	},
}

func init() {
	rootCmd.AddCommand(llmCmd)

	// Basic options
	llmCmd.Flags().StringP("message", "m", "", "Message to send to the LLM (alternative to positional argument)")
	llmCmd.Flags().StringP("model", "", "", "Model to use")
	llmCmd.Flags().StringP("system-prompt", "", "", "System prompt for the chat agent")
	llmCmd.Flags().BoolP("stream", "s", false, "Stream the response")

	// Content options
	llmCmd.Flags().StringP("pdfs", "", "", "Comma-separated list of PDF URLs or file paths to include as document content")
	llmCmd.Flags().StringP("images", "", "", "Comma-separated list of image URLs or file paths to include as image content")

	// LLM configuration options
	llmCmd.Flags().IntP("reasoning-budget", "", 0, "Reasoning budget for the chat agent")
	llmCmd.Flags().StringP("reasoning-effort", "", "", "Reasoning effort level (low, medium, high)")
	llmCmd.Flags().IntP("max-tokens", "", 0, "Maximum number of tokens to generate")
	llmCmd.Flags().Float64P("temperature", "", -1, "Temperature for randomness (0.0 to 2.0)")
	llmCmd.Flags().Float64P("presence-penalty", "", 0, "Presence penalty (-2.0 to 2.0)")
	llmCmd.Flags().Float64P("frequency-penalty", "", 0, "Frequency penalty (-2.0 to 2.0)")
	llmCmd.Flags().StringP("tools", "", "", "Tools to use (comma separated list of tool names)")

	// Provider options
	llmCmd.Flags().StringP("api-key", "", "", "API key for the LLM provider")
	llmCmd.Flags().StringP("endpoint", "", "", "Custom endpoint URL for the LLM provider")
	llmCmd.Flags().StringP("service-tier", "", "", "Service tier for the request")

	// Advanced options
	llmCmd.Flags().StringP("prefill", "", "", "Prefill text for assistant response")
	llmCmd.Flags().StringP("previous-response-id", "", "", "Previous response ID for continuation")

	llmCmd.Flags().StringP("write-events", "", "", "Write events to a file")
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
