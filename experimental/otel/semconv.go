package otel

// GenAI semantic convention attribute keys.
//
// Tracks the OTel GenAI semconv as of 2026-04. The conventions are still
// flagged "experimental" in OTel; we mirror them verbatim so destinations
// (Datadog, Honeycomb, Phoenix, Langfuse, Mobius) decode out of the box.
const (
	AttrMobiusRunID   = "mobius.run.id"
	AttrMobiusStepID  = "mobius.step.id"
	AttrMobiusJobID   = "mobius.job.id"
	AttrMobiusAgentID = "mobius.agent.id"
)

const (
	AttrGenAISystem        = "gen_ai.system"
	AttrGenAIOperationName = "gen_ai.operation.name"

	AttrGenAIRequestModel       = "gen_ai.request.model"
	AttrGenAIRequestMaxTokens   = "gen_ai.request.max_tokens"
	AttrGenAIRequestTemperature = "gen_ai.request.temperature"

	AttrGenAIResponseModel         = "gen_ai.response.model"
	AttrGenAIResponseID            = "gen_ai.response.id"
	AttrGenAIResponseFinishReasons = "gen_ai.response.finish_reasons"

	AttrGenAIUsageInputTokens  = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputTokens = "gen_ai.usage.output_tokens"

	// gen_ai.input.messages and gen_ai.output.messages are opt-in (privacy).
	AttrGenAIInputMessages      = "gen_ai.input.messages"
	AttrGenAIOutputMessages     = "gen_ai.output.messages"
	AttrGenAISystemInstructions = "gen_ai.system_instructions"

	AttrGenAIToolName       = "gen_ai.tool.name"
	AttrGenAIToolType       = "gen_ai.tool.type"
	AttrGenAIToolCallID     = "gen_ai.tool.call.id"
	AttrGenAIToolCallArgs   = "gen_ai.tool.call.arguments"
	AttrGenAIToolCallResult = "gen_ai.tool.call.result"

	AttrGenAIAgentName        = "gen_ai.agent.name"
	AttrGenAIAgentDescription = "gen_ai.agent.description"

	AttrErrorType = "error.type"
)

// GenAI operation names.
const (
	OperationChat        = "chat"
	OperationExecuteTool = "execute_tool"
	OperationInvokeAgent = "invoke_agent"
)
