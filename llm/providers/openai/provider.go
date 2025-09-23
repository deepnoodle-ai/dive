package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/internal/retry"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
)

const ProviderName = "openai"

var (
	DefaultModel         = "gpt-5-2025-08-07"
	DefaultMaxTokens     = 4096
	DefaultMaxRetries    = 2
	DefaultRetryBaseWait = 1 * time.Second
	DefaultClient        = &http.Client{Timeout: 300 * time.Second}
)

var _ llm.LLM = &Provider{}
var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	client        openai.Client
	model         openai.ChatModel
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	httpClient    *http.Client
	options       []option.RequestOption
}

func New(opts ...Option) *Provider {
	p := &Provider{
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
		httpClient:    DefaultClient,
	}
	for _, opt := range opts {
		opt(p)
	}
	p.client = openai.NewClient(p.options...)
	return p
}

func (p *Provider) Name() string {
	return ProviderName
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

	var resp *responses.Response
	err = retry.Do(ctx, func() error {
		resp, err = p.client.Responses.New(
			ctx,
			params,
			option.WithRequestTimeout(5*time.Minute),
			option.WithHTTPClient(p.httpClient),
		)
		if err != nil {
			var apierr *openai.Error
			if errors.As(err, &apierr) {
				return providers.NewError(apierr.StatusCode, apierr.Message)
			}
			return err
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}

	return decodeAssistantResponse(resp)
}

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
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

	streamSDK := p.client.Responses.NewStreaming(
		ctx,
		params,
		option.WithRequestTimeout(5*time.Minute),
		option.WithHTTPClient(p.httpClient),
	)

	return newOpenAIStreamIterator(streamSDK, config), nil
}

// buildRequestParams converts llm.Config to responses.ResponseNewParams
func (p *Provider) buildRequestParams(config *llm.Config) (responses.ResponseNewParams, error) {
	if len(config.Messages) == 0 {
		return responses.ResponseNewParams{}, fmt.Errorf("no messages provided")
	}

	// Convert input messages to the OpenAI SDK input type
	input, err := encodeMessages(config.Messages)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
		Store: openai.Bool(false),
	}

	if config.Model != "" {
		params.Model = config.Model
	} else {
		params.Model = p.model
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

	includes := map[Include]bool{}

	// Handle reasoning effort
	if config.ReasoningEffort != "" {
		params.Reasoning = responses.ReasoningParam{
			Effort: responses.ReasoningEffort(config.ReasoningEffort),
		}
		if config.ReasoningSummary != "" {
			switch config.ReasoningSummary {
			case llm.ReasoningSummaryAuto:
				params.Reasoning.Summary = responses.ReasoningSummaryAuto
			case llm.ReasoningSummaryConcise:
				params.Reasoning.Summary = responses.ReasoningSummaryConcise
			case llm.ReasoningSummaryDetailed:
				params.Reasoning.Summary = responses.ReasoningSummaryDetailed
			default:
				return responses.ResponseNewParams{},
					fmt.Errorf("invalid reasoning summary: %s", config.ReasoningSummary)
			}
		}
		includes[IncludeReasoningEncryptedContent] = true
	}

	// Includes are used to include additional data in the response
	if len(includes) > 0 {
		params.Include = make([]responses.ResponseIncludable, 0, len(includes))
		for include := range includes {
			params.Include = append(params.Include, responses.ResponseIncludable(include))
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
			return responses.ResponseNewParams{},
				fmt.Errorf("invalid service tier: %s", config.ServiceTier)
		}
	}

	// Handle response format
	if config.ResponseFormat != nil {
		if err := applyResponseFormat(&params, config); err != nil {
			return responses.ResponseNewParams{}, err
		}
	}

	// Handle tool choice
	if config.ToolChoice != nil && len(config.Tools) > 0 {
		switch config.ToolChoice.Type {
		case llm.ToolChoiceTypeAuto:
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
			}
		case llm.ToolChoiceTypeNone:
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsNone),
			}
		case llm.ToolChoiceTypeAny:
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsRequired),
			}
		case llm.ToolChoiceTypeTool:
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfFunctionTool: &responses.ToolChoiceFunctionParam{
					Name: config.ToolChoice.Name,
				},
			}
		default:
			return responses.ResponseNewParams{},
				fmt.Errorf("invalid tool choice: %s", config.ToolChoice)
		}
	}

	// Convert tools
	if len(config.Tools) > 0 {
		var tools []responses.ToolUnionParam
		for _, tool := range config.Tools {
			if toolWebSearch, ok := tool.(*WebSearchPreviewTool); ok {
				tools = append(tools, responses.ToolUnionParam{
					OfWebSearchPreview: toolWebSearch.Param(),
				})
				continue
			}
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
			toolConfig := mcpServer.ToolConfiguration
			if len(toolConfig.AllowedTools) > 0 {
				mcpParam.AllowedTools = responses.ToolMcpAllowedToolsUnionParam{
					OfMcpAllowedTools: toolConfig.AllowedTools,
				}
			}
			if toolConfig.ApprovalMode != "" && toolConfig.ApprovalFilter != nil {
				return responses.ResponseNewParams{}, fmt.Errorf("tool approval and tool approval filter cannot be used together")
			}
			if toolConfig.ApprovalMode != "" {
				mcpParam.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
					OfMcpToolApprovalSetting: openai.String(toolConfig.ApprovalMode),
				}
			} else if toolConfig.ApprovalFilter != nil {
				mcpParam.RequireApproval = responses.ToolMcpRequireApprovalUnionParam{
					OfMcpToolApprovalFilter: &responses.ToolMcpRequireApprovalMcpToolApprovalFilterParam{
						Always: responses.ToolMcpRequireApprovalMcpToolApprovalFilterAlwaysParam{
							ToolNames: toolConfig.ApprovalFilter.Always,
						},
						Never: responses.ToolMcpRequireApprovalMcpToolApprovalFilterNeverParam{
							ToolNames: toolConfig.ApprovalFilter.Never,
						},
					},
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
		params.Tools = append(params.Tools, tool)
	}
	return params, nil
}

// applyResponseFormat handles setting up response format options
func applyResponseFormat(params *responses.ResponseNewParams, config *llm.Config) error {
	format := config.ResponseFormat

	switch format.Type {
	case llm.ResponseFormatTypeJSONSchema:
		if format.Schema == nil {
			return fmt.Errorf("schema is required for json_schema response format")
		}
		if format.Name == "" {
			return fmt.Errorf("name is required for json_schema response format")
		}
		schemaMap := format.Schema.AsMap()
		schemaMap["additionalProperties"] = false
		schema := &responses.ResponseFormatTextJSONSchemaConfigParam{
			Type:   "json_schema",
			Name:   format.Name,
			Schema: schemaMap,
			Strict: openai.Bool(true),
		}
		if format.Description != "" {
			schema.Description = openai.String(format.Description)
		}
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: schema,
			},
		}

	case llm.ResponseFormatTypeJSON:
		params.Text = responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &responses.ResponseFormatJSONObjectParam{
					Type: "json_object",
				},
			},
		}

	case llm.ResponseFormatTypeText:
		// Text is the default format, no need to set anything
		return nil

	default:
		return fmt.Errorf("unsupported response format type: %s", format.Type)
	}
	return nil
}

// determineStopReason maps SDK response data to standard stop reasons
func determineStopReason(response *responses.Response) string {
	// Check if the response contains any tool calls
	for _, item := range response.Output {
		if strings.HasSuffix(item.Type, "_call") {
			return "tool_use"
		}
	}

	// Handle different response statuses
	switch response.Status {
	case "completed":
		return "end_turn"
	case "incomplete":
		// Map specific incomplete reasons if available
		if response.IncompleteDetails.Reason != "" {
			switch response.IncompleteDetails.Reason {
			case "max_output_tokens":
				return "max_tokens"
			case "content_filter":
				return "content_filter"
			case "run_cancelled":
				return "cancelled"
			case "run_expired":
				return "timeout"
			case "run_failed":
				return "error"
			default:
				return "incomplete"
			}
		}
		return "incomplete"
	case "failed":
		return "error"
	case "in_progress":
		return "incomplete"
	default:
		return "end_turn"
	}
}
