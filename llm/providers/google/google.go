package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/internal/retry"
	"github.com/deepnoodle-ai/dive/llm"
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

	// Convert messages to genai.Content format
	contents, err := messagesToContents(config.Messages)
	if err != nil {
		return nil, err
	}

	// Create generation config
	genConfig, err := buildGenAIGenerateConfig(&request)
	if err != nil {
		return nil, err
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
		// Use Models.GenerateContent directly
		resp, err := p.client.Models.GenerateContent(ctx, request.Model, contents, genConfig)
		if err != nil {
			return fmt.Errorf("error generating content: %w", err)
		}

		var convErr error
		result, convErr = convertGoogleResponse(resp, request.Model)
		if convErr != nil {
			return fmt.Errorf("error converting response: %w", convErr)
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

	// Convert messages to genai.Content format
	contents, err := messagesToContents(config.Messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	// Create generation config
	genConfig, err := buildGenAIGenerateConfig(&request)
	if err != nil {
		return nil, err
	}

	var stream *StreamIterator
	err = retry.Do(ctx, func() error {
		// Use Models.GenerateContentStream directly
		streamSeq := p.client.Models.GenerateContentStream(ctx, request.Model, contents, genConfig)
		stream = NewStreamIteratorFromSeq(ctx, streamSeq, request.Model)
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
