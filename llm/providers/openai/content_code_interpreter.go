package openai

import (
	"encoding/json"

	"github.com/deepnoodle-ai/dive/llm"
)

const ContentTypeCodeInterpreterCall llm.ContentType = "code_interpreter_call"

// CodeInterpreterCallResultFile mirrors openai.ResponseCodeInterpreterToolCallResultFilesFile
// It represents a file output from a code interpreter.
type CodeInterpreterCallResultFile struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
}

// CodeInterpreterCallResult mirrors parts of openai.ResponseCodeInterpreterToolCallResultUnion.
// It represents the content of a single result from a code interpreter execution (e.g., logs or files).
type CodeInterpreterCallResult struct {
	// Type specifies the nature of the result, e.g., "logs" or "files".
	// Corresponds to openai.ResponseCodeInterpreterToolCallResultUnion.Type
	Type string `json:"type"`

	// Logs contains textual output if Type is "logs".
	// Corresponds to openai.ResponseCodeInterpreterToolCallResultLogs.Logs
	Logs string `json:"logs,omitempty"`

	// Files contains a list of file outputs if Type is "files".
	// Corresponds to openai.ResponseCodeInterpreterToolCallResultFiles.Files
	Files []CodeInterpreterCallResultFile `json:"files,omitempty"`
}

// CodeInterpreterCallContent mirrors openai.ResponseCodeInterpreterToolCall.
// This structure is expected as part of an assistant's message when it decides to call the code interpreter tool.
type CodeInterpreterCallContent struct {
	// ID is the unique identifier for this specific tool call.
	// Corresponds to openai.ResponseCodeInterpreterToolCall.ID
	ID string `json:"id"`

	// Code is the Python code the interpreter should execute.
	// Corresponds to openai.ResponseCodeInterpreterToolCall.Code
	Code string `json:"code"`

	// Results contains the output from the interpreter's execution.
	// Corresponds to openai.ResponseCodeInterpreterToolCall.Results
	Results []CodeInterpreterCallResult `json:"results"`

	// Status indicates the execution status, e.g., "in_progress", "interpreting", "completed".
	// Corresponds to openai.ResponseCodeInterpreterToolCall.Status (openai.ResponseCodeInterpreterToolCallStatus)
	Status string `json:"status"`

	// ContainerID is the identifier of the container used for execution, if applicable.
	// Corresponds to openai.ResponseCodeInterpreterToolCall.ContainerID
	ContainerID string `json:"container_id,omitempty"`
}

func (c *CodeInterpreterCallContent) Type() llm.ContentType {
	return ContentTypeCodeInterpreterCall
}

// MarshalJSON ensures that when this content is marshalled, it includes a top-level "type" field.
func (c *CodeInterpreterCallContent) MarshalJSON() ([]byte, error) {
	type Alias CodeInterpreterCallContent
	return json.Marshal(struct {
		Type llm.ContentType `json:"type"`
		*Alias
	}{
		Type:  ContentTypeCodeInterpreterCall,
		Alias: (*Alias)(c),
	})
}

// CodeInterpreterCallResultContent appears to be a wrapper for representing
// the content of a tool result message, possibly for serialization.
type CodeInterpreterCallResultContent struct {
	ToolUseID string                    `json:"tool_use_id"`
	Content   CodeInterpreterCallResult `json:"content"`
}

func (c *CodeInterpreterCallResultContent) Type() llm.ContentType {
	return ContentTypeCodeInterpreterCall
}

// MarshalJSON ensures that when this content is marshalled, it includes a top-level "type" field.
func (c *CodeInterpreterCallResultContent) MarshalJSON() ([]byte, error) {
	type Alias CodeInterpreterCallResultContent
	return json.Marshal(struct {
		Type llm.ContentType `json:"type"` // Fixed: Use llm.ContentType
		*Alias
	}{
		Type:  ContentTypeCodeInterpreterCall, // Sets the outer "type" for this content block
		Alias: (*Alias)(c),
	})
}
