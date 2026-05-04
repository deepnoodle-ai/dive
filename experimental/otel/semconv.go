package otel

// Vendor-extension attribute keys. Not part of OTel GenAI semconv — these are
// resource-style identifiers Mobius uses to correlate spans with workflow
// runs. The OTel GenAI keys (gen_ai.*, server.*, error.type) live in
// go.opentelemetry.io/otel/semconv/v1.40.0 and .../v1.40.0/genaiconv.
const (
	AttrMobiusRunID   = "mobius.run.id"
	AttrMobiusStepID  = "mobius.step.id"
	AttrMobiusJobID   = "mobius.job.id"
	AttrMobiusAgentID = "mobius.agent.id"
)

// Legacy gen_ai.system attribute key. The OTel GenAI spec migrated to
// gen_ai.provider.name (semconv.GenAIProviderNameKey). The extension emits
// both during the deprecation window so backends keyed on either name keep
// working.
const attrGenAISystem = "gen_ai.system"
