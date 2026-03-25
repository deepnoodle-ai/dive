package openai

import "github.com/openai/openai-go/v3/responses"

// ResponsesToolProvider is implemented by tools that provide native OpenAI
// Responses API tool parameters. This allows server-side tools (like web search
// or provider-specific tools) to be passed directly to the API without being
// wrapped as function tools.
type ResponsesToolProvider interface {
	ResponsesToolParam() responses.ToolUnionParam
}
