package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
)

var (
	DefaultModel     = openai.ChatModelGPT4
	DefaultMaxTokens = 4096
)

var _ llm.LLM = &Provider{}

type Provider struct {
	client    openai.Client
	model     openai.ChatModel
	maxTokens int
	options   []option.RequestOption
}

func New(opts ...Option) *Provider {
	p := &Provider{
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.client = openai.NewClient(p.options...)
	return p
}

func (p *Provider) Name() string {
	return "openai"
}

func (p *Provider) ModelName() string {
	return string(p.model)
}

func (p *Provider) buildConfig(opts ...llm.Option) *llm.Config {
	config := &llm.Config{}
	config.Apply(opts...)
	return config
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := p.buildConfig(opts...)

	params, err := p.buildRequestParams(config)
	if err != nil {
		return nil, err
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
		},
	}); err != nil {
		return nil, err
	}

	response, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	return p.convertResponse(response)
}

// buildRequestParams converts llm.Config to responses.ResponseNewParams
func (p *Provider) buildRequestParams(config *llm.Config) (responses.ResponseNewParams, error) {
	if len(config.Messages) == 0 {
		return responses.ResponseNewParams{}, fmt.Errorf("no messages provided")
	}

	// Convert messages to input format
	input, err := p.convertMessagesToInput(config.Messages)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}

	params := responses.ResponseNewParams{
		Model: p.model,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
	}

	if config.SystemPrompt != "" {
		params.Instructions = openai.String(config.SystemPrompt)
	}

	// Set max tokens
	if config.MaxTokens != nil && *config.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(*config.MaxTokens))
	} else if p.maxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(p.maxTokens))
	}

	// Set temperature
	if config.Temperature != nil {
		params.Temperature = openai.Float(*config.Temperature)
	}

	// Handle reasoning effort
	if config.ReasoningEffort != "" {
		params.Reasoning = responses.ReasoningParam{
			Effort: responses.ReasoningEffort(config.ReasoningEffort),
		}
	}

	// Handle parallel tool calls
	if config.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*config.ParallelToolCalls)
	}

	// Handle previous response ID
	if config.PreviousResponseID != "" {
		params.PreviousResponseID = openai.String(config.PreviousResponseID)
	}

	// Handle service tier
	if config.ServiceTier != "" {
		switch config.ServiceTier {
		case "auto":
			params.ServiceTier = responses.ResponseNewParamsServiceTierAuto
		case "default":
			params.ServiceTier = responses.ResponseNewParamsServiceTierDefault
		case "flex":
			params.ServiceTier = responses.ResponseNewParamsServiceTierFlex
		default:
			return responses.ResponseNewParams{}, fmt.Errorf("invalid service tier: %s", config.ServiceTier)
		}
	}

	// Handle tool choice
	if config.ToolChoice != "" {
		switch string(config.ToolChoice) {
		case "auto":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
			}
		case "none":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsNone),
			}
		case "required", "any":
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsRequired),
			}
		default:
			return responses.ResponseNewParams{}, fmt.Errorf("invalid tool choice: %s", config.ToolChoice)
		}
	}

	// Convert tools
	if len(config.Tools) > 0 {
		var tools []responses.ToolUnionParam
		for _, tool := range config.Tools {
			// Handle tools that explicitly provide a configuration
			if toolWithConfig, ok := tool.(llm.ToolConfiguration); ok {
				toolConfig := toolWithConfig.ToolConfiguration(p.Name())
				if toolConfig != nil {
					tools = append(tools, responses.ToolUnionParam{
						OfFunction: &responses.FunctionToolParam{
							Name:        tool.Name(),
							Strict:      openai.Bool(false),
							Description: openai.String(tool.Description()),
							Parameters:  toolConfig,
						},
					})
					continue
				}
			}
			// Default tool handling
			tools = append(tools, responses.ToolUnionParam{
				OfFunction: &responses.FunctionToolParam{
					Name:        tool.Name(),
					Strict:      openai.Bool(false),
					Description: openai.String(tool.Description()),
					Parameters:  tool.Schema().AsMap(),
				},
			})
		}
		params.Tools = tools
	}

	// Handle MCP servers
	for _, mcpServer := range config.MCPServers {
		mcpParam := &responses.ToolMcpParam{
			ServerLabel: mcpServer.Name,
			ServerURL:   mcpServer.URL,
		}
		tool := responses.ToolUnionParam{OfMcp: mcpParam}

		if mcpServer.ToolConfiguration != nil {
			if len(mcpServer.ToolConfiguration.AllowedTools) > 0 {
				mcpParam.AllowedTools = responses.ToolMcpAllowedToolsUnionParam{
					OfMcpAllowedTools: mcpServer.ToolConfiguration.AllowedTools,
				}
			}
		}

		if mcpServer.AuthorizationToken != "" || len(mcpServer.Headers) > 0 {
			headers := make(map[string]string)
			if mcpServer.AuthorizationToken != "" {
				headers["Authorization"] = "Bearer " + mcpServer.AuthorizationToken
			}
			for key, value := range mcpServer.Headers {
				headers[key] = value
			}
			mcpParam.Headers = headers
		}

		if mcpServer.ToolApproval != "" && mcpServer.ToolApprovalFilter != nil {
			return responses.ResponseNewParams{}, fmt.Errorf("tool approval and tool approval filter cannot be used together")
		}

		if mcpServer.ToolApproval != "" {
			mcpParam.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
				OfMcpToolApprovalSetting: openai.String(mcpServer.ToolApproval),
			}
		} else if mcpServer.ToolApprovalFilter != nil {
			mcpParam.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
				OfMcpToolApprovalFilter: &responses.ToolMcpRequireApprovalMcpToolApprovalFilterParam{
					Always: responses.ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam{
						ToolNames: mcpServer.ToolApprovalFilter.Always,
					},
					Never: responses.ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam{
						ToolNames: mcpServer.ToolApprovalFilter.Never,
					},
				},
			}
		}
		params.Tools = append(params.Tools, tool)
	}
	return params, nil
}

