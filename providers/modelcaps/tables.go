package modelcaps

import "github.com/deepnoodle-ai/dive/llm"

var (
	// The o-series and its pro/mini variants.
	effortsLowToHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
	}
	// The original gpt-5 family: takes minimal, but not none.
	effortsMinimalToHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortMinimal,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
	}
	// gpt-5.1 onward: none replaces minimal.
	effortsNoneToHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortNone,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
	}
	// gpt-5.2 through gpt-5.5: adds xhigh.
	effortsNoneToXHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortNone,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
	}
	// gpt-5.6: the first OpenAI family to accept max.
	effortsNoneToMax = []llm.ReasoningEffort{
		llm.ReasoningEffortNone,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
		llm.ReasoningEffortMax,
	}
	// gpt-6-astra: none is rejected, so the ladder starts at low and runs to max.
	effortsLowToMax = []llm.ReasoningEffort{
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
		llm.ReasoningEffortMax,
	}
	// The ladder most Grok models accept: everything except max.
	grokBelowMax = []llm.ReasoningEffort{
		llm.ReasoningEffortNone,
		llm.ReasoningEffortMinimal,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
	}
	grokThroughMax = append(append([]llm.ReasoningEffort{}, grokBelowMax...), llm.ReasoningEffortMax)
	// Muse Spark: everything except none (a 400) and max (not offered).
	museMinimalToXHigh = []llm.ReasoningEffort{
		llm.ReasoningEffortMinimal,
		llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh,
		llm.ReasoningEffortXHigh,
	}
)

// openAITable maps OpenAI model-id prefixes to capabilities. It is declared in
// readable order rather than lookup order and so is unexported; callers go
// through Lookup, which searches the longest-prefix-first copy below so that
// "gpt-5" and "gpt-5-pro" can coexist.
var openAITable = Table{
	// No reasoning parameter at all. Sending one is rejected outright, which is
	// what makes a non-empty default effort a request-breaking change.
	{Prefix: "gpt-4o", Caps: Capabilities{Temperature: true}},
	{Prefix: "gpt-4.1", Caps: Capabilities{Temperature: true}},

	{Prefix: "gpt-5", Caps: Capabilities{Efforts: effortsMinimalToHigh}},
	{Prefix: "gpt-5-mini", Caps: Capabilities{Efforts: effortsMinimalToHigh}},
	{Prefix: "gpt-5-nano", Caps: Capabilities{Efforts: effortsMinimalToHigh}},
	// gpt-5-pro accepts high and nothing else.
	{Prefix: "gpt-5-pro", Caps: Capabilities{
		Efforts: []llm.ReasoningEffort{llm.ReasoningEffortHigh},
	}},

	{Prefix: "gpt-5.1", Caps: Capabilities{Efforts: effortsNoneToHigh, Temperature: true}},

	{Prefix: "gpt-5.2", Caps: Capabilities{Efforts: effortsNoneToXHigh, Temperature: true}},
	// The pro variant narrows the range rather than widening it: no none, no low.
	{Prefix: "gpt-5.2-pro", Caps: Capabilities{
		Efforts: []llm.ReasoningEffort{
			llm.ReasoningEffortMedium,
			llm.ReasoningEffortHigh,
			llm.ReasoningEffortXHigh,
		},
	}},

	// The chat-tuned model accepts medium and nothing else.
	{Prefix: "gpt-5.3-chat", Caps: Capabilities{
		Efforts: []llm.ReasoningEffort{llm.ReasoningEffortMedium},
	}},
	{Prefix: "gpt-5.3-codex", Caps: Capabilities{Efforts: effortsNoneToXHigh, Temperature: true}},

	{Prefix: "gpt-5.4", Caps: Capabilities{Efforts: effortsNoneToXHigh, Temperature: true}},
	{Prefix: "gpt-5.4-mini", Caps: Capabilities{Efforts: effortsNoneToXHigh, Temperature: true}},
	{Prefix: "gpt-5.4-nano", Caps: Capabilities{Efforts: effortsNoneToXHigh, Temperature: true}},

	{Prefix: "gpt-5.5", Caps: Capabilities{Efforts: effortsNoneToXHigh}},

	{Prefix: "gpt-5.6", Caps: Capabilities{Efforts: effortsNoneToMax}},
	{Prefix: "gpt-5.6-sol", Caps: Capabilities{Efforts: effortsNoneToMax}},
	{Prefix: "gpt-5.6-terra", Caps: Capabilities{Efforts: effortsNoneToMax}},
	{Prefix: "gpt-5.6-luna", Caps: Capabilities{Efforts: effortsNoneToMax}},

	// Documented rather than probed: gpt-6-astra is gated to the Trusted Access
	// Program and answers "does not exist" for accounts outside it, so this
	// entry comes from OpenAI's release notes, not a 200/400 from the endpoint.
	// It is recorded rather than left Unverified because the two exclusions are
	// stated outright -- no "none" effort, and no custom temperature, top_p, or
	// logprobs -- and passing those through would send a request already known
	// to fail. Re-probe once the model is reachable.
	{Prefix: "gpt-6-astra", Caps: Capabilities{Efforts: effortsLowToMax}},

	{Prefix: "o3", Caps: Capabilities{Efforts: effortsLowToHigh}},
	{Prefix: "o3-pro", Caps: Capabilities{Efforts: effortsLowToHigh}},
	{Prefix: "o3-mini", Caps: Capabilities{Efforts: effortsLowToHigh}},
	{Prefix: "o4-mini", Caps: Capabilities{Efforts: effortsLowToHigh}},

	// Catalogued but unreachable for verification ("does not exist" or 404 for
	// the probing account). Left as passthrough rather than guessed at from a
	// sibling — the gpt-5.2-pro and gpt-5.3-chat entries above show that
	// variants do not reliably inherit their family's range.
	{Prefix: "o3-deep-research", Unverified: true},
	{Prefix: "o4-mini-deep-research", Unverified: true},
	{Prefix: "gpt-5-codex", Unverified: true},
	{Prefix: "gpt-5-codex-mini", Unverified: true},
	{Prefix: "gpt-5.1-codex", Unverified: true},
	{Prefix: "gpt-5.1-codex-max", Unverified: true},
	{Prefix: "gpt-5.2-codex", Unverified: true},
	{Prefix: "codex-mini-latest", Unverified: true},
	{Prefix: "codex-ask", Unverified: true},
}

