package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestExpandArguments(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		args         string
		want         string
	}{
		{
			name:         "single positional argument",
			instructions: "Fix issue #$1",
			args:         "123",
			want:         "Fix issue #123",
		},
		{
			name:         "multiple positional arguments",
			instructions: "Fix issue #$1 with priority $2",
			args:         "123 high",
			want:         "Fix issue #123 with priority high",
		},
		{
			name:         "ARGUMENTS placeholder",
			instructions: "Process: $ARGUMENTS",
			args:         "file1.txt file2.txt",
			want:         "Process: file1.txt file2.txt",
		},
		{
			name:         "mixed placeholders",
			instructions: "Command: $1, Priority: $2, All: $ARGUMENTS",
			args:         "cmd high extra",
			want:         "Command: cmd, Priority: high, All: cmd high extra",
		},
		{
			name:         "unused positional placeholder",
			instructions: "Arg1: $1, Arg2: $2, Arg3: $3",
			args:         "one two",
			want:         "Arg1: one, Arg2: two, Arg3: $3",
		},
		{
			name:         "empty arguments",
			instructions: "Process: $ARGUMENTS, First: $1",
			args:         "",
			want:         "Process: , First: $1",
		},
		{
			name:         "no placeholders",
			instructions: "Just do the thing.",
			args:         "ignored args",
			want:         "Just do the thing.",
		},
		{
			name:         "repeated placeholders",
			instructions: "First: $1, Again: $1",
			args:         "value",
			want:         "First: value, Again: value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Skill{Instructions: tt.instructions}
			result := s.ExpandArguments(tt.args)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestExpand_ShellDisabledByDefault(t *testing.T) {
	s := &Skill{Instructions: "Branch: !{echo main}"}
	result, err := s.Expand(context.Background(), "")
	assert.NoError(t, err)
	// Shell expansion disabled by default, placeholder left as-is
	assert.Equal(t, "Branch: !{echo main}", result)
}

func TestExpand_ShellEnabled(t *testing.T) {
	s := &Skill{Instructions: "Result: !{echo hello}"}
	result, err := s.Expand(context.Background(), "", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "Result: hello", result)
}

func TestExpand_ShellTimeout(t *testing.T) {
	s := &Skill{Instructions: "Result: !{sleep 10}"}
	_, err := s.Expand(context.Background(), "",
		WithShellExpansion(true),
		WithShellTimeout(100*time.Millisecond),
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestExpand_ShellError(t *testing.T) {
	s := &Skill{Instructions: "Result: !{false}"}
	_, err := s.Expand(context.Background(), "", WithShellExpansion(true))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shell expansion")
}

func TestExpand_MixedExpansion(t *testing.T) {
	s := &Skill{Instructions: "Deploy $1 to $2. User: !{whoami}. Full: $ARGUMENTS"}
	result, err := s.Expand(context.Background(), "app staging", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Contains(t, result, "Deploy app to staging")
	assert.Contains(t, result, "Full: app staging")
	// whoami should have been replaced with something non-empty
	assert.NotContains(t, result, "!{whoami}")
}

func TestExpand_ShellWithEmptyArgs(t *testing.T) {
	s := &Skill{Instructions: "Branch: !{echo main}. Args: $ARGUMENTS"}
	result, err := s.Expand(context.Background(), "", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "Branch: main. Args: ", result)
}

func TestExpand_ArgInjectionNotExecuted(t *testing.T) {
	// Regression test for shell-expansion injection: model-controlled args
	// containing !{...} must never be executed, even with shell expansion
	// enabled. The injected sequence must appear as literal text.
	marker := filepath.Join(t.TempDir(), "pwned")
	hostileArgs := fmt.Sprintf("Bob !{touch %s}", marker)

	s := &Skill{Instructions: "Hello $ARGUMENTS. Branch: !{echo main}"}
	result, err := s.Expand(context.Background(), hostileArgs, WithShellExpansion(true))
	assert.NoError(t, err)

	// The template's own shell block ran.
	assert.Contains(t, result, "Branch: main")
	// The injected !{...} appears literally in the output.
	assert.Contains(t, result, fmt.Sprintf("!{touch %s}", marker))
	// The injected command did NOT run.
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "injected command must not execute")
}

func TestExpand_ArgInjectionViaPositional(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	// A whitespace-free injection so it survives positional splitting intact:
	// `>file` would create the file if the shell executed it.
	hostileArg := fmt.Sprintf("!{>%s}", marker)
	s := &Skill{Instructions: "First: $1. Branch: !{echo main}"}
	result, err := s.Expand(context.Background(), hostileArg, WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Contains(t, result, "Branch: main")
	assert.Contains(t, result, "First: "+hostileArg)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "injected command must not execute")
}

func TestExpand_ArgsInsideShellBlock(t *testing.T) {
	// Template authors can reference arguments inside !{...}: positional
	// args are shell positional parameters and $ARGUMENTS is exported in
	// the environment. The shell receives them as data, not code.
	s := &Skill{Instructions: `Result: !{echo "first=$1 all=$ARGUMENTS"}`}
	result, err := s.Expand(context.Background(), "foo bar", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "Result: first=foo all=foo bar", result)
}

func TestExpand_ArgsInsideShellBlockAreData(t *testing.T) {
	// Hostile args referenced inside a shell block must be expanded as data
	// by the shell — command substitution syntax in args must not execute.
	marker := filepath.Join(t.TempDir(), "pwned")
	s := &Skill{Instructions: `Result: !{echo "$ARGUMENTS"}`}
	hostileArgs := fmt.Sprintf("$(touch %s)", marker)
	result, err := s.Expand(context.Background(), hostileArgs, WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Contains(t, result, hostileArgs)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "command substitution in args must not execute")
}

func TestExpand_ShellOutputNotReexpanded(t *testing.T) {
	// Shell output is inserted verbatim: placeholders or !{...} sequences
	// in command output must not be substituted or executed. The command
	// emits "!{echo nested}" via printf escapes (\041 = "!", \175 = "}")
	// because a !{command} block cannot itself contain a literal "}".
	s := &Skill{Instructions: `Result: !{printf '$1 $ARGUMENTS \041{echo nested\175'}`}
	result, err := s.Expand(context.Background(), "foo", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "Result: $1 $ARGUMENTS !{echo nested}", result)
}

func TestExpand_NoPlaceholders(t *testing.T) {
	s := &Skill{Instructions: "No placeholders here."}
	result, err := s.Expand(context.Background(), "some args", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "No placeholders here.", result)
}
