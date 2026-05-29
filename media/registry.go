package media

import (
	"strings"
	"sync"
)

// ImageProviderFactory creates an ImageProvider for a given model name.
type ImageProviderFactory func(model string) ImageProvider

// VideoProviderFactory creates a VideoProvider for a given model name.
type VideoProviderFactory func(model string) VideoProvider

// SpeechProviderFactory creates a SpeechProvider for a given model name.
type SpeechProviderFactory func(model string) SpeechProvider

// SpeechRecognitionProviderFactory creates a SpeechRecognitionProvider for a given model name.
type SpeechRecognitionProviderFactory func(model string) SpeechRecognitionProvider

// ModelMatcher determines if a model name matches a provider.
type ModelMatcher func(model string) bool

// ImageProviderEntry pairs a matcher with its factory.
type ImageProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory ImageProviderFactory
}

// VideoProviderEntry pairs a matcher with its factory.
type VideoProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory VideoProviderFactory
}

// SpeechProviderEntry pairs a matcher with its factory.
type SpeechProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory SpeechProviderFactory
}

// SpeechRecognitionProviderEntry pairs a matcher with its factory.
type SpeechRecognitionProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory SpeechRecognitionProviderFactory
}

// Registry manages model-to-provider mappings for media generation.
type Registry struct {
	mu                         sync.RWMutex
	imageProviders             []ImageProviderEntry
	videoProviders             []VideoProviderEntry
	speechProviders            []SpeechProviderEntry
	speechRecognitionProviders []SpeechRecognitionProviderEntry
}

// RegisterImage adds an image provider entry to the registry.
func (r *Registry) RegisterImage(entry ImageProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.imageProviders = append(r.imageProviders, entry)
}

// RegisterVideo adds a video provider entry to the registry.
func (r *Registry) RegisterVideo(entry VideoProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.videoProviders = append(r.videoProviders, entry)
}

// RegisterSpeech adds a speech provider entry to the registry.
func (r *Registry) RegisterSpeech(entry SpeechProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.speechProviders = append(r.speechProviders, entry)
}

// RegisterSpeechRecognition adds a speech recognition provider entry to the registry.
func (r *Registry) RegisterSpeechRecognition(entry SpeechRecognitionProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.speechRecognitionProviders = append(r.speechRecognitionProviders, entry)
}

// ResolveImage returns an ImageProvider for the given model name.
func (r *Registry) ResolveImage(model string) (ImageProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.imageProviders {
		if entry.Match(model) {
			return entry.Factory(model), nil
		}
	}
	return nil, ErrProviderNotFound
}

// ResolveVideo returns a VideoProvider for the given model name.
func (r *Registry) ResolveVideo(model string) (VideoProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.videoProviders {
		if entry.Match(model) {
			return entry.Factory(model), nil
		}
	}
	return nil, ErrProviderNotFound
}

// ResolveSpeech returns a SpeechProvider for the given model name.
func (r *Registry) ResolveSpeech(model string) (SpeechProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.speechProviders {
		if entry.Match(model) {
			return entry.Factory(model), nil
		}
	}
	return nil, ErrProviderNotFound
}

// ResolveSpeechRecognition returns a SpeechRecognitionProvider for the given model name.
func (r *Registry) ResolveSpeechRecognition(model string) (SpeechRecognitionProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.speechRecognitionProviders {
		if entry.Match(model) {
			return entry.Factory(model), nil
		}
	}
	return nil, ErrProviderNotFound
}

// ImageEntries returns a copy of all registered image provider entries.
func (r *Registry) ImageEntries() []ImageProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ImageProviderEntry, len(r.imageProviders))
	copy(result, r.imageProviders)
	return result
}

// VideoEntries returns a copy of all registered video provider entries.
func (r *Registry) VideoEntries() []VideoProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]VideoProviderEntry, len(r.videoProviders))
	copy(result, r.videoProviders)
	return result
}

// SpeechEntries returns a copy of all registered speech provider entries.
func (r *Registry) SpeechEntries() []SpeechProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]SpeechProviderEntry, len(r.speechProviders))
	copy(result, r.speechProviders)
	return result
}

// SpeechRecognitionEntries returns a copy of all registered speech recognition provider entries.
func (r *Registry) SpeechRecognitionEntries() []SpeechRecognitionProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]SpeechRecognitionProviderEntry, len(r.speechRecognitionProviders))
	copy(result, r.speechRecognitionProviders)
	return result
}

// Global default registry

var defaultRegistry = &Registry{}

// RegisterImage adds an image provider entry to the default registry.
func RegisterImage(entry ImageProviderEntry) {
	defaultRegistry.RegisterImage(entry)
}

// RegisterVideo adds a video provider entry to the default registry.
func RegisterVideo(entry VideoProviderEntry) {
	defaultRegistry.RegisterVideo(entry)
}

// RegisterSpeech adds a speech provider entry to the default registry.
func RegisterSpeech(entry SpeechProviderEntry) {
	defaultRegistry.RegisterSpeech(entry)
}

// RegisterSpeechRecognition adds a speech recognition provider entry to the default registry.
func RegisterSpeechRecognition(entry SpeechRecognitionProviderEntry) {
	defaultRegistry.RegisterSpeechRecognition(entry)
}

// DefaultRegistry returns the default global registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Matcher helpers

// PrefixMatcher returns a matcher that checks for a case-insensitive prefix.
func PrefixMatcher(prefix string) ModelMatcher {
	prefix = strings.ToLower(prefix)
	return func(model string) bool {
		return strings.HasPrefix(strings.ToLower(model), prefix)
	}
}

// PrefixesMatcher returns a matcher that checks for any of the given
// prefixes (case-insensitive).
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
