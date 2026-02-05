package dive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
)

type ToolAnnotations struct {
	Title           string         `json:"title,omitempty"`
	ReadOnlyHint    bool           `json:"readOnlyHint,omitempty"`
	DestructiveHint bool           `json:"destructiveHint,omitempty"`
	IdempotentHint  bool           `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool           `json:"openWorldHint,omitempty"`
	EditHint        bool           `json:"editHint,omitempty"` // Indicates file edit operations for acceptEdits mode
	Extra           map[string]any `json:"extra,omitempty"`
}

func (a *ToolAnnotations) MarshalJSON() ([]byte, error) {
	data := map[string]any{
		"title":           a.Title,
		"readOnlyHint":    a.ReadOnlyHint,
		"destructiveHint": a.DestructiveHint,
		"idempotentHint":  a.IdempotentHint,
		"openWorldHint":   a.OpenWorldHint,
		"editHint":        a.EditHint,
	}
	if a.Extra != nil {
		for k, v := range a.Extra {
			data[k] = v
		}
	}
	return json.Marshal(data)
}

func (a *ToolAnnotations) UnmarshalJSON(data []byte) error {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	// Extract known fields
	if title, ok := rawMap["title"]; ok {
		json.Unmarshal(title, &a.Title)
		delete(rawMap, "title")
	}
	// Handle boolean hints
	boolFields := map[string]*bool{
		"readOnlyHint":    &a.ReadOnlyHint,
		"destructiveHint": &a.DestructiveHint,
		"idempotentHint":  &a.IdempotentHint,
		"openWorldHint":   &a.OpenWorldHint,
		"editHint":        &a.EditHint,
	}
	for name, field := range boolFields {
		if val, ok := rawMap[name]; ok {
			json.Unmarshal(val, field)
			delete(rawMap, name)
		}
	}
	// Remaining fields go to Extra
	a.Extra = make(map[string]any)
	for k, v := range rawMap {
		var val any
		json.Unmarshal(v, &val)
		a.Extra[k] = val
	}
	return nil
}

type ToolResultContentType string

const (
	ToolResultContentTypeText  ToolResultContentType = "text"
	ToolResultContentTypeImage ToolResultContentType = "image"
	ToolResultContentTypeAudio ToolResultContentType = "audio"
)

func (t ToolResultContentType) String() string {
	return string(t)
}

type ToolResultContent struct {
	Type        ToolResultContentType `json:"type"`
	Text        string                `json:"text,omitempty"`
	Data        string                `json:"data,omitempty"`
	MimeType    string                `json:"mimeType,omitempty"`
	Annotations map[string]any        `json:"annotations,omitempty"`
}

// ToolResult is the output from a tool call.
type ToolResult struct {
	// Content is the tool output sent to the LLM.
	Content []*ToolResultContent `json:"content"`
	// Display is an optional human-readable markdown summary of the result.
	// If empty, consumers should fall back to Content for display.
	Display string `json:"display,omitempty"`
	// IsError indicates whether the tool call resulted in an error.
	IsError bool `json:"isError,omitempty"`
}

// WithDisplay returns a copy of the ToolResult with the Display field set.
func (r *ToolResult) WithDisplay(display string) *ToolResult {
	r.Display = display
	return r
}

// NewToolResultError creates a new ToolResult containing an error message.
func NewToolResultError(text string) *ToolResult {
	return &ToolResult{
		IsError: true,
		Content: []*ToolResultContent{
			{
				Type: ToolResultContentTypeText,
				Text: text,
			},
		},
	}
}

// NewToolResult creates a new ToolResult with the given content.
func NewToolResult(content ...*ToolResultContent) *ToolResult {
	return &ToolResult{Content: content}
}

// NewToolResultText creates a new ToolResult with the given text content.
func NewToolResultText(text string) *ToolResult {
	return NewToolResult(&ToolResultContent{
		Type: ToolResultContentTypeText,
		Text: text,
	})
}

// Tool is an interface for a tool that can be called by an LLM.
type Tool interface {
	// Name of the tool.
	Name() string

	// Description of the tool.
	Description() string

	// Schema describes the parameters used to call the tool.
	Schema() *Schema

	// Annotations returns optional properties that describe tool behavior.
	Annotations() *ToolAnnotations

	// Call is the function that is called to use the tool.
	Call(ctx context.Context, input any) (*ToolResult, error)
}

// ToolCallPreview contains human-readable information about a pending tool call.
type ToolCallPreview struct {
	// Summary is a short description of what the tool will do, e.g., "Fetch https://example.com"
	Summary string `json:"summary"`
	// Details is optional longer markdown with more context about the operation.
	Details string `json:"details,omitempty"`
}

// ToolPreviewer is an optional interface that tools can implement to provide
// human-readable previews of what they will do before execution.
type ToolPreviewer interface {
	// PreviewCall returns a markdown description of what the tool will do
	// given the input. The input is the same type passed to Call().
	PreviewCall(ctx context.Context, input any) *ToolCallPreview
}

// TypedTool is a tool that can be called with a specific type of input.
type TypedTool[T any] interface {
	// Name of the tool.
	Name() string

	// Description of the tool.
	Description() string

	// Schema describes the parameters used to call the tool.
	Schema() *Schema

	// Annotations returns optional properties that describe tool behavior.
	Annotations() *ToolAnnotations

	// Call is the function that is called to use the tool.
	Call(ctx context.Context, input T) (*ToolResult, error)
}

// TypedToolPreviewer is an optional interface that typed tools can implement
// to provide human-readable previews with typed input.
type TypedToolPreviewer[T any] interface {
	// PreviewCall returns a markdown description of what the tool will do.
	PreviewCall(ctx context.Context, input T) *ToolCallPreview
}

// ToolAdapter creates a new TypedToolAdapter for the given tool.
func ToolAdapter[T any](tool TypedTool[T]) *TypedToolAdapter[T] {
	return &TypedToolAdapter[T]{tool: tool}
}

// TypedToolAdapter is an adapter that allows a TypedTool to be used as a regular Tool.
// Specifically the Call method accepts `input any` and then internally unmarshals the input
// to the correct type and passes it to the TypedTool.
type TypedToolAdapter[T any] struct {
	tool TypedTool[T]
}

func (t *TypedToolAdapter[T]) Name() string {
	return t.tool.Name()
}

func (t *TypedToolAdapter[T]) Description() string {
	return t.tool.Description()
}

func (t *TypedToolAdapter[T]) Schema() *Schema {
	return t.tool.Schema()
}

func (t *TypedToolAdapter[T]) Annotations() *ToolAnnotations {
	return t.tool.Annotations()
}

func (t *TypedToolAdapter[T]) Call(ctx context.Context, input any) (*ToolResult, error) {
	typedInput, err := t.convertInput(input)
	if err != nil {
		return NewToolResultError(err.Error()), nil
	}
	return t.tool.Call(ctx, typedInput)
}

// Unwrap returns the underlying TypedTool.
func (t *TypedToolAdapter[T]) Unwrap() TypedTool[T] {
	return t.tool
}

func (t *TypedToolAdapter[T]) ToolConfiguration(providerName string) map[string]any {
	if toolWithConfig, ok := t.tool.(llm.ToolConfiguration); ok {
		return toolWithConfig.ToolConfiguration(providerName)
	}
	return nil
}

// PreviewCall implements ToolPreviewer by delegating to the underlying TypedTool
// if it implements TypedToolPreviewer[T].
func (t *TypedToolAdapter[T]) PreviewCall(ctx context.Context, input any) *ToolCallPreview {
	// Check if underlying tool implements TypedToolPreviewer
	previewer, ok := t.tool.(TypedToolPreviewer[T])
	if !ok {
		return nil
	}

	// Convert input to typed T
	typedInput, err := t.convertInput(input)
	if err != nil {
		return nil
	}

	return previewer.PreviewCall(ctx, typedInput)
}

// convertInput converts any input to the typed T, handling json.RawMessage and other types.
func (t *TypedToolAdapter[T]) convertInput(input any) (T, error) {
	var zero T

	// Pass through if the input is already the correct type
	if converted, ok := input.(T); ok {
		return converted, nil
	}

	// Access the raw JSON
	var data []byte
	var err error
	if raw, ok := input.(json.RawMessage); ok {
		data = raw
	} else if raw, ok := input.([]byte); ok {
		data = raw
	} else if input == nil {
		// Nil input is treated as empty object
		data = []byte("{}")
	} else {
		data, err = json.Marshal(input)
		if err != nil {
			return zero, fmt.Errorf("invalid json for tool %s: %w", t.Name(), err)
		}
	}

	// Handle empty input - treat as empty object
	if len(data) == 0 {
		data = []byte("{}")
	}

	// Unmarshal into the typed input
	var typedInput T
	err = json.Unmarshal(data, &typedInput)
	if err != nil {
		return zero, fmt.Errorf("invalid json for tool %s: %w", t.Name(), err)
	}
	return typedInput, nil
}

// ToolCallResult is a tool call that has been made. This is used to understand
// what calls have happened during an LLM interaction.
type ToolCallResult struct {
	ID      string
	Name    string
	Input   any
	Preview *ToolCallPreview // Preview generated before execution (if tool implements ToolPreviewer)
	Result  *ToolResult
	Error   error
}