// convertMessagesToInput converts llm.Message slice to SDK input format
func (p *Provider) convertMessagesToInput(messages []*llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	var inputItems []responses.ResponseInputItemUnionParam

	for _, msg := range messages {
		var contentItems []responses.ResponseInputContentUnionParam

		for _, content := range msg.Content {
			switch c := content.(type) {
			case *llm.TextContent:
				contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: c.Text,
					},
				})

			case *llm.RefusalContent:
				// Unclear if this is the correct way to handle refusals.
				// OpenAI does not support refusals in the input?
				contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: c.Text,
					},
				})

			case *llm.ImageContent:
				if c.Source != nil {
					inputImage := &responses.ResponseInputImageParam{
						Detail: responses.ResponseInputImageDetailAuto,
					}
					switch c.Source.Type {
					case llm.ContentSourceTypeBase64:
						if c.Source.MediaType == "" || c.Source.Data == "" {
							return nil, fmt.Errorf("media type and data are required for base64 image content")
						}
						// Create data URL for base64 image data
						dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
						inputImage.ImageURL = openai.String(dataURL)

					case llm.ContentSourceTypeURL:
						if c.Source.URL == "" {
							return nil, fmt.Errorf("URL is required for URL-based image content")
						}
						inputImage.ImageURL = openai.String(c.Source.URL)

					case llm.ContentSourceTypeFile:
						if c.Source.FileID == "" {
							return nil, fmt.Errorf("file ID is required for file-based image content")
						}
						inputImage.FileID = openai.String(c.Source.FileID)

					default:
						return nil, fmt.Errorf("unsupported content source type for image: %v", c.Source.Type)
					}

					contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
						OfInputImage: inputImage,
					})
				}

			case *llm.DocumentContent:
				if c.Source == nil {
					return nil, fmt.Errorf("document content source is required")
				}

				var fileParam responses.ResponseInputFileParam

				// Handle filename - preserve empty titles if explicitly set, otherwise default
				if c.Title != "" {
					fileParam.Filename = openai.String(c.Title)
				} else {
					fileParam.Filename = openai.String("document")
				}

				switch c.Source.Type {
				case llm.ContentSourceTypeBase64:
					if c.Source.MediaType == "" {
						return nil, fmt.Errorf("media type is required for base64 document content")
					}
					if c.Source.Data == "" {
						return nil, fmt.Errorf("data is required for base64 document content")
					}
					// Create data URL format expected by OpenAI
					dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
					fileParam.FileData = openai.String(dataURL)

				case llm.ContentSourceTypeFile:
					if c.Source.FileID == "" {
						return nil, fmt.Errorf("file ID is required for file-based document content")
					}
					fileParam.FileID = openai.String(c.Source.FileID)

				case llm.ContentSourceTypeURL:
					return nil, fmt.Errorf("URL-based document content is not supported by OpenAI Responses API - use file upload or base64 encoding instead")

				default:
					return nil, fmt.Errorf("unsupported content source type for document: %v", c.Source.Type)
				}

				contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
					OfInputFile: &fileParam,
				})

			case *llm.ToolResultContent:
				if contentStr, ok := c.Content.(string); ok {
					contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
						OfInputText: &responses.ResponseInputTextParam{
							Text: contentStr,
						},
					})
				}

			case *llm.ToolUseContent:
				contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{
						Text: fmt.Sprintf("Tool use: %s", c.Name),
					},
				})

			default:
				return nil, fmt.Errorf("unsupported content type: %T", c)
			}
		}

		inputItems = append(inputItems, responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Role: responses.EasyInputMessageRole(msg.Role),
				Content: responses.EasyInputMessageContentUnionParam{
					OfInputItemContentList: contentItems,
				},
			},
		})
	}
	return inputItems, nil
}

