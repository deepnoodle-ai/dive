package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/settings"
)

// HookEvent represents the type of hook event
type HookEvent string

const (
	PreToolUse         HookEvent = "PreToolUse"
	PostToolUse        HookEvent = "PostToolUse"
	UserPromptSubmit   HookEvent = "UserPromptSubmit"
	Stop               HookEvent = "Stop"
	SubagentStop       HookEvent = "SubagentStop"
	PreCompact         HookEvent = "PreCompact"
	SessionStart       HookEvent = "SessionStart"
	SessionEnd         HookEvent = "SessionEnd"
	Notification       HookEvent = "Notification"
)

// HookInput represents the input to a hook
type HookInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path"`
	CWD            string         `json:"cwd"`
	HookEventName  string         `json:"hook_event_name"`
	ToolName       string         `json:"tool_name,omitempty"`
	ToolInput      interface{}    `json:"tool_input,omitempty"`
	ToolResponse   interface{}    `json:"tool_response,omitempty"`
	Prompt         string         `json:"prompt,omitempty"`
	Message        string         `json:"message,omitempty"`
	Trigger        string         `json:"trigger,omitempty"`
	Source         string         `json:"source,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	StopHookActive bool           `json:"stop_hook_active,omitempty"`
	CustomInstructions string     `json:"custom_instructions,omitempty"`
}

// HookOutput represents the output from a hook
type HookOutput struct {
	Continue         bool   `json:"continue"`
	StopReason       string `json:"stopReason,omitempty"`
	SuppressOutput   bool   `json:"suppressOutput"`
	SystemMessage    string `json:"systemMessage,omitempty"`
	Decision         string `json:"decision,omitempty"`
	Reason           string `json:"reason,omitempty"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput contains event-specific output
type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// HookManager manages hook execution
type HookManager struct {
	settings *settings.Settings
	disabled bool
	mu       sync.RWMutex
}

// NewHookManager creates a new hook manager
func NewHookManager(s *settings.Settings) *HookManager {
	return &HookManager{
		settings: s,
		disabled: s != nil && s.DisableAllHooks,
	}
}

// ExecuteHooks executes all matching hooks for an event
func (hm *HookManager) ExecuteHooks(ctx context.Context, event HookEvent, input *HookInput) (*HookOutput, error) {
	if hm.disabled {
		return &HookOutput{Continue: true}, nil
	}

	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if hm.settings == nil || hm.settings.Hooks == nil {
		return &HookOutput{Continue: true}, nil
	}

	hookConfigs, ok := hm.settings.Hooks[string(event)]
	if !ok || len(hookConfigs) == 0 {
		return &HookOutput{Continue: true}, nil
	}

	// Collect all matching hooks
	var matchingHooks []settings.HookAction
	for _, config := range hookConfigs {
		if hm.matchesPattern(config.Matcher, input.ToolName) {
			matchingHooks = append(matchingHooks, config.Hooks...)
		}
	}

	if len(matchingHooks) == 0 {
		return &HookOutput{Continue: true}, nil
	}

	// Deduplicate hooks
	uniqueHooks := hm.deduplicateHooks(matchingHooks)

	// Execute hooks in parallel
	results := make(chan *HookOutput, len(uniqueHooks))
	errors := make(chan error, len(uniqueHooks))

	var wg sync.WaitGroup
	for _, hook := range uniqueHooks {
		wg.Add(1)
		go func(h settings.HookAction) {
			defer wg.Done()
			output, err := hm.executeHook(ctx, h, input)
			if err != nil {
				errors <- err
			} else {
				results <- output
			}
		}(hook)
	}

	// Wait for all hooks to complete
	wg.Wait()
	close(results)
	close(errors)

	// Collect errors
	var hookErrors []error
	for err := range errors {
		hookErrors = append(hookErrors, err)
	}

	// Merge outputs - any hook can stop execution
	finalOutput := &HookOutput{Continue: true}
	for output := range results {
		if !output.Continue {
			finalOutput.Continue = false
			if output.StopReason != "" {
				finalOutput.StopReason = output.StopReason
			}
		}
		if output.SystemMessage != "" {
			if finalOutput.SystemMessage != "" {
				finalOutput.SystemMessage += "\n"
			}
			finalOutput.SystemMessage += output.SystemMessage
		}
		if output.Decision != "" {
			finalOutput.Decision = output.Decision
		}
		if output.Reason != "" {
			finalOutput.Reason = output.Reason
		}
		if output.HookSpecificOutput != nil {
			finalOutput.HookSpecificOutput = output.HookSpecificOutput
		}
	}

	if len(hookErrors) > 0 {
		return finalOutput, fmt.Errorf("hook execution errors: %v", hookErrors)
	}

	return finalOutput, nil
}

