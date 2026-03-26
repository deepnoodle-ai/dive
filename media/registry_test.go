package media

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

type mockImageProvider struct {
	model  string
	result []*ImageResult
	err    error
}

func (m *mockImageProvider) GenerateImage(_ context.Context, _ string, _ *Config) ([]*ImageResult, error) {
	return m.result, m.err
}

type mockImageEditor struct {
	mockImageProvider
	editResult []*ImageResult
	editErr    error
}

func (m *mockImageEditor) EditImage(_ context.Context, _ string, _ *Config) ([]*ImageResult, error) {
	return m.editResult, m.editErr
}

type mockVideoProvider struct {
	model  string
	result *VideoResult
	err    error
}

func (m *mockVideoProvider) GenerateVideo(_ context.Context, _ string, _ *Config) (*VideoResult, error) {
	return m.result, m.err
}

func TestRegistry_ResolveImage(t *testing.T) {
	r := &Registry{}
	called := false
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			called = true
			return &mockImageProvider{model: model}
		},
	})

	provider, err := r.ResolveImage("test-model")
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.True(t, called)
}

func TestRegistry_ResolveImage_NotFound(t *testing.T) {
	r := &Registry{}
	_, err := r.ResolveImage("unknown-model")
	assert.Equal(t, ErrProviderNotFound, err)
}

func TestRegistry_ResolveVideo(t *testing.T) {
	r := &Registry{}
	r.RegisterVideo(VideoProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("veo-"),
		Factory: func(model string) VideoProvider {
			return &mockVideoProvider{model: model}
		},
	})

	provider, err := r.ResolveVideo("veo-3")
	assert.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestRegistry_ResolveVideo_NotFound(t *testing.T) {
	r := &Registry{}
	_, err := r.ResolveVideo("unknown")
	assert.Equal(t, ErrProviderNotFound, err)
}

func TestRegistry_FirstMatchWins(t *testing.T) {
	r := &Registry{}
	r.RegisterImage(ImageProviderEntry{
		Name:  "first",
		Match: PrefixMatcher("model-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{model: "first"}
		},
	})
	r.RegisterImage(ImageProviderEntry{
		Name:  "second",
		Match: PrefixMatcher("model-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{model: "second"}
		},
	})

	provider, err := r.ResolveImage("model-x")
	assert.NoError(t, err)
	mock := provider.(*mockImageProvider)
	assert.Equal(t, "first", mock.model)
}

func TestPrefixesMatcher(t *testing.T) {
	matcher := PrefixesMatcher("gpt-image-", "dall-e-")
	assert.True(t, matcher("gpt-image-1"))
	assert.True(t, matcher("GPT-IMAGE-1"))
	assert.True(t, matcher("dall-e-3"))
	assert.True(t, !matcher("stable-diffusion"))
}

func TestRegistry_Entries(t *testing.T) {
	r := &Registry{}
	r.RegisterImage(ImageProviderEntry{Name: "a", Match: PrefixMatcher("a-"), Factory: func(string) ImageProvider { return nil }})
	r.RegisterImage(ImageProviderEntry{Name: "b", Match: PrefixMatcher("b-"), Factory: func(string) ImageProvider { return nil }})
	r.RegisterVideo(VideoProviderEntry{Name: "v", Match: PrefixMatcher("v-"), Factory: func(string) VideoProvider { return nil }})

	assert.Equal(t, 2, len(r.ImageEntries()))
	assert.Equal(t, 1, len(r.VideoEntries()))
}
