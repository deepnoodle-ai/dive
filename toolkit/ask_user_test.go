package toolkit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestAskUserTool_Name(t *testing.T) {
	tool := NewAskUserTool()
	assert.Equal(t, "AskUserQuestion", tool.Name())
}

func TestAskUserTool_Schema(t *testing.T) {
	tool := NewAskUserTool()
	s := tool.Schema()
	assert.Equal(t, "object", string(s.Type))
	assert.Contains(t, s.Required, "question")
	assert.Contains(t, s.Required, "type")
	assert.Contains(t, s.Properties, "options")
}

func TestAskUserTool_Annotations(t *testing.T) {
	tool := NewAskUserTool()
	a := tool.Annotations()
	assert.Equal(t, "AskUserQuestion", a.Title)
	assert.True(t, a.ReadOnlyHint)
	assert.False(t, a.DestructiveHint)
	assert.False(t, a.IdempotentHint)
}

func TestAskUserTool_SyncNoDialogReturnsError(t *testing.T) {
	tool := NewAskUserTool()
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Proceed?",
		Type:     "confirm",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Nil(t, result.Suspend)
}

func TestAskUserTool_SyncConfirmWithAutoApprove(t *testing.T) {
	tool := NewAskUserTool(AskUserToolOptions{Dialog: &dive.AutoApproveDialog{}})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Proceed?",
		Type:     "confirm",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Nil(t, result.Suspend)
	var out AskUserOutput
	assert.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &out))
	assert.Equal(t, "yes", out.Response)
	assert.False(t, out.Canceled)
}

func TestAskUserTool_SyncSelectWithAutoApprove(t *testing.T) {
	tool := NewAskUserTool(AskUserToolOptions{Dialog: &dive.AutoApproveDialog{}})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Pick one",
		Type:     "select",
		Options: []AskUserInputOption{
			{Value: "a", Label: "A"},
			{Value: "b", Label: "B", Default: true},
		},
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Nil(t, result.Suspend)
	var out AskUserOutput
	assert.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &out))
	assert.Equal(t, "b", out.Response)
}

func TestAskUserTool_SyncDeniedConfirmCancels(t *testing.T) {
	tool := NewAskUserTool(AskUserToolOptions{Dialog: &dive.DenyAllDialog{}})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Delete everything?",
		Type:     "confirm",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Nil(t, result.Suspend)
	var out AskUserOutput
	assert.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &out))
	assert.Equal(t, "no", out.Response)
}

// Async mode pins:

func TestAskUserTool_AsyncReturnsSuspendResult(t *testing.T) {
	tool := NewAskUserTool(AskUserToolOptions{Async: true})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Approve deploy to prod?",
		Type:     "confirm",
	})
	assert.NoError(t, err)
	assert.NotNil(t, result.Suspend)

	// Suspend results must be a clean tagged-union: no Content, no Display,
	// no IsError. The agent's boundary validator rejects mixed states.
	assert.Equal(t, 0, len(result.Content))
	assert.Equal(t, "", result.Display)
	assert.False(t, result.IsError)

	assert.Equal(t, "Approve deploy to prod?", result.Suspend.Prompt)
	assert.Equal(t, "confirm", result.Suspend.Metadata["type"])
}

func TestAskUserTool_AsyncSelectCarriesType(t *testing.T) {
	tool := NewAskUserTool(AskUserToolOptions{Async: true})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Choose region",
		Type:     "select",
		Options: []AskUserInputOption{
			{Value: "us-east", Label: "US East"},
			{Value: "eu-west", Label: "EU West"},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, result.Suspend)
	assert.Equal(t, "Choose region", result.Suspend.Prompt)
	assert.Equal(t, "select", result.Suspend.Metadata["type"])
}

func TestAskUserTool_AsyncIgnoresDialog(t *testing.T) {
	// Even with a Dialog configured, Async=true short-circuits to suspend.
	// This is the documented contract: Async wins over Dialog.
	tool := NewAskUserTool(AskUserToolOptions{
		Async:  true,
		Dialog: &dive.DenyAllDialog{}, // would otherwise cancel
	})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Pick one",
		Type:     "select",
		Options: []AskUserInputOption{
			{Value: "a", Label: "A"},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, result.Suspend)
	assert.Equal(t, "Pick one", result.Suspend.Prompt)
}

func TestAskUserTool_AsyncNoDialogIsValid(t *testing.T) {
	// In async mode, Dialog may be left nil. The sync-mode "no dialog
	// configured" error path must NOT trigger.
	tool := NewAskUserTool(AskUserToolOptions{Async: true})
	result, err := tool.Call(context.Background(), &AskUserInput{
		Question: "Anything?",
		Type:     "input",
	})
	assert.NoError(t, err)
	assert.NotNil(t, result.Suspend)
	assert.False(t, result.IsError)
}

func TestAskUserTool_AsyncInputDecodableFromPendingCall(t *testing.T) {
	// The integrator gets PendingToolCall.Input as raw JSON. Verify the
	// AskUserInput round-trips so a downstream caller can decode it via
	// dive.DecodePendingInput[*AskUserInput].
	tool := NewAskUserTool(AskUserToolOptions{Async: true})
	original := &AskUserInput{
		Question: "Pick options",
		Type:     "multiselect",
		Options: []AskUserInputOption{
			{Value: "x", Label: "X"},
			{Value: "y", Label: "Y"},
		},
		MinSelect: 1,
		MaxSelect: 2,
	}
	result, err := tool.Call(context.Background(), original)
	assert.NoError(t, err)
	assert.NotNil(t, result.Suspend)

	// Round-trip the input through JSON the way the agent would surface
	// it on PendingToolCall.Input.
	raw, err := json.Marshal(original)
	assert.NoError(t, err)

	pending := &dive.PendingToolCall{
		ID:    "call_test",
		Name:  "AskUserQuestion",
		Input: raw,
	}
	decoded, err := dive.DecodePendingInput[*AskUserInput](pending)
	assert.NoError(t, err)
	assert.Equal(t, original.Question, decoded.Question)
	assert.Equal(t, original.Type, decoded.Type)
	assert.Equal(t, 2, len(decoded.Options))
	assert.Equal(t, 1, decoded.MinSelect)
	assert.Equal(t, 2, decoded.MaxSelect)
}
