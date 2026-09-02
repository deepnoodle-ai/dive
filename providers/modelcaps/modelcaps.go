// Package modelcaps records which reasoning and sampling parameters each model
// accepts, so providers can clamp or drop a setting a model cannot take instead
// of sending a request the API will reject.
//
// Every entry is a fact about the API, verified by sending the parameter to the
// live endpoint and recording whether it returned 200 or 400. Nothing here is
// inferred from a model's name or generation — several models contradict what
// their names suggest, and a few contradict their own families.
//
// The tables live here rather than in each provider package because the OpenAI
// Responses API, the Chat Completions API, and OpenRouter all serve the same
// models and previously carried separate, separately-wrong copies.
package modelcaps

import (
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// Capabilities describes the parameters a single model accepts.
type Capabilities struct {
	// Efforts lists the levels reasoning.effort accepts. Empty means the model
	// has no reasoning parameter at all, and sending one is rejected with
	// "Unsupported parameter: 'reasoning.effort'".
	Efforts []llm.ReasoningEffort

	// Temperature reports whether the temperature parameter is accepted.
	Temperature bool
}

// Entry binds a model-id prefix to its capabilities.
type Entry struct {
	Prefix string
	Caps   Capabilities

	// Unverified marks a model that is catalogued but could not be reached for
	// verification — it answered "does not exist" or 404. Lookup reports it as
	// unknown so its parameters pass through untouched, rather than having its
	// behavior guessed at from a sibling model.
	Unverified bool
}

// Table is a set of entries, searched by longest matching prefix.
type Table []Entry

func sortByPrefixLength(entries Table) Table {
	out := make(Table, len(entries))
	copy(out, entries)
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Prefix) > len(out[j].Prefix)
	})
	return out
}

// NormalizeModelID lowercases a model id and strips the vendor prefixes that
// OpenRouter-style ids carry.
func NormalizeModelID(model string) string {
	id := strings.ToLower(strings.TrimSpace(model))
	id = strings.TrimPrefix(id, "openai/")
	id = strings.TrimPrefix(id, "x-ai/")
	return id
}

// matchesPrefix reports whether an id belongs to the family a prefix names.
//
// A plain strings.HasPrefix is not enough: OpenAI and xAI separate versions with
// "." and variants with "-", so "gpt-5" prefixes "gpt-5.7" and "grok-4" prefixes
// "grok-4.7". Inheriting a family's ladder that way silently misclassifies the
// next point release — gpt-5.7 would be capped at gpt-5's "high" — and the
// catalog coverage test would still pass, because the id does resolve to an
// entry. Requiring the next character to start a variant or date suffix keeps
// "gpt-5-pro", "o4-mini-deep-research" and "grok-4-1-fast-reasoning" matching
// while letting unknown point releases fall through as unknown.
func matchesPrefix(id, prefix string) bool {
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	rest := id[len(prefix):]
	return rest == "" || rest[0] == '-'
}

// TableFor picks the table for a provider and model. It consults the model id
// as well as the provider name, because OpenRouter serves both vendors' models
// through one provider. An unrecognized vendor returns nil, which Lookup
// reports as unknown so the request is left alone.
func TableFor(providerName, model string) Table {
	id := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.EqualFold(providerName, "grok"),
		strings.HasPrefix(id, "x-ai/"),
		strings.HasPrefix(id, "grok-"):
		return sortedGrok
	case strings.EqualFold(providerName, "meta"),
		strings.HasPrefix(id, "muse-"):
		return sortedMuse
	case strings.EqualFold(providerName, "openai"),
		strings.HasPrefix(id, "openai/"),
		strings.HasPrefix(id, "gpt-"),
		strings.HasPrefix(id, "o1"),
		strings.HasPrefix(id, "o3"),
		strings.HasPrefix(id, "o4"),
		strings.HasPrefix(id, "codex"):
		return sortedOpenAI
	default:
		return nil
	}
}

// Lookup returns the capabilities for a model. The bool reports whether the
// model is known and verified; anything else — a fine-tune, a gateway
// deployment, an unverified entry — is reported as unknown, and the caller must
// forward its parameters untouched, since Dive cannot tell what it accepts.
func Lookup(providerName, model string) (Capabilities, bool) {
	caps, known := lookup(providerName, model)
	if !known {
		return Capabilities{}, false
	}
	// Hand back an independent slice: the tables are package-level and a caller
	// indexing or appending into Efforts would corrupt every later lookup.
	return Capabilities{Efforts: cloneEfforts(caps.Efforts), Temperature: caps.Temperature}, true
}

// lookup is the internal, allocation-free form used on the request path.
func lookup(providerName, model string) (Capabilities, bool) {
	entry, found := LookupEntry(providerName, model)
	if !found || entry.Unverified {
		return Capabilities{}, false
	}
	return entry.Caps, true
}

func cloneEfforts(efforts []llm.ReasoningEffort) []llm.ReasoningEffort {
	if len(efforts) == 0 {
		return nil
	}
	out := make([]llm.ReasoningEffort, len(efforts))
	copy(out, efforts)
	return out
}

// LookupEntry returns the raw entry, including unverified ones. It backs the
// coverage tests, which require every catalogued model to be classified — even
// when the classification is "could not verify".
func LookupEntry(providerName, model string) (Entry, bool) {
	id := NormalizeModelID(model)
	for _, entry := range TableFor(providerName, model) {
		if matchesPrefix(id, entry.Prefix) {
			return entry, true
		}
	}
	return Entry{}, false
}

// ResolveEffort maps a requested effort onto what the model accepts. The bool
// reports whether an effort should be sent at all: a model with no reasoning
// parameter yields false, and the caller omits the field entirely.
//
// Unsupported settings are clamped or dropped, never turned into an error, so
// that one set of options survives being pointed at a different model.
func ResolveEffort(
	providerName, model string,
	effort llm.ReasoningEffort,
	logger llm.Logger,
) (llm.ReasoningEffort, bool) {
	if effort == "" {
		return "", false
	}
	caps, known := lookup(providerName, model)
	if !known {
		return effort, true // unknown model: forward untouched
	}
	if len(caps.Efforts) == 0 {
		warn(logger, "model does not accept a reasoning effort; omitting it",
			"model", model, "effort", effort)
		return "", false
	}
	clamped, changed := llm.ClampReasoningEffort(effort, caps.Efforts)
	if changed {
		warn(logger, "model does not support the requested reasoning effort; clamping",
			"model", model, "requested", effort, "using", clamped)
	}
	return clamped, true
}

// AcceptsTemperature reports whether the model takes a temperature. Unknown
// models are assumed to accept it, as they did before these tables existed.
func AcceptsTemperature(providerName, model string) bool {
	caps, known := lookup(providerName, model)
	return !known || caps.Temperature
}

func warn(logger llm.Logger, msg string, args ...any) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}
