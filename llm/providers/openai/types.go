package openai

import (
	"github.com/diveagents/dive/schema"
)

// Include is a type that represents different types of content that can be
// optionally requested in OpenAI Responses API responses.
type Include string

const (
	IncludeReasoningEncryptedContent  Include = "reasoning.encrypted_content"
	IncludeFileSearchCallResults      Include = "file_search_call.results"
	IncludeInputImageURL              Include = "message.input_image.image_url"
	IncludeComputerCallOutputImageURL Include = "computer_call_output.output.image_url"
	IncludeCodeInterpreterCallOutputs Include = "code_interpreter_call.outputs"
)

// // Request represents the OpenAI Responses API request structure
// type Request struct {
// 	Model              string            `json:"model"`
// 	Input              []*InputMessage   `json:"input"`
// 	Include            []Include         `json:"include,omitempty"`
// 	Instructions       string            `json:"instructions,omitempty"`
// 	MaxOutputTokens    int               `json:"max_output_tokens,omitempty"`
// 	Metadata           map[string]string `json:"metadata,omitempty"`
// 	PreviousResponseID string            `json:"previous_response_id,omitempty"`
// 	ServiceTier        string            `json:"service_tier,omitempty"`
// 	Reasoning          *ReasoningConfig  `json:"reasoning,omitempty"`
// 	ParallelToolCalls  *bool             `json:"parallel_tool_calls,omitempty"`
// 	Stream             *bool             `json:"stream,omitempty"`
// 	Temperature        *float64          `json:"temperature,omitempty"`
// 	Text               *TextConfig       `json:"text,omitempty"`
// 	ToolChoice         interface{}       `json:"tool_choice,omitempty"`
// 	Tools              []any             `json:"tools,omitempty"`
// }

// // ReasoningConfig for o-series models
// type ReasoningConfig struct {
// 	Effort *string `json:"effort,omitempty"` // "low", "medium", "high"
// }

// TextConfig for text response configuration
type TextConfig struct {
	Format TextFormat `json:"format"`
}

// TextFormat defines the text output format
type TextFormat struct {
	Type   string      `json:"type"` // "text" or "json_schema"
	Schema interface{} `json:"schema,omitempty"`
}

// InputMessage represents a message in the input
// type InputMessage struct {
// 	Role    string          `json:"role"`
// 	Content []*InputContent `json:"content"`
// }

// InputContent represents content within a message
// type InputContent struct {
// 	Type              string `json:"type"`
// 	Text              string `json:"text,omitempty"`
// 	ImageURL          string `json:"image_url,omitempty"`
// 	Filename          string `json:"filename,omitempty"`
// 	FileData          string `json:"file_data,omitempty"`
// 	FileID            string `json:"file_id,omitempty"`
// 	Approve           *bool  `json:"approve,omitempty"`
// 	ApprovalRequestID string `json:"approval_request_id,omitempty"`
// }

// UserLocation represents user location for web search
type UserLocation struct {
	Type     string `json:"type"`
	City     string `json:"city,omitempty"`     // e.g. "San Francisco"
	Country  string `json:"country,omitempty"`  // two letter ISO code e.g. "US"
	Region   string `json:"region,omitempty"`   // e.g. "California"
	Timezone string `json:"timezone,omitempty"` // e.g. "America/Los_Angeles"
}

