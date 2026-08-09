package google

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
	"google.golang.org/genai"
)

type Request struct {
	Model       string           `json:"model"`
	Messages    []*llm.Message   `json:"messages"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	System      string           `json:"system,omitempty"`
	Tools       []map[string]any `json:"tools,omitempty"`

	// Thinking carries Gemini's thinkingConfig: the thinking level or budget,
	// plus whether to surface thought summaries. Nil leaves the model on its
	// own default.
	Thinking *genai.ThinkingConfig `json:"thinking,omitempty"`
}

type Tool struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	InputSchema schema.Schema `json:"input_schema"`
}
