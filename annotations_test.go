package dive

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestDisableParallelUse(t *testing.T) {
	t.Run("marshal includes DisableParallelUse", func(t *testing.T) {
		a := &ToolAnnotations{
			Title:              "Test Tool",
			DisableParallelUse: true,
			ReadOnlyHint:       true,
		}

		data, err := json.Marshal(a)
		assert.NoError(t, err)

		var m map[string]any
		err = json.Unmarshal(data, &m)
		assert.NoError(t, err)

		assert.Equal(t, m["disableParallelUse"], true)
		assert.Equal(t, m["readOnlyHint"], true)
		assert.Equal(t, m["title"], "Test Tool")
	})

	t.Run("unmarshal reads DisableParallelUse", func(t *testing.T) {
		data := `{"title":"Test","disableParallelUse":true,"readOnlyHint":false}`
		var a ToolAnnotations
		err := json.Unmarshal([]byte(data), &a)
		assert.NoError(t, err)

		assert.True(t, a.DisableParallelUse)
		assert.Equal(t, a.Title, "Test")
		assert.False(t, a.ReadOnlyHint)
	})

	t.Run("false by default", func(t *testing.T) {
		data := `{"title":"Test"}`
		var a ToolAnnotations
		err := json.Unmarshal([]byte(data), &a)
		assert.NoError(t, err)

		assert.False(t, a.DisableParallelUse)
	})

	t.Run("extra fields preserved", func(t *testing.T) {
		data := `{"title":"Test","disableParallelUse":true,"customField":"value"}`
		var a ToolAnnotations
		err := json.Unmarshal([]byte(data), &a)
		assert.NoError(t, err)

		assert.True(t, a.DisableParallelUse)
		assert.Equal(t, a.Extra["customField"], "value")
	})
}