// // Response represents the OpenAI Responses API response structure
// type Response struct {
// 	ID                 string             `json:"id"`
// 	Object             string             `json:"object"`
// 	CreatedAt          int64              `json:"created_at"`
// 	Status             string             `json:"status"`
// 	Error              *ResponseError     `json:"error,omitempty"`
// 	IncompleteDetails  *IncompleteDetails `json:"incomplete_details,omitempty"`
// 	Instructions       string             `json:"instructions,omitempty"`
// 	MaxOutputTokens    int                `json:"max_output_tokens,omitempty"`
// 	Model              string             `json:"model"`
// 	Output             []OutputItem       `json:"output"`
// 	ParallelToolCalls  bool               `json:"parallel_tool_calls"`
// 	PreviousResponseID string             `json:"previous_response_id,omitempty"`
// 	ServiceTier        string             `json:"service_tier,omitempty"`
// 	Reasoning          *ReasoningResult   `json:"reasoning,omitempty"`
// 	Store              bool               `json:"store"`
// 	Temperature        float64            `json:"temperature"`
// 	Text               *TextConfig        `json:"text,omitempty"`
// 	ToolChoice         interface{}        `json:"tool_choice,omitempty"`
// 	Tools              []any              `json:"tools"`
// 	TopP               float64            `json:"top_p"`
// 	Truncation         string             `json:"truncation"`
// 	Usage              *Usage             `json:"usage,omitempty"`
// 	User               string             `json:"user,omitempty"`
// 	Metadata           map[string]string  `json:"metadata,omitempty"`
// }

// // ResponseError represents an error in the response
// type ResponseError struct {
// 	Type    string `json:"type"`
// 	Message string `json:"message"`
// }

// // IncompleteDetails provides details about incomplete responses
// type IncompleteDetails struct {
// 	Reason string `json:"reason"`
// }

// // ReasoningResult contains reasoning information for o-series models
// type ReasoningResult struct {
// 	Effort  *string `json:"effort,omitempty"`
// 	Summary *string `json:"summary,omitempty"`
// }

// Usage represents token usage information
type Usage struct {
	InputTokens         int                  `json:"input_tokens"`
	InputTokensDetails  *InputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokens        int                  `json:"output_tokens"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
	TotalTokens         int                  `json:"total_tokens"`
}

// InputTokensDetails provides breakdown of input tokens
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails provides breakdown of output tokens
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// OutputItem represents an item in the response output
type OutputItem struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Status  string          `json:"status,omitempty"`
	Role    string          `json:"role,omitempty"`
	Content []OutputContent `json:"content,omitempty"`
	// Tool call fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// Web search fields
	Results []WebSearchResult `json:"results,omitempty"`
	// Image generation fields
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Result        string `json:"result,omitempty"` // base64 image
	// MCP fields
	ServerLabel       string              `json:"server_label,omitempty"`
	Tools             []MCPToolDefinition `json:"tools,omitempty"`
	Output            string              `json:"output,omitempty"`
	ApprovalRequestID string              `json:"approval_request_id,omitempty"`
}

// OutputContent represents content within an output item
type OutputContent struct {
	Type        string       `json:"type"`
	Text        string       `json:"text,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

// Annotation represents an annotation in the content
type Annotation struct {
	Type       string `json:"type"`
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
}

// WebSearchResult represents a web search result
type WebSearchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// MCPToolDefinition represents an MCP tool definition
type MCPToolDefinition struct {
	Name        string        `json:"name"`
	InputSchema schema.Schema `json:"input_schema"`
}

// // StreamEvent represents a streaming event from the Responses API
// type StreamEvent struct {
// 	Type           string    `json:"type"`
// 	SequenceNumber int       `json:"sequence_number,omitempty"`
// 	Response       *Response `json:"response,omitempty"`

// 	// Fields for output_item events
// 	OutputIndex int         `json:"output_index,omitempty"`
// 	Item        *OutputItem `json:"item,omitempty"`

// 	// Fields for content_part events
// 	ItemID       string             `json:"item_id,omitempty"`
// 	ContentIndex int                `json:"content_index,omitempty"`
// 	Part         *StreamContentPart `json:"part,omitempty"`

// 	// Fields for delta events
// 	Delta string `json:"delta,omitempty"`

// 	// Fields for done events
// 	Text string `json:"text,omitempty"`
// }

// // StreamContentPart represents a content part in streaming events
// type StreamContentPart struct {
// 	Type        string       `json:"type"`
// 	Text        string       `json:"text,omitempty"`
// 	Annotations []Annotation `json:"annotations,omitempty"`
// }

// // StreamResponse represents the structure of streaming responses
// type StreamResponse struct {
// 	Type     string    `json:"type"`
// 	Response *Response `json:"response,omitempty"`
// }
