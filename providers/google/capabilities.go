package google

import (
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// dynamicThinkingBudget asks the model to choose its own budget.
const dynamicThinkingBudget = -1

// modelCapabilities records which thinking parameters a Gemini model accepts.
// Every field was verified against the live API by sending the parameter and
// recording whether it returned 200 or 400 — including the budget bounds, which
// the API reports in its error text and which differ per model.
type modelCapabilities struct {
	// efforts lists the levels thinkingConfig.thinkingLevel accepts, expressed
	// in Dive's provider-neutral terms. Empty means the model has no thinking
	// level at all ("Thinking level is not supported for this model") and
	// effort must be emulated with a budget. Gemini's ladder tops out at HIGH,
	// so no model lists xhigh or max.
	efforts []llm.ReasoningEffort

	// minBudget and maxBudget bound thinkingBudget. They vary by model: the 3.x
	// generation takes [1, 65535], gemini-2.5-pro [128, 32768], and
	// gemini-2.5-flash-lite [512, 24576].
	minBudget int
	maxBudget int

	// canDisableThinking reports whether thinkingBudget: 0 is accepted. It is a
	// value outside [minBudget, maxBudget] and varies within a family rather
	// than by generation. Models that always think answer 400, so a request to
	// disable thinking has to degrade instead.
	canDisableThinking bool

	// unverified marks a model that is catalogued but could not be reached — it
	// answered 404 "no longer available". Lookup reports it as unknown so its
	// parameters pass through untouched rather than being guessed at.
	unverified bool
}

// supportsThinkingLevel reports whether the model takes thinkingConfig.thinkingLevel.
func (c modelCapabilities) supportsThinkingLevel() bool {
	return len(c.efforts) > 0
}

// effortsThroughHigh is Gemini 3.x's full ladder: MINIMAL, LOW, MEDIUM, HIGH.
var effortsThroughHigh = []llm.ReasoningEffort{
	llm.ReasoningEffortMinimal,
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
}

// effortsLowThroughHigh drops MINIMAL, which the Pro models reject with
// "Thinking level MINIMAL is not supported for this model".
var effortsLowThroughHigh = []llm.ReasoningEffort{
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
}

type capabilityEntry struct {
	prefix string
	caps   modelCapabilities
}

// modelCapabilityTable maps Gemini model-id prefixes to capabilities. Lookup
// takes the longest matching prefix.
//
// Every model id in catalog.json must resolve to an entry here;
// TestEveryCatalogModelHasCapabilities enforces it, so a newly added model
// fails the build until it has been classified. That matters more than usual
// on Gemini: thinking support varies *within* a family, not just by
// generation — gemini-3.5-flash can turn thinking off and gemini-3.5-flash-lite
// cannot, and the 2.5 generation has no thinking level at all.
var modelCapabilityTable = []capabilityEntry{
	// --- 3.x: thinking levels, budgets in [1, 65535]. ---
	{prefix: "gemini-3.6-flash", caps: modelCapabilities{
		efforts: effortsThroughHigh, minBudget: 1, maxBudget: 65535,
	}},
	{prefix: "gemini-3.5-flash", caps: modelCapabilities{
		efforts: effortsThroughHigh, minBudget: 1, maxBudget: 65535,
		canDisableThinking: true,
	}},
	{prefix: "gemini-3.5-flash-lite", caps: modelCapabilities{
		efforts: effortsThroughHigh, minBudget: 1, maxBudget: 65535,
	}},
	{prefix: "gemini-3.1-flash-lite", caps: modelCapabilities{
		efforts: effortsThroughHigh, minBudget: 1, maxBudget: 65535,
		canDisableThinking: true,
	}},
	{prefix: "gemini-3-flash", caps: modelCapabilities{
		efforts: effortsThroughHigh, minBudget: 1, maxBudget: 65535,
		canDisableThinking: true,
	}},
	// Pro rejects MINIMAL and cannot turn thinking off.
	{prefix: "gemini-3.1-pro", caps: modelCapabilities{
		efforts: effortsLowThroughHigh, minBudget: 1, maxBudget: 65535,
	}},

	// --- 2.5: budgets only. "Thinking level is not supported for this model." ---
	{prefix: "gemini-2.5-pro", caps: modelCapabilities{
		minBudget: 128, maxBudget: 32768,
	}},
	{prefix: "gemini-2.5-flash", caps: modelCapabilities{
		minBudget: 0, maxBudget: 24576, canDisableThinking: true,
	}},
	{prefix: "gemini-2.5-flash-lite", caps: modelCapabilities{
		minBudget: 512, maxBudget: 24576, canDisableThinking: true,
	}},

	// --- Retired: 404 "no longer available". Passthrough rather than guessed. ---
	{prefix: "gemini-3-pro-preview", caps: modelCapabilities{unverified: true}},
	{prefix: "gemini-2.0-flash", caps: modelCapabilities{unverified: true}},
	{prefix: "gemini-1.5-pro", caps: modelCapabilities{unverified: true}},
	{prefix: "gemini-1.5-flash", caps: modelCapabilities{unverified: true}},
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

func normalizeModelID(model string) string {
	id := strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(id, "models/")
}

// lookupCapabilities returns the capabilities for a model id. The bool reports
// whether the model is known and verified: anything else — a preview id, a
// tuned model, a Vertex deployment, a retired model — gets its parameters
// forwarded untouched, since Dive cannot tell what it accepts.
func lookupCapabilities(model string) (modelCapabilities, bool) {
	entry, found := lookupEntry(model)
	if !found || entry.caps.unverified {
		return modelCapabilities{}, false
	}
	return entry.caps, true
}

// lookupEntry returns the raw entry, including unverified ones. It backs the
// coverage test, which requires every catalogued model to be classified — even
// when the classification is "could not verify".
func lookupEntry(model string) (capabilityEntry, bool) {
	id := normalizeModelID(model)
	for _, entry := range sortedCapabilityTable {
		if strings.HasPrefix(id, entry.prefix) {
			return entry, true
		}
	}
	return capabilityEntry{}, false
}

// modelAcceptsTemperature reports whether Dive forwards a temperature for this
// model. The rule lives in shouldOmitTemperature, which keys on the request
// generation and so also covers models that do not exist yet.
//
// Do not "fix" this from probe results alone. Gemini 3.5 Flash-Lite and 3.6+
// accept a temperature and even range-validate it — sending 5.0 returns
// "temperature must be in the range [0.0, 2.0]" — so a probe that only records
// status codes concludes the parameter is live. It is not honored. Sampling the
// same prompt eight times shows gemini-3.5-flash pinned to a single answer at
// temperature 0 while gemini-3.6-flash keeps varying, with thinking held at the
// same level in both. The parameter is accepted and ignored, and withholding it
// is correct.
func modelAcceptsTemperature(model string) bool {
	return !shouldOmitTemperature(model)
}
