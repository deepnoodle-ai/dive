package enhanced

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/hooks"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/memory"
	"github.com/deepnoodle-ai/dive/permissions"
	"github.com/deepnoodle-ai/dive/subagents"
)

// Agent wraps a standard agent with Claude Code-inspired features
type Agent struct {
	dive.Agent
	Environment       *Environment
	MemoryManager     *memory.Memory
	HookManager       *hooks.HookManager
	PermissionManager *permissions.PermissionManager
	SubagentManager   *subagents.SubagentManager
	sessionID         string
	transcriptPath    string
}

// CreateResponse creates a response with enhanced features
func (ea *Agent) CreateResponse(ctx context.Context, opts ...dive.CreateResponseOption) (*dive.Response, error) {
	// Parse options
	options := &dive.CreateResponseOptions{}
	options.Apply(opts)

	// Initialize session if needed
	if ea.sessionID == "" {
		ea.sessionID = fmt.Sprintf("enhanced-%s-%d", ea.Name(), randomInt())
		if ea.HookManager != nil {
			ea.transcriptPath, _ = hooks.CreateTranscriptFile(ea.sessionID)
		}
	}

	// Execute SessionStart hook if this is the first message
	if ea.HookManager != nil && len(options.Messages) > 0 {
		cwd := getCurrentDir()
		hookInput := &hooks.HookInput{
			SessionID:      ea.sessionID,
			TranscriptPath: ea.transcriptPath,
			CWD:           cwd,
			HookEventName:  string(hooks.SessionStart),
			Source:         "agent",
		}
		ea.HookManager.ExecuteHooks(ctx, hooks.SessionStart, hookInput)
	}

	// Apply memory context
	if ea.MemoryManager != nil && len(options.Messages) > 0 {
		memoryContext := ea.MemoryManager.GetCombinedMemory()
		if memoryContext != "" {
			// Prepend memory context as a system message
			memoryMsg := llm.NewSystemMessage(fmt.Sprintf("Memory Context:\n%s", memoryContext))
			options.Messages = append([]*llm.Message{memoryMsg}, options.Messages...)
		}
	}

	// Execute UserPromptSubmit hook
	if ea.HookManager != nil && len(options.Messages) > 0 {
		lastMsg := options.Messages[len(options.Messages)-1]
		if lastMsg.Role == llm.User {
			cwd := getCurrentDir()
			hookInput := &hooks.HookInput{
				SessionID:      ea.sessionID,
				TranscriptPath: ea.transcriptPath,
				CWD:           cwd,
				HookEventName:  string(hooks.UserPromptSubmit),
				Prompt:         contentToString(lastMsg.Content),
			}

			output, err := ea.HookManager.ExecuteHooks(ctx, hooks.UserPromptSubmit, hookInput)
			if err != nil {
				return nil, fmt.Errorf("hook execution failed: %w", err)
			}

			if !output.Continue {
				// Hook blocked execution
				return nil, fmt.Errorf("execution blocked by hook: %s", output.StopReason)
			}

			// Add any additional context from hooks
			if output.HookSpecificOutput != nil && output.HookSpecificOutput.AdditionalContext != "" {
				contextMsg := llm.NewSystemMessage(fmt.Sprintf("Hook Context: %s", output.HookSpecificOutput.AdditionalContext))
				options.Messages = append(options.Messages, contextMsg)
			}
		}
	}

	// Check for subagent invocation patterns
	if ea.SubagentManager != nil && len(options.Messages) > 0 {
		lastMsg := options.Messages[len(options.Messages)-1]
		if subagentName := ea.detectSubagentInvocation(contentToString(lastMsg.Content)); subagentName != "" {
			return ea.invokeSubagent(ctx, subagentName, options)
		}
	}

	// Wrap the original event callback to add hook and permission checks
	originalCallback := options.EventCallback
	options.EventCallback = func(ctx context.Context, item *dive.ResponseItem) error {
		// Check for tool use and apply permissions/hooks
		if item.Type == dive.ResponseItemTypeToolCall {
			toolCall := item.ToolCall

			// Parse tool input
			var params map[string]interface{}
			if len(toolCall.Input) > 0 {
				json.Unmarshal(toolCall.Input, &params)
			}

			// Check permissions
			if ea.PermissionManager != nil {
				decision := ea.PermissionManager.CheckToolPermission(toolCall.Name, params)
				switch decision {
				case permissions.Deny:
					return fmt.Errorf("tool use denied by permissions: %s", toolCall.Name)
				case permissions.Ask:
					// In automated mode, we'll allow for now
					// In interactive mode, this would prompt the user
					// For now, we'll always prompt in ask mode
					{
						prompt := &permissions.InteractivePrompt{
							Tool:        toolCall.Name,
							Description: fmt.Sprintf("Tool %s wants to execute", toolCall.Name),
							Params:      params,
						}
						if decision, err := prompt.ShowPrompt(); err != nil || decision != permissions.Allow {
							return fmt.Errorf("tool use not approved: %s", toolCall.Name)
						}
					}
				}
			}

			// Execute PreToolUse hook
			if ea.HookManager != nil {
				cwd := getCurrentDir()
				hookInput := &hooks.HookInput{
					SessionID:      ea.sessionID,
					TranscriptPath: ea.transcriptPath,
					CWD:           cwd,
					HookEventName:  string(hooks.PreToolUse),
					ToolName:       toolCall.Name,
					ToolInput:      params,
				}

				output, err := ea.HookManager.ExecuteHooks(ctx, hooks.PreToolUse, hookInput)
				if err != nil {
					return err
				}
				if !output.Continue {
					return fmt.Errorf("tool use blocked by hook: %s", output.StopReason)
				}
			}
		}

		// Call original callback if present
		if originalCallback != nil {
			return originalCallback(ctx, item)
		}

		// Execute PostToolUse hook for tool results
		if item.Type == dive.ResponseItemTypeToolCallResult && ea.HookManager != nil {
			toolResult := item.ToolCallResult
			cwd := getCurrentDir()
			hookInput := &hooks.HookInput{
				SessionID:      ea.sessionID,
				TranscriptPath: ea.transcriptPath,
				CWD:           cwd,
				HookEventName:  string(hooks.PostToolUse),
				ToolName:       toolResult.Name,
				ToolResponse:   toolResultToString(toolResult.Result),
			}

			ea.HookManager.ExecuteHooks(ctx, hooks.PostToolUse, hookInput)
		}

		return nil
	}

	// Call the underlying agent's CreateResponse
	response, err := ea.Agent.CreateResponse(ctx, opts...)

	// Execute SessionEnd hook if needed
	// (In a real implementation, we'd track when the conversation ends)

	return response, err
}

