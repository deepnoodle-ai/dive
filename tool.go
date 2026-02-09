package dive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

// ToolAnnotations contains optional metadata hints that describe a tool's behavior.
// These hints help agents and permission systems make decisions about tool usage.
type ToolAnnotations struct {
	Title              string         `json:"title,omitempty"`
	ReadOnlyHint       bool           `json:"readOnlyHint,omitempty"`
	DestructiveHint    bool           `json:"destructiveHint,omitempty"`
	IdempotentHint     bool           `json:"idempotentHint,omitempty"`
	OpenWorldHint      bool           `json:"openWorldHint,omitempty"`
	EditHint           bool           `json:"editHint,omitempty"`           // Indicates file edit operations for acceptEdits mode
	Extra map[string]any `json:"extra,omitempty"`
}

func (a *ToolAnnotations) MarshalJSON() ([]byte, error) {
	data := map[string]any{
		"title":              a.Title,
		"readOnlyHint":       a.ReadOnlyHint,
		"destructiveHint":    a.DestructiveHint,
		"idempotentHint":     a.IdempotentHint,
		"openWorldHint":      a.OpenWorldHint,
		"editHint": a.EditHint,
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
		"readOnlyHint":       &a.ReadOnlyHint,
		"destructiveHint":    &a.DestructiveHint,
		"idempotentHint":     &a.IdempotentHint,
		"openWorldHint": &a.OpenWorldHint,
		"editHint":      &a.EditHint,
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

// ToolResultContentType indicates the media type of a tool result content block.
type ToolResultContentType string

const (
	ToolResultContentTypeText  ToolResultContentType = "text"
	ToolResultContentTypeImage ToolResultContentType = "image"
	ToolResultContentTypeAudio ToolResultContentType = "audio"
)

func (t ToolResultContentType) String() string {
	return string(t)
}

// ToolResultContent is a single content block within a tool result, such as
// text output, an image, or audio data.
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

// WithDisplay sets the Display field and returns the receiver for chaining.
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

// ToolConfiguration delegates to the underlying tool's ToolConfiguration method
// if it implements the llm.ToolConfiguration interface.
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

// Toolset provides dynamic tool resolution. Tools() is called before each
// LLM request, allowing the available tools to vary based on runtime context.
// Use toolsets for MCP servers, permission-filtered tools, or context-dependent
// tool availability.
type Toolset interface {
	// Name identifies this toolset for logging and debugging.
	Name() string

	// Tools returns the tools available in the current context.
	// Called before each LLM request. Implementations should cache tool
	// instances and avoid re-creating them on every call.
	Tools(ctx context.Context) ([]Tool, error)
}

// ToolsetFunc adapts a function into a Toolset.
type ToolsetFunc struct {
	// ToolsetName identifies this toolset.
	ToolsetName string
	// Resolve returns the tools for the current context.
	Resolve func(ctx context.Context) ([]Tool, error)
}

// Name returns the toolset name.
func (f *ToolsetFunc) Name() string { return f.ToolsetName }

// Tools calls the resolve function.
func (f *ToolsetFunc) Tools(ctx context.Context) ([]Tool, error) {
	return f.Resolve(ctx)
}

// FuncTool creates a Tool from a function with an auto-generated schema.
//
// The schema is generated from the input type T using struct tags. Use json
// tags for field names, description tags for parameter descriptions, and
// omitempty to mark optional fields. See [schema.Generate] for all supported tags.
//
// Example:
//
//	type WeatherInput struct {
//	    City  string `json:"city" description:"City name"`
//	    Units string `json:"units,omitempty" description:"Temperature units" enum:"celsius,fahrenheit"`
//	}
//
//	weatherTool := dive.FuncTool("get_weather", "Get current weather",
//	    func(ctx context.Context, input *WeatherInput) (*dive.ToolResult, error) {
//	        return dive.NewToolResultText("72°F"), nil
//	    },
//	)
func FuncTool[T any](name, description string, fn func(ctx context.Context, input T) (*ToolResult, error), opts ...FuncToolOption) Tool {
	ft := &funcTool[T]{
		name:        name,
		description: description,
		fn:          fn,
	}
	for _, opt := range opts {
		opt.applyFuncTool(ft)
	}
	// Auto-generate schema from T if not overridden
	if ft.schema == nil {
		var zero T
		s, err := schema.Generate(zero)
		if err != nil {
			// If schema generation fails, store the error and report at call time
			ft.schemaErr = fmt.Errorf("FuncTool %q: cannot generate schema from %T: %w", name, zero, err)
			ft.schema = &Schema{Type: Object}
		} else {
			ft.schema = s
		}
	}
	return ToolAdapter(ft)
}

// FuncToolOption configures a FuncTool.
type FuncToolOption interface {
	applyFuncTool(ft any)
}

// WithFuncToolAnnotations sets annotations on a FuncTool.
func WithFuncToolAnnotations(a *ToolAnnotations) FuncToolOption {
	return &withAnnotationsOption{annotations: a}
}

type withAnnotationsOption struct {
	annotations *ToolAnnotations
}

func (o *withAnnotationsOption) applyFuncTool(ft any) {
	// Use reflection-free approach via the interface
	if s, ok := ft.(interface{ setAnnotations(*ToolAnnotations) }); ok {
		s.setAnnotations(o.annotations)
	}
}

// WithFuncToolSchema overrides the auto-generated schema.
func WithFuncToolSchema(s *Schema) FuncToolOption {
	return &withSchemaOption{schema: s}
}

type withSchemaOption struct {
	schema *Schema
}

func (o *withSchemaOption) applyFuncTool(ft any) {
	if s, ok := ft.(interface{ setSchema(*Schema) }); ok {
		s.setSchema(o.schema)
	}
}

// funcTool is the internal implementation backing FuncTool.
type funcTool[T any] struct {
	name        string
	description string
	fn          func(ctx context.Context, input T) (*ToolResult, error)
	annotations *ToolAnnotations
	schema      *Schema
	schemaErr   error
}

func (f *funcTool[T]) setAnnotations(a *ToolAnnotations) { f.annotations = a }
func (f *funcTool[T]) setSchema(s *Schema)               { f.schema = s }

func (f *funcTool[T]) Name() string        { return f.name }
func (f *funcTool[T]) Description() string { return f.description }
func (f *funcTool[T]) Schema() *Schema     { return f.schema }

func (f *funcTool[T]) Annotations() *ToolAnnotations {
	return f.annotations
}

func (f *funcTool[T]) Call(ctx context.Context, input T) (*ToolResult, error) {
	if f.schemaErr != nil {
		return NewToolResultError(f.schemaErr.Error()), nil
	}
	return f.fn(ctx, input)
}

// ToolCallResult is a tool call that has been made. This is used to understand
// what calls have happened during an LLM interaction.
//
// Error and Result.IsError track different failure modes:
//   - Error is a Go error from tool.Call() — the tool itself crashed or failed unexpectedly.
//   - Result.IsError is a protocol-level flag — the tool ran but reported a failure to the LLM
//     (e.g. via NewToolResultError). Both are surfaced to the LLM as an error result.
type ToolCallResult struct {
	ID                string
	Name              string
	Input             any
	Preview           *ToolCallPreview // Preview generated before execution (if tool implements ToolPreviewer)
	Result            *ToolResult      // Protocol-level result sent to the LLM
	Error             error            // Go error if tool.Call() itself failed
	AdditionalContext string           // Context injected by hooks, appended to the tool result message
}
