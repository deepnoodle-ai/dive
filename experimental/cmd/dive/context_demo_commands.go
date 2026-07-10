package main

import (
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/permission"
)

// shellInvocations returns normalized direct command segments. The boolean is
// true when command or process substitution makes the invocation shape dynamic.
func shellInvocations(command string) ([][]string, bool) {
	segments, hasSubstitution := permission.SplitCommand(strings.ToLower(command))
	invocations := make([][]string, 0, len(segments))
	for _, segment := range segments {
		fields := normalizeShellFields(strings.Fields(segment))
		if len(fields) > 0 {
			invocations = append(invocations, fields)
		}
	}
	return invocations, hasSubstitution
}

// normalizeShellFields unwraps only transparent, non-shell launchers. Shell
// interpreters such as bash -c remain opaque because their argument is code.
func normalizeShellFields(fields []string) []string {
	for len(fields) > 0 && isShellAssignment(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) > 0 && filepath.Base(fields[0]) == "command" {
		fields = fields[1:]
	}
	if len(fields) > 0 && filepath.Base(fields[0]) == "env" {
		fields = fields[1:]
		for len(fields) > 0 && isShellAssignment(fields[0]) {
			fields = fields[1:]
		}
	}
	return fields
}

func finalShellInvocation(command string) ([]string, bool) {
	invocations, hasSubstitution := shellInvocations(command)
	if hasSubstitution || len(invocations) == 0 {
		return nil, false
	}
	return invocations[len(invocations)-1], true
}

func shellArgument(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return strings.Trim(fields[index], `"'`)
}

func isShellAssignment(value string) bool {
	name, _, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}
