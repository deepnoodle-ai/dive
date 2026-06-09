package permission

import (
	"strings"
)

// SplitCommand splits a shell command into segments separated by unquoted
// control operators: ;, &&, ||, |, &, and newlines. Quoting is respected:
// operators inside single or double quotes (or escaped with a backslash) do
// not split. The returned hasSubstitution is true when the command contains
// command or process substitution outside single quotes ($(...), backticks,
// <(...), >(...)), which means parts of the command cannot be validated by
// pattern matching.
func SplitCommand(command string) (segments []string, hasSubstitution bool) {
	var cur strings.Builder
	inSingle := false
	inDouble := false
	flush := func() {
		s := strings.TrimSpace(cur.String())
		if s != "" {
			segments = append(segments, s)
		}
		cur.Reset()
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			cur.WriteByte(c)
			continue
		}
		if c == '\\' && i+1 < len(command) {
			cur.WriteByte(c)
			i++
			cur.WriteByte(command[i])
			continue
		}
		switch c {
		case '\'':
			if !inDouble {
				inSingle = true
			}
			cur.WriteByte(c)
		case '"':
			inDouble = !inDouble
			cur.WriteByte(c)
		case '`':
			// Backticks substitute even inside double quotes.
			hasSubstitution = true
			cur.WriteByte(c)
		case '$':
			// $(...) substitutes even inside double quotes.
			if i+1 < len(command) && command[i+1] == '(' {
				hasSubstitution = true
			}
			cur.WriteByte(c)
		case '<', '>':
			if !inDouble && i+1 < len(command) && command[i+1] == '(' {
				hasSubstitution = true
			}
			cur.WriteByte(c)
		case ';', '\n':
			if inDouble {
				cur.WriteByte(c)
				continue
			}
			flush()
		case '&', '|':
			if inDouble {
				cur.WriteByte(c)
				continue
			}
			flush()
			if i+1 < len(command) && command[i+1] == c {
				i++ // skip the second char of && or ||
			}
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return segments, hasSubstitution
}

// MatchCommandAllow reports whether a shell command is authorized by an
// allow-side specifier pattern. The command is split on unquoted shell
// control operators and EVERY segment must match the pattern; a compound
// command like "go test ./...; rm -rf /" therefore does not match
// "go test *". Commands containing command or process substitution never
// match, since the substituted command cannot be validated.
func MatchCommandAllow(pattern, command string) bool {
	segments, hasSubstitution := SplitCommand(command)
	if hasSubstitution || len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if !MatchGlob(pattern, segment) {
			return false
		}
	}
	return true
}

// MatchCommandDeny reports whether a shell command is blocked by a deny-side
// specifier pattern. The pattern is matched against the full command and
// against each segment after splitting on unquoted shell control operators,
// so "ls\nrm -rf /" is still caught by "rm -rf*".
func MatchCommandDeny(pattern, command string) bool {
	if MatchGlob(pattern, command) {
		return true
	}
	segments, _ := SplitCommand(command)
	for _, segment := range segments {
		if MatchGlob(pattern, segment) {
			return true
		}
	}
	return false
}
