package llm

// Response represents an LLM response
type Response struct {
	id         string
	model      string
	stopReason string
	role       Role
	message    *Message
	usage      Usage
	object     any
	toolCalls  []ToolCall
}

// ID returns the unique identifier of the response
func (r *Response) ID() string { return r.id }

// Model returns the model name that generated the response
func (r *Response) Model() string { return r.model }

// Role returns the role associated with the response
func (r *Response) Role() Role { return r.role }

// Message returns the message content
func (r *Response) Message() *Message { return r.message }

// Usage returns the token usage information
func (r *Response) Usage() Usage { return r.usage }

// Object returns any additional metadata
func (r *Response) Object() any { return r.object }

// ToolCalls returns the tool calls made by the LLM
func (r *Response) ToolCalls() []ToolCall { return r.toolCalls }

// ResponseOptions contains the configuration for creating a new Response
type ResponseOptions struct {
	ID         string
	Model      string
	StopReason string
	Role       Role
	Message    *Message
	Usage      Usage
	ToolCalls  []ToolCall
	Object     any
}

// NewResponse creates a new Response instance with the given options
func NewResponse(opts ResponseOptions) *Response {
	return &Response{
		id:         opts.ID,
		model:      opts.Model,
		stopReason: opts.StopReason,
		role:       opts.Role,
		message:    opts.Message,
		usage:      opts.Usage,
		object:     opts.Object,
		toolCalls:  opts.ToolCalls,
	}
}

// ToolCall is a call made by an LLM
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

// ToolResult contains the result of a tool call
type ToolResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Result string `json:"result"`
}

// ToolError is an error that occurred during a tool call
type ToolError struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}
