package anthropic

import (
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// modelCapabilities records which reasoning, thinking, and sampling parameters a
// model accepts. Every field is a fact about the API, not a preference: each was
// verified by sending the parameter to the live endpoint and recording whether
// it returned 200 or 400.
//
// The zero value describes a model with no reasoning support at all, which is
// the safe default — a model absent from the table is treated as unknown
// instead, and its parameters pass through untouched.
type modelCapabilities struct {
	// efforts lists the levels output_config.effort accepts. Empty means the
	// model has no native effort parameter.
	efforts []llm.ReasoningEffort

	// manualBudget reports whether thinking:{type:"enabled",budget_tokens:N} is
	// accepted. Models with no native effort parameter but a manual budget
	// emulate effort with a budget; models with neither cannot reason at all.
	manualBudget bool

	// adaptive reports whether thinking:{type:"adaptive"} is accepted. The 4.5
	// generation predates adaptive thinking and returns
	// "adaptive thinking is not supported on this model".
	adaptive bool

	// explicitDisable reports whether thinking:{type:"disabled"} is accepted.
	// Fable 5 and Mythos 5 reject it; for them thinking is omitted instead.
	explicitDisable bool

	// thinkingOnByDefault reports whether omitting the thinking parameter still
	// leaves thinking active. These models need an explicit disable to turn it
	// off, where omission suffices everywhere else.
	thinkingOnByDefault bool

	// disabledEffortCap caps effort while thinking is explicitly disabled. Opus
	// 5 answers 400 for xhigh or max paired with a disable but accepts high and
	// below. Empty means no cap.
	disabledEffortCap llm.ReasoningEffort

	// temperature reports whether the temperature parameter is accepted.
	temperature bool
}

// reasoningKind classifies how the model takes a reasoning effort.
type reasoningKind int

const (
	// reasoningNone: the model cannot reason; effort must be dropped.
	reasoningNone reasoningKind = iota
	// reasoningLegacyBudget: no native effort parameter, but effort can be
	// emulated with a thinking budget.
	reasoningLegacyBudget
	// reasoningNative: the model takes output_config.effort directly.
	reasoningNative
)

func (c modelCapabilities) reasoningKind() reasoningKind {
	switch {
	case len(c.efforts) > 0:
		return reasoningNative
	case c.manualBudget:
		return reasoningLegacyBudget
	default:
		return reasoningNone
	}
}

var (
	effortsThroughHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
	}
	// The 4.6 generation accepts max but rejects xhigh — an ordering that looks
	// like a typo and is not. Verified against the live API.
	effortsThroughHighPlusMax = []llm.ReasoningEffort{
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortMax,
	}
	effortsFull = []llm.ReasoningEffort{
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
		llm.ReasoningEffortMax,
	}
)

type capabilityEntry struct {
	prefix string
	caps   modelCapabilities
}

