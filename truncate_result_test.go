package dive_test

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestTruncateResult(t *testing.T) {
	t.Run("short string is unchanged", func(t *testing.T) {
		result, path, err := dive.TruncateResult("hello", 100)
		assert.NoError(t, err)
		assert.Equal(t, "hello", result)
		assert.Equal(t, "", path)
	})

	t.Run("exact limit is unchanged", func(t *testing.T) {
		s := strings.Repeat("a", 100)
		result, path, err := dive.TruncateResult(s, 100)
		assert.NoError(t, err)
		assert.Equal(t, s, result)
		assert.Equal(t, "", path)
	})

	t.Run("over limit is truncated with footer", func(t *testing.T) {
		s := strings.Repeat("a", 200)
		result, path, err := dive.TruncateResult(s, 100)
		assert.NoError(t, err)
		assert.NotEqual(t, "", path)
		assert.True(t, strings.HasPrefix(result, strings.Repeat("a", 100)))
		assert.True(t, strings.Contains(result, "truncated at 100 chars"))
		assert.True(t, strings.Contains(result, path))
	})

	t.Run("unicode is truncated by rune count", func(t *testing.T) {
		s := strings.Repeat("😀", 10)
		result, path, err := dive.TruncateResult(s, 5)
		assert.NoError(t, err)
		assert.NotEqual(t, "", path)
		assert.True(t, strings.HasPrefix(result, strings.Repeat("😀", 5)))
		assert.False(t, strings.HasPrefix(result, strings.Repeat("😀", 6)))
	})
}

func TestToolAnnotationsMaxResultSizeHint(t *testing.T) {
	ann := &dive.ToolAnnotations{MaxResultSizeHint: 50_000}
	assert.Equal(t, 50_000, ann.MaxResultSizeHint)

	// Round-trip through JSON
	data, err := ann.MarshalJSON()
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "maxResultSizeHint"))

	var ann2 dive.ToolAnnotations
	err = ann2.UnmarshalJSON(data)
	assert.NoError(t, err)
	assert.Equal(t, 50_000, ann2.MaxResultSizeHint)
}

func TestToolAnnotationsZeroHintOmitted(t *testing.T) {
	ann := &dive.ToolAnnotations{Title: "test"}
	data, err := ann.MarshalJSON()
	assert.NoError(t, err)
	assert.False(t, strings.Contains(string(data), "maxResultSizeHint"))
}
