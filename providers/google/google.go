package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/retry"
	"google.golang.org/genai"
)

const ProviderName = "google"

var (
	DefaultModel         = ModelGemini25Pro
	DefaultMaxTokens     = 32768
	DefaultClient        *http.Client
	DefaultMaxRetries    = 3
	DefaultRetryBaseWait = 1 * time.Second
	DefaultVersion       = "v1"
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Google Gemini LLM provider.
type Provider struct {
	client        *genai.Client
	projectID     string
	location      string
	apiKey        string
	vertexAI      bool
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	version       string
	mutex         sync.Mutex
}

// New creates a new Google Gemini provider with the given options.
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
	var cfg *genai.ClientConfig
	if p.vertexAI {
		// Vertex AI authenticates with Application Default Credentials. An API
		// key is mutually exclusive with project/location in the genai client
		// config, and this backend resolves auth via ADC, so we pass only the
		// project and location. An empty location is resolved by the SDK from
		// GOOGLE_CLOUD_LOCATION/GOOGLE_CLOUD_REGION, defaulting to "global".
		cfg = &genai.ClientConfig{
			Backend:  genai.BackendVertexAI,
			Project:  p.projectID,
			Location: p.location,
		}
	} else {
		// The Gemini API backend authenticates with an API key, which is
		// mutually exclusive with project/location, so we pass only the key.
		cfg = &genai.ClientConfig{
			APIKey: p.apiKey,
		}
	}
	client, err := genai.NewClient(ctx, cfg)
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
	config := &llm.Config{}
	config.Apply(opts...)
	rendered, err := renderReminderMessages(config.Messages)
	if err != nil {
		return nil, err
	}
	if _, err := p.initClient(ctx); err != nil {
		return nil, err
	}

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	// Convert messages to genai.Content format
	contents, err := messagesToContents(rendered)
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
	err = retry.DoSimple(ctx, func() error {
		// Use Models.GenerateContent directly
		resp, err := p.client.Models.GenerateContent(ctx, request.Model, contents, genConfig)
		if err != nil {
			return wrapGoogleError(err)
		}

		var convErr error
		result, convErr = convertGoogleResponse(resp, request.Model)
		if convErr != nil {
			return fmt.Errorf("error converting response: %w", convErr)
		}
		return nil
	}, retry.WithMaxAttempts(p.maxRetries+1), retry.WithBackoff(p.retryBaseWait, 5*time.Minute), retry.WithRetryIf(retry.SkipPermanent()))

	if err != nil {
		return nil, err
	}

	llm.PopulateCost(result.Model, result.Usage.Speed == string(llm.SpeedFast), &result.Usage)

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
	config := &llm.Config{}
	config.Apply(opts...)
	rendered, err := renderReminderMessages(config.Messages)
	if err != nil {
		return nil, err
	}
	if _, err := p.initClient(ctx); err != nil {
		return nil, err
	}

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	// Convert messages to genai.Content format
	contents, err := messagesToContents(rendered)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	// Create generation config
	genConfig, err := buildGenAIGenerateConfig(&request)
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

	stream := providers.NewRetryingStreamIterator(ctx, providers.StreamRetryConfig{
		Provider:      p.Name(),
		MaxRetries:    p.maxRetries,
		RetryBaseWait: p.retryBaseWait,
		Logger:        config.Logger,
	}, func() (llm.StreamIterator, error) {
		// GenerateContentStream reports request failures through its lazy
		// sequence. The shared iterator consumes the first result as part of
		// the provider's pre-event retry boundary.
		streamSeq := p.client.Models.GenerateContentStream(ctx, request.Model, contents, genConfig)
		return NewStreamIteratorFromSeq(ctx, streamSeq, request.Model), nil
	})

	return stream, nil
}

func renderReminderMessages(messages []*llm.Message) ([]*llm.Message, error) {
	// Gemini contents allow only user/model roles, so operator reminders always
	// render as tagged user messages (nil resolver = no native authority).
	return llm.RenderReminders(messages, nil)
}

// wrapGoogleError converts a Google API error to a providers.NewError so that
// retry.SkipPermanent can detect non-retryable status codes. Falls back to the
// original error if it's not a genai.APIError. The SDK returns APIError as a
// value today, while callers and wrappers may still expose a pointer, so both
// forms must be handled.
func wrapGoogleError(err error) error {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return providers.NewError(apiErr.Code, apiErr.Message)
	}
	var apiErrPtr *genai.APIError
	if errors.As(err, &apiErrPtr) {
		return providers.NewError(apiErrPtr.Code, apiErrPtr.Message)
	}
	return err
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

	if !shouldOmitTemperature(req.Model) {
		req.Temperature = config.Temperature
	} else if config.Temperature != nil && config.Logger != nil {
		config.Logger.Warn("temperature is not supported by this Google model and will be ignored",
			"model", req.Model)
	}
	// Dive does not currently expose top_p or top_k. If those controls are
	// added, apply the same Gemini request-generation cutoff used above.
	req.System = config.SystemPrompt

	return nil
}
