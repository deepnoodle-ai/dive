package skill

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
// Security: shell expansion runs against the raw template only, before any
// argument substitution. Arguments are model-controlled, so a !{...} sequence
// carried in args is never executed — it appears as literal text in the
// output. Shell command output is likewise inserted verbatim and never
// re-scanned for !{...} or $N placeholders.
//
// Template authors can still reference arguments inside a !{command} block:
// $1-$9 are passed to the shell as positional parameters and the full
// argument string is exported as $ARGUMENTS in the environment, so the shell
// receives argument values as data rather than as spliced command text.
//
// Returns the expanded instructions text.
func (s *Skill) Expand(ctx context.Context, args string, opts ...ExpandOption) (string, error) {
	cfg := defaultExpandConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if !cfg.allowShellExpansion {
		return s.expandVariables(args), nil
	}

	positionalArgs := strings.Fields(args)
	var sb strings.Builder
	var expandErr error
	last := 0
	for _, loc := range shellPattern.FindAllStringSubmatchIndex(s.Instructions, -1) {
		// Literal template text before the !{...} block: substitute args.
		sb.WriteString(substituteArgs(s.Instructions[last:loc[0]], args, positionalArgs))
		match := s.Instructions[loc[0]:loc[1]]
		command := s.Instructions[loc[2]:loc[3]]
		last = loc[1]
		if expandErr != nil {
			sb.WriteString(match)
			continue
		}
		output, err := runShellCommand(ctx, command, args, positionalArgs, cfg.shellTimeout)
		if err != nil {
			expandErr = fmt.Errorf("shell expansion !{%s}: %w", command, err)
			sb.WriteString(match)
			continue
		}
		// Insert command output verbatim — never re-scanned for
		// placeholders, so output cannot trigger further expansion.
		sb.WriteString(strings.TrimSpace(output))
	}
	// Remaining literal template text after the last !{...} block.
	sb.WriteString(substituteArgs(s.Instructions[last:], args, positionalArgs))

	if expandErr != nil {
		return sb.String(), expandErr
	}
	return sb.String(), nil
}

// ExpandArguments replaces $ARGUMENTS and $1-$9 placeholders only (no shell).
// This is a convenience method for simple use cases.
func (s *Skill) ExpandArguments(args string) string {
	return s.expandVariables(args)
}

// expandVariables handles $ARGUMENTS and positional $N substitution.
func (s *Skill) expandVariables(argsString string) string {
	return substituteArgs(s.Instructions, argsString, strings.Fields(argsString))
}

// substituteArgs replaces $1-$9 positional placeholders and $ARGUMENTS in the
// given text. It performs plain text substitution and must never be applied
// to text that will subsequently be shell-expanded.
func substituteArgs(text, argsString string, positionalArgs []string) string {
	// Replace positional arguments $1, $2, etc.
	result := positionalArgPattern.ReplaceAllStringFunc(text, func(match string) string {
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
// Skill arguments are passed as data, not code: positional args become shell
// positional parameters ($1, $2, ...) and the full argument string is
// exported as $ARGUMENTS, so the command can reference them without the
// argument text ever being interpreted as shell syntax.
func runShellCommand(ctx context.Context, command, argsString string, positionalArgs []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// "skill" is $0; positional args follow as $1, $2, ...
	shellArgs := append([]string{"-c", command, "skill"}, positionalArgs...)
	cmd := exec.CommandContext(ctx, "sh", shellArgs...)
	cmd.Env = append(os.Environ(), "ARGUMENTS="+argsString)
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