// grokTable maps xAI model-id prefixes to capabilities, unexported for the same
// reason as openAITable. Grok is broadly more permissive than OpenAI — most
// models take the full ladder below max — but several reject the reasoning
// parameter entirely, including one whose name says "reasoning".
var grokTable = Table{
	// "does not support parameter reasoningEffort". Note that
	// grok-4.20-0309-reasoning is among them despite its name.
	{Prefix: "grok-4.20-0309-reasoning", Caps: Capabilities{Temperature: true}},
	{Prefix: "grok-4.20-0309-non-reasoning", Caps: Capabilities{Temperature: true}},
	{Prefix: "grok-build", Caps: Capabilities{Temperature: true}},
	{Prefix: "grok-code-fast", Caps: Capabilities{Temperature: true}},

	// grok-4.5 is the one Grok model that rejects none.
	{Prefix: "grok-4.5", Caps: Capabilities{
		Efforts: []llm.ReasoningEffort{
			llm.ReasoningEffortMinimal,
			llm.ReasoningEffortLow,
			llm.ReasoningEffortMedium,
			llm.ReasoningEffortHigh,
			llm.ReasoningEffortXHigh,
		},
		Temperature: true,
	}},

	// grok-4.6 narrows further still: xAI's docs list only low, medium,
	// high (the default), and xhigh — neither none nor minimal.
	{Prefix: "grok-4.6", Caps: Capabilities{
		Efforts: []llm.ReasoningEffort{
			llm.ReasoningEffortLow,
			llm.ReasoningEffortMedium,
			llm.ReasoningEffortHigh,
			llm.ReasoningEffortXHigh,
		},
		Temperature: true,
	}},

	{Prefix: "grok-4.3", Caps: Capabilities{Efforts: grokBelowMax, Temperature: true}},
	{Prefix: "grok-4", Caps: Capabilities{Efforts: grokBelowMax, Temperature: true}},
	{Prefix: "grok-3", Caps: Capabilities{Efforts: grokBelowMax, Temperature: true}},

	// The multi-agent model is the only Grok model that accepts max.
	{Prefix: "grok-4.20-multi-agent", Caps: Capabilities{
		Efforts: grokThroughMax, Temperature: true,
	}},
}

// museTable maps Meta Model API model-id prefixes to capabilities, unexported
// for the same reason as openAITable. The Muse Spark ladder is unusual at both
// ends: "none" is rejected with HTTP 400 rather than ignored — Meta documents
// Muse Spark as a reasoning model that cannot be asked to stop reasoning — and
// "max" is not offered at all, so the ladder runs minimal through xhigh.
// Omitting the parameter is still legal and lets the model pick its own depth.
var museTable = Table{
	{Prefix: "muse-spark-1.1", Caps: Capabilities{Efforts: museMinimalToXHigh, Temperature: true}},
	{Prefix: "muse-spark-1.2", Caps: Capabilities{Efforts: museMinimalToXHigh, Temperature: true}},
	{Prefix: "muse-spark-1.2-contributor", Caps: Capabilities{Efforts: museMinimalToXHigh, Temperature: true}},
	{Prefix: "muse-spark-1.3", Caps: Capabilities{Efforts: museMinimalToXHigh, Temperature: true}},
	{Prefix: "muse-spark-1.3-contributor", Caps: Capabilities{Efforts: museMinimalToXHigh, Temperature: true}},
}

var (
	sortedOpenAI = sortByPrefixLength(openAITable)
	sortedGrok   = sortByPrefixLength(grokTable)
	sortedMuse   = sortByPrefixLength(museTable)
)
