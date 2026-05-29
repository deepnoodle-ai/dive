package openai

import "github.com/openai/openai-go/v3/responses"

// ResponsesToolProvider is implemented by tools that provide native OpenAI
// Responses API tool parameters. This allows server-side tools (like web search
// or provider-specific tools) to be passed directly to the API without being
// wrapped as function tools.
type ResponsesToolProvider interface {
	ResponsesToolParam() responses.ToolUnionParam
}

// ResponsesIncludeProvider is an optional interface that a ResponsesToolProvider
// may also implement to request additional data be returned in the response via
// the Responses API `include` parameter (e.g. "code_interpreter_call.outputs" or
// "file_search_call.results"). The provider merges these with any other includes
// it sets for the request.
type ResponsesIncludeProvider interface {
	ResponsesIncludes() []responses.ResponseIncludable
}
