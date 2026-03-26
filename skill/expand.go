package skill

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ExpandOption configures variable expansion behavior.
type ExpandOption func(*expandConfig)

type expandConfig struct {
	allowShellExpansion bool
	shellTimeout        time.Duration
}

func defaultExpandConfig() *expandConfig {
	return &expandConfig{
		shellTimeout: 10 * time.Second,
	}
}

// WithShellExpansion enables !{command} substitution.
// Disabled by default for security.
func WithShellExpansion(allow bool) ExpandOption {
	return func(c *expandConfig) {
		c.allowShellExpansion = allow
	}
}

// WithShellTimeout sets the timeout for each !{command} execution.
// Default is 10 seconds.
func WithShellTimeout(d time.Duration) ExpandOption {
	return func(c *expandConfig) {
		c.shellTimeout = d
	}
}

// positionalArgPattern matches $1, $2, etc.
var positionalArgPattern = regexp.MustCompile(`\$(\d+)`)

// shellPattern matches !{command} placeholders.
var shellPattern = regexp.MustCompile(`!\{([^}]+)\}`)

// Expand processes a skill's instructions with the given arguments.
//
// Supports:
//   - $ARGUMENTS — full argument string
//   - $1, $2, ..., $9 — positional arguments
//   - !{command} — shell command substitution (requires WithShellExpansion(true))
//
// Returns the expanded instructions text.
func (s *Skill) Expand(ctx context.Context, args string, opts ...ExpandOption) (string, error) {
	cfg := defaultExpandConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	result := s.expandVariables(args)

	if cfg.allowShellExpansion {
		var expandErr error
		result = shellPattern.ReplaceAllStringFunc(result, func(match string) string {
			if expandErr != nil {
				return match
			}
			submatch := shellPattern.FindStringSubmatch(match)
			if len(submatch) < 2 {
				return match
			}
			cmd := submatch[1]
			output, err := runShellCommand(ctx, cmd, cfg.shellTimeout)
			if err != nil {
				expandErr = fmt.Errorf("shell expansion !{%s}: %w", cmd, err)
				return match
			}
			return strings.TrimSpace(output)
		})
		if expandErr != nil {
			return result, expandErr
		}
	}

	return result, nil
}

// ExpandArguments replaces $ARGUMENTS and $1-$9 placeholders only (no shell).
// This is a convenience method for simple use cases.
func (s *Skill) ExpandArguments(args string) string {
	return s.expandVariables(args)
}

// expandVariables handles $ARGUMENTS and positional $N substitution.
func (s *Skill) expandVariables(argsString string) string {
	positionalArgs := strings.Fields(argsString)
	result := s.Instructions

	// Replace positional arguments $1, $2, etc.
	result = positionalArgPattern.ReplaceAllStringFunc(result, func(match string) string {
		var num int
		fmt.Sscanf(match, "$%d", &num)
		if num > 0 && num <= len(positionalArgs) {
			return positionalArgs[num-1]
		}
		return match
	})

	// Replace $ARGUMENTS with full argument string
	result = strings.ReplaceAll(result, "$ARGUMENTS", argsString)

	return result
}

// runShellCommand executes a command with the user's shell and returns stdout.
func runShellCommand(ctx context.Context, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timed out after %s", timeout)
		}
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}