// executeHook executes a single hook
func (hm *HookManager) executeHook(ctx context.Context, hook settings.HookAction, input *HookInput) (*HookOutput, error) {
	if hook.Type != "command" {
		return nil, fmt.Errorf("unsupported hook type: %s", hook.Type)
	}

	// Prepare environment
	env := os.Environ()
	projectDir, _ := os.Getwd()
	env = append(env, fmt.Sprintf("DIVE_PROJECT_DIR=%s", projectDir))

	// Create command with timeout
	timeout := time.Duration(hook.Timeout) * time.Second
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse command
	cmdParts := strings.Fields(hook.Command)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("empty hook command")
	}

	// Expand environment variables in command
	expandedCommand := os.ExpandEnv(hook.Command)
	cmdParts = strings.Fields(expandedCommand)

	cmd := exec.CommandContext(cmdCtx, cmdParts[0], cmdParts[1:]...)
	cmd.Env = env

	// Prepare input JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hook input: %w", err)
	}

	// Set up pipes
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err = cmd.Run()

	// Handle exit codes
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}
	}

	// Parse output based on exit code
	switch exitCode {
	case 0:
		// Success - try to parse JSON output
		if stdout.Len() > 0 {
			var output HookOutput
			if err := json.Unmarshal(stdout.Bytes(), &output); err == nil {
				return &output, nil
			}
			// If not JSON, treat as simple success with stdout as context
			return &HookOutput{
				Continue: true,
				HookSpecificOutput: &HookSpecificOutput{
					HookEventName:     string(input.HookEventName),
					AdditionalContext: stdout.String(),
				},
			}, nil
		}
		return &HookOutput{Continue: true}, nil

	case 2:
		// Blocking error
		return &HookOutput{
			Continue:   false,
			StopReason: stderr.String(),
		}, nil

	default:
		// Non-blocking error
		return &HookOutput{
			Continue:      true,
			SystemMessage: fmt.Sprintf("Hook error: %s", stderr.String()),
		}, nil
	}
}

// matchesPattern checks if a tool name matches a pattern
func (hm *HookManager) matchesPattern(pattern, toolName string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}

	// Try regex match
	if regex, err := regexp.Compile(pattern); err == nil {
		return regex.MatchString(toolName)
	}

	// Fall back to exact match
	return pattern == toolName
}

// deduplicateHooks removes duplicate hook commands
func (hm *HookManager) deduplicateHooks(hooks []settings.HookAction) []settings.HookAction {
	seen := make(map[string]bool)
	var unique []settings.HookAction

	for _, hook := range hooks {
		key := fmt.Sprintf("%s:%s:%d", hook.Type, hook.Command, hook.Timeout)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, hook)
		}
	}

	return unique
}

// ValidateHooks validates hook configuration
func ValidateHooks(s *settings.Settings) error {
	if s == nil || s.Hooks == nil {
		return nil
	}

	validEvents := map[string]bool{
		"PreToolUse":       true,
		"PostToolUse":      true,
		"UserPromptSubmit": true,
		"Stop":             true,
		"SubagentStop":     true,
		"PreCompact":       true,
		"SessionStart":     true,
		"SessionEnd":       true,
		"Notification":     true,
	}

	for event, configs := range s.Hooks {
		if !validEvents[event] {
			return fmt.Errorf("invalid hook event: %s", event)
		}

		for _, config := range configs {
			for _, hook := range config.Hooks {
				if hook.Type != "command" {
					return fmt.Errorf("invalid hook type: %s (only 'command' is supported)", hook.Type)
				}
				if hook.Command == "" {
					return fmt.Errorf("empty hook command for event %s", event)
				}
			}
		}
	}

	return nil
}

// CreateTranscriptFile creates a transcript file for the session
func CreateTranscriptFile(sessionID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	transcriptDir := filepath.Join(homeDir, ".dive", "projects", "current")
	if err := os.MkdirAll(transcriptDir, 0755); err != nil {
		return "", err
	}

	transcriptPath := filepath.Join(transcriptDir, fmt.Sprintf("%s.jsonl", sessionID))

	// Create empty transcript file
	file, err := os.Create(transcriptPath)
	if err != nil {
		return "", err
	}
	file.Close()

	return transcriptPath, nil
}

// AppendToTranscript appends a line to the transcript
func AppendToTranscript(transcriptPath string, data interface{}) error {
	file, err := os.OpenFile(transcriptPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = file.Write(append(jsonData, '\n'))
	return err
}

// ReadTranscript reads a transcript file
func ReadTranscript(transcriptPath string) ([]json.RawMessage, error) {
	file, err := os.Open(transcriptPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []json.RawMessage
	decoder := json.NewDecoder(file)

	for {
		var line json.RawMessage
		if err := decoder.Decode(&line); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}

	return lines, nil
}