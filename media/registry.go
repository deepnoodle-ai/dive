package media

import (
	"strings"
	"sync"
)

// ImageProviderFactory creates an ImageProvider for a given model name.
type ImageProviderFactory func(model string) ImageProvider

// VideoProviderFactory creates a VideoProvider for a given model name.
type VideoProviderFactory func(model string) VideoProvider

// TextToSpeechProviderFactory creates a TextToSpeechProvider for a given model name.
type TextToSpeechProviderFactory func(model string) TextToSpeechProvider

// TranscriptionProviderFactory creates a TranscriptionProvider for a given model name.
type TranscriptionProviderFactory func(model string) TranscriptionProvider

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

// TextToSpeechProviderEntry pairs a matcher with its factory.
type TextToSpeechProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory TextToSpeechProviderFactory
}

// TranscriptionProviderEntry pairs a matcher with its factory.
type TranscriptionProviderEntry struct {
	Name    string
	Match   ModelMatcher
	Factory TranscriptionProviderFactory
}

// Registry manages model-to-provider mappings for media generation.
type Registry struct {
	mu                     sync.RWMutex
	imageProviders         []ImageProviderEntry
	videoProviders         []VideoProviderEntry
	textToSpeechProviders  []TextToSpeechProviderEntry
	transcriptionProviders []TranscriptionProviderEntry
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

// RegisterTextToSpeech adds a text-to-speech provider entry to the registry.
func (r *Registry) RegisterTextToSpeech(entry TextToSpeechProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.textToSpeechProviders = append(r.textToSpeechProviders, entry)
}

// RegisterTranscription adds a transcription provider entry to the registry.
func (r *Registry) RegisterTranscription(entry TranscriptionProviderEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transcriptionProviders = append(r.transcriptionProviders, entry)
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

// ResolveTextToSpeech returns a TextToSpeechProvider for the given model name.
func (r *Registry) ResolveTextToSpeech(model string) (TextToSpeechProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.textToSpeechProviders {
		if entry.Match(model) {
			return entry.Factory(model), nil
		}
	}
	return nil, ErrProviderNotFound
}

// ResolveTranscription returns a TranscriptionProvider for the given model name.
func (r *Registry) ResolveTranscription(model string) (TranscriptionProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.transcriptionProviders {
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

// TextToSpeechEntries returns a copy of all registered text-to-speech provider entries.
func (r *Registry) TextToSpeechEntries() []TextToSpeechProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]TextToSpeechProviderEntry, len(r.textToSpeechProviders))
	copy(result, r.textToSpeechProviders)
	return result
}

// TranscriptionEntries returns a copy of all registered transcription provider entries.
func (r *Registry) TranscriptionEntries() []TranscriptionProviderEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]TranscriptionProviderEntry, len(r.transcriptionProviders))
	copy(result, r.transcriptionProviders)
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

// RegisterTextToSpeech adds a text-to-speech provider entry to the default registry.
func RegisterTextToSpeech(entry TextToSpeechProviderEntry) {
	defaultRegistry.RegisterTextToSpeech(entry)
}

// RegisterTranscription adds a transcription provider entry to the default registry.
func RegisterTranscription(entry TranscriptionProviderEntry) {
	defaultRegistry.RegisterTranscription(entry)
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
