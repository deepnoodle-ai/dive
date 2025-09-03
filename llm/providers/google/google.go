package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/retry"
	"google.golang.org/genai"
)

const ProviderName = "google"

var (
	DefaultModel         = ModelGemini25FlashPro
	DefaultMaxTokens     = 4096
	DefaultClient        *http.Client
	DefaultMaxRetries    = 3
	DefaultRetryBaseWait = 1 * time.Second
	DefaultVersion       = "v1"
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	client        *genai.Client
	projectID     string
	location      string
	apiKey        string
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	version       string
	mutex         sync.Mutex
}

func New(opts ...Option) *Provider {
	var apiKey string
	if value := os.Getenv("GEMINI_API_KEY"); value != "" {
		apiKey = value
	} else if value := os.Getenv("GOOGLE_API_KEY"); value != "" {
		apiKey = value
	}
	p := &Provider{
		apiKey:        apiKey,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
		version:       DefaultVersion,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) initClient(ctx context.Context) (*genai.Client, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.client != nil {
		return p.client, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:   p.apiKey,
		Project:  p.projectID,
		Location: p.location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create google genai client: %v", err)
	}
	p.client = client
	return p.client, nil
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	if _, err := p.initClient(ctx); err != nil {
		return nil, err
	}

	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, err
	}

	// Create generation config
	genConfig := &genai.GenerateContentConfig{}

	if request.Temperature != nil {
		temp := float32(*request.Temperature)
		genConfig.Temperature = &temp
	}

	if request.MaxTokens > 0 {
		genConfig.MaxOutputTokens = int32(request.MaxTokens)
	}

	// Handle system prompt
	if request.System != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(request.System)},
		}
	}

	// Handle tools
	if len(request.Tools) > 0 {
		tools := make([]*genai.Tool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema *genai.Schema
			if inputSchema, ok := tool["input_schema"]; ok && inputSchema != nil {
				// Convert the input schema from Dive format to genai.Schema
				schema = convertAnySchemaToGenAI(inputSchema)
			}

			genaiTool := &genai.Tool{
				FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool["name"].(string),
						Description: tool["description"].(string),
						Parameters:  schema,
					},
				},
			}
			tools = append(tools, genaiTool)
		}
		genConfig.Tools = tools

		// Enable function calling
		genConfig.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
	}); err != nil {
		return nil, err
	}

	var result *llm.Response
	err = retry.Do(ctx, func() error {
		// Create chat with history if we have multiple messages
		var chat *genai.Chat
		var parts []*genai.Part

		if len(msgs) > 1 {
			// Create chat with history
			var history []*genai.Content
			for _, msg := range msgs[:len(msgs)-1] {
				content := &genai.Content{
					Role:  string(msg.Role),
					Parts: make([]*genai.Part, 0, len(msg.Content)),
				}
				for _, c := range msg.Content {
					switch ct := c.(type) {
					case *llm.TextContent:
						content.Parts = append(content.Parts, genai.NewPartFromText(ct.Text))
					case *llm.ImageContent:
						if ct.Source != nil && ct.Source.Data != "" {
							// For now, skip image handling - would need proper base64 decoding
							// and use genai.NewPartFromBytes
							content.Parts = append(content.Parts, genai.NewPartFromText("[Image content]"))
						}
					}
				}
				history = append(history, content)
			}

			chat, err = p.client.Chats.Create(ctx, request.Model, genConfig, history)
			if err != nil {
				return fmt.Errorf("error creating chat: %w", err)
			}

			// Use the last message as the current input
			lastMsg := msgs[len(msgs)-1]
			for _, c := range lastMsg.Content {
				switch ct := c.(type) {
				case *llm.TextContent:
					parts = append(parts, genai.NewPartFromText(ct.Text))
				case *llm.ImageContent:
					if ct.Source != nil && ct.Source.Data != "" {
						// For now, skip image handling - would need proper base64 decoding
						// and use genai.NewPartFromBytes
						parts = append(parts, genai.NewPartFromText("[Image content]"))
					}
				}
			}

			// Convert []*genai.Part to []genai.Part for variadic function
			partValues := make([]genai.Part, len(parts))
			for i, part := range parts {
				partValues[i] = *part
			}
			resp, err := chat.SendMessage(ctx, partValues...)
			if err != nil {
				return fmt.Errorf("error making request: %w", err)
			}

			// Convert Google response to Dive format
			result, err = convertGoogleResponse(resp)
			if err != nil {
				return fmt.Errorf("error converting response: %w", err)
			}
		} else if len(msgs) == 1 {
			// Single message, create chat without history
			chat, err = p.client.Chats.Create(ctx, request.Model, genConfig, nil)
			if err != nil {
				return fmt.Errorf("error creating chat: %w", err)
			}

			// Use the single message as input
			for _, c := range msgs[0].Content {
				switch ct := c.(type) {
				case *llm.TextContent:
					parts = append(parts, genai.NewPartFromText(ct.Text))
				case *llm.ImageContent:
					if ct.Source != nil && ct.Source.Data != "" {
						// For now, skip image handling - would need proper base64 decoding
						// and use genai.NewPartFromBytes
						parts = append(parts, genai.NewPartFromText("[Image content]"))
					}
				}
			}

			// Convert []*genai.Part to []genai.Part for variadic function
			partValues := make([]genai.Part, len(parts))
			for i, part := range parts {
				partValues[i] = *part
			}
			resp, err := chat.SendMessage(ctx, partValues...)
			if err != nil {
				return fmt.Errorf("error making request: %w", err)
			}

			// Convert Google response to Dive format
			result, err = convertGoogleResponse(resp)
			if err != nil {
				return fmt.Errorf("error converting response: %w", err)
			}
		} else {
			return fmt.Errorf("no messages provided")
		}

		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.AfterGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
		Response: &llm.HookResponseContext{
			Response: result,
		},
	}); err != nil {
		return nil, err
	}

	return result, nil
}

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	if _, err := p.initClient(ctx); err != nil {
		return nil, err
	}

	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	// Create generation config
	genConfig := &genai.GenerateContentConfig{}

	if request.Temperature != nil {
		temp := float32(*request.Temperature)
		genConfig.Temperature = &temp
	}

	if request.MaxTokens > 0 {
		genConfig.MaxOutputTokens = int32(request.MaxTokens)
	}

	// Handle system prompt
	if request.System != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(request.System)},
		}
	}

	// Handle tools
	if len(request.Tools) > 0 {
		tools := make([]*genai.Tool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema *genai.Schema
			if inputSchema, ok := tool["input_schema"]; ok && inputSchema != nil {
				// Convert the input schema from Dive format to genai.Schema
				schema = convertAnySchemaToGenAI(inputSchema)
			}

			genaiTool := &genai.Tool{
				FunctionDeclarations: []*genai.FunctionDeclaration{
					{
						Name:        tool["name"].(string),
						Description: tool["description"].(string),
						Parameters:  schema,
					},
				},
			}
			tools = append(tools, genaiTool)
		}
		genConfig.Tools = tools

		// Enable function calling
		genConfig.ToolConfig = &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: genai.FunctionCallingConfigModeAuto,
			},
		}
	}

	var stream *StreamIterator
	err = retry.Do(ctx, func() error {
		// Create chat with history if we have multiple messages
		var chat *genai.Chat
		var parts []*genai.Part

		if len(msgs) > 1 {
			// Create chat with history
			var history []*genai.Content
			for _, msg := range msgs[:len(msgs)-1] {
				content := &genai.Content{
					Role:  string(msg.Role),
					Parts: make([]*genai.Part, 0, len(msg.Content)),
				}
				for _, c := range msg.Content {
					switch ct := c.(type) {
					case *llm.TextContent:
						content.Parts = append(content.Parts, genai.NewPartFromText(ct.Text))
					case *llm.ImageContent:
						if ct.Source != nil && ct.Source.Data != "" {
							// For now, skip image handling - would need proper base64 decoding
							// and use genai.NewPartFromBytes
							content.Parts = append(content.Parts, genai.NewPartFromText("[Image content]"))
						}
					}
				}
				history = append(history, content)
			}

			chat, err = p.client.Chats.Create(ctx, request.Model, genConfig, history)
			if err != nil {
				fmt.Println("CREATE FAILED: ", err)
				return fmt.Errorf("error creating chat: %w", err)
			}

			// Use the last message as the current input
			lastMsg := msgs[len(msgs)-1]
			for _, c := range lastMsg.Content {
				switch ct := c.(type) {
				case *llm.TextContent:
					parts = append(parts, genai.NewPartFromText(ct.Text))
				case *llm.ImageContent:
					if ct.Source != nil && ct.Source.Data != "" {
						// For now, skip image handling - would need proper base64 decoding
						// and use genai.NewPartFromBytes
						parts = append(parts, genai.NewPartFromText("[Image content]"))
					}
				}
			}
		} else if len(msgs) == 1 {
			// Single message, create chat without history
			chat, err = p.client.Chats.Create(ctx, request.Model, genConfig, nil)
			if err != nil {
				fmt.Println("CREATE FAILED: ", err)
				return fmt.Errorf("error creating chat: %w", err)
			}

			// Use the single message as input
			for _, c := range msgs[0].Content {
				switch ct := c.(type) {
				case *llm.TextContent:
					parts = append(parts, genai.NewPartFromText(ct.Text))
				case *llm.ImageContent:
					if ct.Source != nil && ct.Source.Data != "" {
						// For now, skip image handling - would need proper base64 decoding
						// and use genai.NewPartFromBytes
						parts = append(parts, genai.NewPartFromText("[Image content]"))
					}
				}
			}
		} else {
			return fmt.Errorf("no messages provided")
		}

		// Convert []*genai.Part to []genai.Part for StreamIterator
		partValues := make([]genai.Part, len(parts))
		for i, part := range parts {
			partValues[i] = *part
		}
		stream = NewStreamIterator(ctx, chat, partValues, request.Model)
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (p *Provider) applyRequestConfig(req *Request, config *llm.Config) error {
	req.Model = config.Model
	if req.Model == "" {
		req.Model = p.model
	}

	if config.MaxTokens != nil {
		req.MaxTokens = *config.MaxTokens
	} else {
		req.MaxTokens = p.maxTokens
	}

	if len(config.Tools) > 0 {
		var tools []map[string]any
		for _, tool := range config.Tools {
			schema := tool.Schema()
			toolConfig := map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
			}
			if schema.Type != "" {
				toolConfig["input_schema"] = schema
			}
			tools = append(tools, toolConfig)
		}
		req.Tools = tools
	}

	req.Temperature = config.Temperature
	req.System = config.SystemPrompt

	return nil
}