// convertResponse converts SDK response to llm.Response
func (p *Provider) convertResponse(response *responses.Response) (*llm.Response, error) {
	var contentBlocks []llm.Content

	for _, item := range response.Output {
		switch item.Type {
		case "message":
			outputMsg := item.AsMessage()
			for _, content := range outputMsg.Content {
				switch content.Type {
				case "output_text":
					contentBlocks = append(contentBlocks, &llm.TextContent{
						Text: content.AsOutputText().Text,
					})
				case "refusal":
					contentBlocks = append(contentBlocks, &llm.RefusalContent{
						Text: content.AsRefusal().Refusal,
					})
				}
			}

		case "function_call":
			functionCall := item.AsFunctionCall()
			contentBlocks = append(contentBlocks, &llm.ToolUseContent{
				ID:    functionCall.CallID,
				Name:  functionCall.Name,
				Input: []byte(functionCall.Arguments),
			})

		case "image_generation_call":
			imgCall := item.AsImageGenerationCall()
			if imgCall.Result != "" {
				contentBlocks = append(contentBlocks, &llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "image/png",
						Data:      imgCall.Result,
					},
				})
			}

		case "web_search_call":
			// Handle web search results if they exist
			contentBlocks = append(contentBlocks, &llm.TextContent{
				Text: "Web search completed",
			})

		case "mcp_call":
			mcpCall := item.AsMcpCall()
			if mcpCall.Output != "" {
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: fmt.Sprintf("MCP tool result: %s", mcpCall.Output),
				})
			}

		case "mcp_list_tools":
			mcpList := item.AsMcpListTools()
			var toolsText strings.Builder
			toolsText.WriteString(fmt.Sprintf("MCP server '%s' tools:\n", mcpList.ServerLabel))
			for _, tool := range mcpList.Tools {
				toolsText.WriteString(fmt.Sprintf("- %s\n", tool.Name))
			}
			contentBlocks = append(contentBlocks, &llm.TextContent{
				Text: toolsText.String(),
			})

		case "mcp_approval_request":
			mcpApproval := item.AsMcpApprovalRequest()
			contentBlocks = append(contentBlocks, &llm.TextContent{
				Text: fmt.Sprintf("MCP approval required for tool '%s' on server '%s'", mcpApproval.Name, mcpApproval.ServerLabel),
			})
		}
	}

	usage := llm.Usage{}
	usage.InputTokens = int(response.Usage.InputTokens)
	usage.OutputTokens = int(response.Usage.OutputTokens)
	usage.CacheReadInputTokens = int(response.Usage.InputTokensDetails.CachedTokens)

	// Determine stop reason based on the response content and status
	stopReason := p.determineStopReason(response)

	return &llm.Response{
		ID:         response.ID,
		Model:      string(response.Model),
		Role:       llm.Assistant,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

// determineStopReason maps SDK response data to standard stop reasons
func (p *Provider) determineStopReason(response *responses.Response) string {
	// Check if the response contains any tool calls
	for _, item := range response.Output {
		if strings.HasSuffix(item.Type, "_call") {
			return "tool_use"
		}
	}
	// If response is completed without tool calls, it's an end_turn
	if response.Status == "completed" {
		return "end_turn"
	}
	// Default to end_turn for other completion scenarios
	return "end_turn"
}
