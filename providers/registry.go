package providers

import (
	"os"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// ProviderFactory creates an LLM provider for a given model name and optional endpoint.
type ProviderFactory func(model, endpoint string) llm.LLM

// ModelMatcher determines if a model name matches a provider.
type ModelMatcher func(model string) bool

// ProviderEntry pairs a matcher with its factory.
type ProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory ProviderFactory
}

// Registry manages model-to-provider mappings.
// Providers register themselves during init() and the registry
// is used to create providers based on model names.
type Registry struct {
	mu       sync.RWMutex
	entries  []ProviderEntry
	fallback ProviderFactory
}

// Register adds a provider entry to the registry.
// Entries are checked in registration order, so register more specific
// matchers before more general ones.
func (r *Registry) Register(entry ProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
}

// SetFallback sets the fallback provider factory used when no matcher matches.
func (r *Registry) SetFallback(factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = factory
}

// CreateModel returns an LLM provider for the given model name and endpoint.
// It iterates through registered entries in order and returns the first match.
// If no entry matches and a fallback is set, the fallback is used.
// Returns nil if no match and no fallback.
func (r *Registry) CreateModel(model, endpoint string) llm.LLM {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, entry := range r.entries {
		if entry.Match(model) {
			return entry.Factory(model, endpoint)
		}
	}
	if r.fallback != nil {
		return r.fallback(model, endpoint)
	}
	return nil
}

// Entries returns a copy of all registered provider entries.
func (r *Registry) Entries() []ProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ProviderEntry, len(r.entries))
	copy(result, r.entries)
	return result
}

// Matcher helpers

// PrefixMatcher returns a matcher that checks for a case-insensitive prefix.
func PrefixMatcher(prefix string) ModelMatcher {
	prefix = strings.ToLower(prefix)
	return func(model string) bool {
		return strings.HasPrefix(strings.ToLower(model), prefix)
	}
}

// PrefixesMatcher returns a matcher that checks for any of the given prefixes (case-insensitive).
func PrefixesMatcher(prefixes ...string) ModelMatcher {
	lowered := make([]string, len(prefixes))
	for i, p := range prefixes {
		lowered[i] = strings.ToLower(p)
	}
	return func(model string) bool {
		lower := strings.ToLower(model)
		for _, prefix := range lowered {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
		return false
	}
}

// ContainsMatcher returns a matcher that checks if the model contains a substring.
func ContainsMatcher(substr string) ModelMatcher {
	return func(model string) bool {
		return strings.Contains(model, substr)
	}
}

// EnvMatcher returns a matcher that only matches if an environment variable is set.
// This is useful for providers that require API keys.
func EnvMatcher(envVar string, inner ModelMatcher) ModelMatcher {
	return func(model string) bool {
		if os.Getenv(envVar) == "" {
			return false
		}
		return inner(model)
	}
}

// Global default registry
var defaultRegistry = &Registry{}

// Register adds a provider entry to the default registry.
// This is typically called from provider init() functions.
func Register(entry ProviderEntry) {
	defaultRegistry.Register(entry)
}

// SetFallback sets the fallback provider on the default registry.
func SetFallback(factory ProviderFactory) {
	defaultRegistry.SetFallback(factory)
}

// CreateModel creates an LLM provider using the default registry.
func CreateModel(model, endpoint string) llm.LLM {
	return defaultRegistry.CreateModel(model, endpoint)
}

// DefaultRegistry returns the default global registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}
