package permission_test

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/permission"
	"github.com/deepnoodle-ai/wonton/assert"
)

// callTool invokes a dive.Tool with raw JSON input bytes.
func callTool(ctx context.Context, tool dive.Tool, rawJSON string) (*dive.ToolResult, error) {
	return tool.Call(ctx, []byte(rawJSON))
}

func resultText(r *dive.ToolResult) string {
	for _, c := range r.Content {
		if c.Text != "" {
			return c.Text
		}
	}
	return ""
}

func contains(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func TestRequestPermissionsToolTurnScope(t *testing.T) {
	ctx := context.Background()

	t.Run("grants turn-scoped access when approved", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, req permission.PermissionRequest) (bool, error) {
			return true, nil
		})

		result, err := callTool(ctx, tool, `{"tools":["Bash","Edit"],"reason":"need to run tests"}`)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, contains(resultText(result), "granted"))

		assert.True(t, manager.IsTurnAllowed("bash"))
		assert.True(t, manager.IsTurnAllowed("edit"))
		assert.False(t, manager.IsSessionAllowed("bash"))
	})

	t.Run("denied when handler returns false", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
			return false, nil
		})

		result, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"risky operation"}`)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.True(t, contains(resultText(result), "denied"))

		assert.False(t, manager.IsTurnAllowed("bash"))
		assert.False(t, manager.IsSessionAllowed("bash"))
	})

	t.Run("ClearTurnAllowlist revokes turn grants", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
			return true, nil
		})

		_, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"run tests"}`)
		assert.NoError(t, err)
		assert.True(t, manager.IsTurnAllowed("bash"))

		manager.ClearTurnAllowlist()
		assert.False(t, manager.IsTurnAllowed("bash"))
	})
}

func TestRequestPermissionsToolSessionScope(t *testing.T) {
	ctx := context.Background()

	t.Run("grants session-scoped access when approved", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager,
			func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
				return true, nil
			},
			permission.GrantScopeSession,
		)

		result, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"need bash"}`)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		assert.True(t, manager.IsSessionAllowed("bash"))
		assert.False(t, manager.IsTurnAllowed("bash"))
	})
}

func TestRequestPermissionsToolHandlerError(t *testing.T) {
	ctx := context.Background()

	t.Run("handler error surfaces as error result", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
			return false, context.DeadlineExceeded
		})

		result, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"test"}`)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.True(t, contains(resultText(result), "failed"))
	})
}

func TestRequestPermissionsToolReceivesRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("handler receives correct request fields", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		var capturedReq permission.PermissionRequest
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, req permission.PermissionRequest) (bool, error) {
			capturedReq = req
			return false, nil
		})

		_, err := callTool(ctx, tool, `{"tools":["Bash","Edit"],"reason":"I need to modify config files"}`)
		assert.NoError(t, err)

		assert.Equal(t, 2, len(capturedReq.Tools))
		assert.Equal(t, "Bash", capturedReq.Tools[0])
		assert.Equal(t, "Edit", capturedReq.Tools[1])
		assert.Equal(t, "I need to modify config files", capturedReq.Reason)
	})
}

func TestTurnAllowlistIntegrationWithEvaluate(t *testing.T) {
	ctx := context.Background()

	t.Run("turn-granted tool passes EvaluateToolUse", func(t *testing.T) {
		manager := permission.NewManager(&permission.Config{Mode: permission.ModeDontAsk}, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
			return true, nil
		})

		// Grant Bash via the request_permissions tool
		_, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"need bash"}`)
		assert.NoError(t, err)
		assert.True(t, manager.IsTurnAllowed("bash"))

		// A Bash tool call should now be allowed
		bashTool := &stubTool{name: "Bash"}
		err = manager.EvaluateToolUse(ctx, bashTool, nil)
		assert.NoError(t, err)
	})

	t.Run("turn grants are not session grants", func(t *testing.T) {
		manager := permission.NewManager(nil, nil)
		tool := permission.RequestPermissionsTool(manager, func(_ context.Context, _ permission.PermissionRequest) (bool, error) {
			return true, nil
		})

		_, err := callTool(ctx, tool, `{"tools":["Bash"],"reason":"temp access"}`)
		assert.NoError(t, err)

		assert.True(t, manager.IsTurnAllowed("bash"))
		assert.False(t, manager.IsSessionAllowed("bash"))
	})
}

// stubTool is a minimal dive.Tool for testing.
type stubTool struct {
	name string
}

func (s *stubTool) Name() string                                            { return s.name }
func (s *stubTool) Description() string                                      { return "" }
func (s *stubTool) Schema() *dive.Schema                                     { return nil }
func (s *stubTool) Annotations() *dive.ToolAnnotations                      { return nil }
func (s *stubTool) Call(_ context.Context, _ any) (*dive.ToolResult, error) { return nil, nil }