// modelCapabilityTable maps model-id prefixes to capabilities. Lookup takes the
// longest matching prefix, so a family entry such as "claude-opus-4" can sit
// alongside the narrower "claude-opus-4-5" without ordering hazards.
//
// Every model id in catalog.json must resolve to an entry here;
// TestEveryCatalogModelHasCapabilities enforces it, so a newly added model
// fails the build until it has been classified.
var modelCapabilityTable = []capabilityEntry{
	// --- Retired. These return 404 at inference and are kept only so the
	// catalog stays fully classified; the values are historical. ---
	{prefix: "claude-3-5-haiku", caps: modelCapabilities{temperature: true}},
	{prefix: "claude-3-5-sonnet", caps: modelCapabilities{temperature: true}},
	{prefix: "claude-3-7-sonnet", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},
	{prefix: "claude-sonnet-4", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},
	{prefix: "claude-opus-4", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},
	{prefix: "claude-opus-4-1", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},

	// --- 4.5: manual budgets only. No native effort, no adaptive thinking. ---
	{prefix: "claude-haiku-4-5", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},
	{prefix: "claude-sonnet-4-5", caps: modelCapabilities{
		manualBudget: true, explicitDisable: true, temperature: true,
	}},
	// Opus 4.5 introduced the effort parameter but still predates adaptive
	// thinking, and accepts a budget and an effort in the same request.
	{prefix: "claude-opus-4-5", caps: modelCapabilities{
		efforts: effortsThroughHigh, manualBudget: true,
		explicitDisable: true, temperature: true,
	}},

	// --- 4.6: adds adaptive thinking and max effort; still takes temperature. ---
	{prefix: "claude-sonnet-4-6", caps: modelCapabilities{
		efforts: effortsThroughHighPlusMax, manualBudget: true,
		adaptive: true, explicitDisable: true, temperature: true,
	}},
	{prefix: "claude-opus-4-6", caps: modelCapabilities{
		efforts: effortsThroughHighPlusMax, manualBudget: true,
		adaptive: true, explicitDisable: true, temperature: true,
	}},

	// --- 4.7/4.8: adaptive-only thinking; temperature and manual budgets rejected. ---
	{prefix: "claude-opus-4-7", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, explicitDisable: true,
	}},
	{prefix: "claude-opus-4-8", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, explicitDisable: true,
	}},

	// --- Claude 5: thinking on by default, adaptive-only, no temperature. ---
	// Opus 5 alone caps effort at high while thinking is explicitly disabled.
	{prefix: "claude-opus-5", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, explicitDisable: true,
		thinkingOnByDefault: true, disabledEffortCap: llm.ReasoningEffortHigh,
	}},
	{prefix: "claude-sonnet-5", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, explicitDisable: true,
		thinkingOnByDefault: true,
	}},
	// Fable 5 and Mythos 5 always think and reject an explicit disable, so
	// Dive omits the thinking parameter for them instead. The 5.1 point
	// releases behave identically here and deliberately share these entries by
	// prefix: "claude-fable-5-1" matches "claude-fable-5", and likewise for
	// Mythos. Their added restriction — forced tool_choice returns a 400 — is
	// already covered, since requestThinkingBlocksForcedToolChoice rejects a
	// forced choice for any model that always thinks and cannot be disabled.
	{prefix: "claude-fable-5", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, thinkingOnByDefault: true,
	}},
	// Mythos 5 and 5.1 are catalogued but reachable only through Anthropic's
	// limited-availability program, so these values mirror Fable and are
	// untested against the live API.
	{prefix: "claude-mythos-5", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, thinkingOnByDefault: true,
	}},
	{prefix: "claude-mythos-preview", caps: modelCapabilities{
		efforts: effortsFull, adaptive: true, thinkingOnByDefault: true,
	}},
}

// sortedCapabilityTable is modelCapabilityTable ordered longest-prefix-first so
// lookup can return the first match.
var sortedCapabilityTable = func() []capabilityEntry {
	entries := make([]capabilityEntry, len(modelCapabilityTable))
	copy(entries, modelCapabilityTable)
	sort.SliceStable(entries, func(i, j int) bool {
		return len(entries[i].prefix) > len(entries[j].prefix)
	})
	return entries
}()

// effortsUpTo returns the levels of an ordered ladder up to and including the
// given cap. It exists so a cap can only ever lower a requested effort: passing
// the cap alone as the supported set would clamp a below-cap request *up* to it.
func effortsUpTo(efforts []llm.ReasoningEffort, capLevel llm.ReasoningEffort) []llm.ReasoningEffort {
	for i, level := range efforts {
		if level == capLevel {
			return efforts[:i+1]
		}
	}
	return efforts
}

// lookupCapabilities returns the capabilities for a model id. The bool reports
// whether the model is known: an unknown model — a fine-tune, a custom
// deployment, or a base-URL passthrough — gets its parameters forwarded
// untouched, since Dive cannot tell what it accepts.
//
// Known limitation: matching is a plain prefix test, so an unreleased point
// release inherits its family's entry — a future "claude-opus-4-9" would match
// "claude-opus-4" and be treated as legacy-budget. providers/modelcaps guards
// the same hazard by requiring the next character to begin a variant suffix,
// but that rule cannot work here: Anthropic writes versions with the same "-"
// that separates variants and dates ("claude-opus-4-5-20251101"), so there is
// nothing to key on. Adding a model to catalog.json therefore means checking
// that it lands on the right entry, not only that it lands on one.
func lookupCapabilities(model string) (modelCapabilities, bool) {
	id := strings.ToLower(strings.TrimSpace(model))
	// Accept vendor-qualified ids such as "anthropic/claude-sonnet-5" and
	// doubly-qualified ones such as "openrouter/anthropic/claude-sonnet-5":
	// only the final segment names the model.
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		id = id[idx+1:]
	}
	for _, entry := range sortedCapabilityTable {
		if strings.HasPrefix(id, entry.prefix) {
			return entry.caps, true
		}
	}
	return modelCapabilities{}, false
}
