package skill

import (
	"context"
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

func TestExpand_NoPlaceholders(t *testing.T) {
	s := &Skill{Instructions: "No placeholders here."}
	result, err := s.Expand(context.Background(), "some args", WithShellExpansion(true))
	assert.NoError(t, err)
	assert.Equal(t, "No placeholders here.", result)
}
