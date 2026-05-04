package permission

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// GrantScope controls how long granted permissions last.
type GrantScope int

const (
	// GrantScopeTurn grants access for the current turn only. The consumer
	// must call Manager.ClearTurnAllowlist() (e.g. in a PreGenerationHook)
	// to reset grants at the start of the next turn. This is the default.
	GrantScopeTurn GrantScope = iota

	// GrantScopeSession grants access for the lifetime of the session.
	// Use when re-prompting on every turn would be disruptive.
	GrantScopeSession
)

// PermissionRequest is the request the model sends when asking for elevated access.
type PermissionRequest struct {
	// Tools lists the tool names the model is requesting access to.
	Tools []string

	// Reason is the model-authored explanation of why access is needed.
	Reason string
}

// PermissionRequestHandler is called when the model invokes request_permissions.
// It returns true if access is granted and an error to surface to the model.
type PermissionRequestHandler func(ctx context.Context, req PermissionRequest) (granted bool, err error)

type requestPermissionsInput struct {
	Tools  []string `json:"tools" jsonschema:"description=Tool names the model is requesting access to,required"`
	Reason string   `json:"reason" jsonschema:"description=Explanation of why access is needed,required"`
}

// RequestPermissionsTool returns a Tool named "request_permissions" that the
// model can call to ask the host for elevated access before performing
// sensitive operations.
//
// When granted, the requested tools are added to the Manager's allowlist using
// the provided scope. With GrantScopeTurn (default), grants apply for the
// current turn only; call Manager.ClearTurnAllowlist() in a PreGenerationHook
// to reset at the start of each new turn.
func RequestPermissionsTool(manager *Manager, handler PermissionRequestHandler, scope ...GrantScope) dive.Tool {
	s := GrantScopeTurn
	if len(scope) > 0 {
		s = scope[0]
	}
	return dive.FuncTool("request_permissions",
		"Request permission to use one or more tools before performing a sensitive operation. "+
			"Provide the tool names you need and explain why the access is required.",
		func(ctx context.Context, input *requestPermissionsInput) (*dive.ToolResult, error) {
			req := PermissionRequest{
				Tools:  input.Tools,
				Reason: input.Reason,
			}
			granted, err := handler(ctx, req)
			if err != nil {
				return dive.NewToolResultError(fmt.Sprintf("Permission request failed: %v", err)), nil
			}
			if !granted {
				return dive.NewToolResultText(
					fmt.Sprintf("Permission denied for: %s", strings.Join(input.Tools, ", ")),
				), nil
			}
			// Grant access for each requested tool.
			for _, toolName := range input.Tools {
				category := GetToolCategory(toolName)
				switch s {
				case GrantScopeSession:
					manager.AllowForSession(category.Key)
				default:
					manager.AllowForTurn(category.Key)
				}
			}
			return dive.NewToolResultText(
				fmt.Sprintf("Permission granted for: %s", strings.Join(input.Tools, ", ")),
			), nil
		})
}