// detectSubagentInvocation checks if the message invokes a subagent
func (ea *Agent) detectSubagentInvocation(content string) string {
	// Look for patterns like "@code-reviewer" or "/agent:debugger"
	if strings.HasPrefix(content, "@") {
		parts := strings.Fields(content)
		if len(parts) > 0 {
			name := strings.TrimPrefix(parts[0], "@")
			if _, err := ea.SubagentManager.GetSubagent(name); err == nil {
				return name
			}
		}
	}

	if strings.HasPrefix(content, "/agent:") {
		parts := strings.SplitN(content, " ", 2)
		if len(parts) > 0 {
			name := strings.TrimPrefix(parts[0], "/agent:")
			if _, err := ea.SubagentManager.GetSubagent(name); err == nil {
				return name
			}
		}
	}

	// Check for auto-invoke patterns in subagent definitions
	// Note: This would require access to the config, which we don't have here
	// to avoid circular imports. This feature could be implemented differently.

	return ""
}

// invokeSubagent invokes a subagent
func (ea *Agent) invokeSubagent(ctx context.Context, name string, options *dive.CreateResponseOptions) (*dive.Response, error) {
	subagentDef, err := ea.SubagentManager.GetSubagent(name)
	if err != nil {
		return nil, err
	}

	// Execute SubagentStop hook
	if ea.HookManager != nil {
		cwd := getCurrentDir()
		hookInput := &hooks.HookInput{
			SessionID:      ea.sessionID,
			TranscriptPath: ea.transcriptPath,
			CWD:           cwd,
			HookEventName:  string(hooks.SubagentStop),
			Message:        fmt.Sprintf("Invoking subagent: %s", name),
		}

		output, err := ea.HookManager.ExecuteHooks(ctx, hooks.SubagentStop, hookInput)
		if err != nil {
			return nil, err
		}
		if !output.Continue {
			return nil, fmt.Errorf("subagent invocation blocked: %s", output.StopReason)
		}
	}

	// Create subagent from definition
	subagent, err := ea.SubagentManager.CreateAgentFromSubagent(subagentDef, ea.Agent)
	if err != nil {
		return nil, fmt.Errorf("failed to create subagent: %w", err)
	}

	// Wrap subagent with enhanced features
	enhancedSubagent := &Agent{
		Agent:             subagent,
		Environment:       ea.Environment,
		MemoryManager:     ea.MemoryManager,
		HookManager:       ea.HookManager,
		PermissionManager: ea.PermissionManager,
		SubagentManager:   ea.SubagentManager,
		sessionID:         ea.sessionID + "-" + name,
		transcriptPath:    ea.transcriptPath,
	}

	// Modify the prompt to indicate subagent context
	if len(options.Messages) > 0 {
		contextMsg := llm.NewSystemMessage(fmt.Sprintf(
			"You are now operating as the '%s' subagent. %s\n\n%s",
			name,
			subagentDef.Description,
			subagentDef.SystemPrompt,
		))
		options.Messages = append([]*llm.Message{contextMsg}, options.Messages...)
	}

	// Invoke the subagent
	return enhancedSubagent.CreateResponse(ctx, dive.WithMessages(options.Messages...))
}

// Helper functions
func getCurrentDir() string {
	dir, _ := os.Getwd()
	return dir
}

func randomInt() int64 {
	// Simple random number for session IDs
	return time.Now().UnixNano()
}

// contentToString converts message content to a string
func contentToString(content []llm.Content) string {
	var result strings.Builder
	for _, c := range content {
		if c.Type() == llm.ContentTypeText {
			if textContent, ok := c.(*llm.TextContent); ok {
				result.WriteString(textContent.Text)
			}
		}
	}
	return result.String()
}

// toolResultToString converts a ToolResult to a string
func toolResultToString(tr *dive.ToolResult) string {
	if tr == nil {
		return ""
	}
	var result strings.Builder
	for _, content := range tr.Content {
		if content.Type == dive.ToolResultContentTypeText {
			result.WriteString(content.Text)
		}
	}
	return result.String()
}