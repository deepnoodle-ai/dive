package dive

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/deepnoodle-ai/dive/llm"
)

// UnknownToolError reports a tool call whose name matched nothing in the
// toolset declared for the request. No tool was dispatched.
type UnknownToolError struct {
	Name        string
	Suggestions []string
}

func (e *UnknownToolError) Error() string {
	return fmt.Sprintf("unknown tool %q", e.Name)
}

func unknownToolResult(call *llm.ToolUseContent, toolsByName map[string]Tool) *ToolCallResult {
	suggestions := suggestToolNames(call.Name, toolsByName)
	return &ToolCallResult{
		ID:    call.ID,
		Name:  call.Name,
		Input: call.Input,
		Result: NewToolResultError(
			unknownToolMessage(call.Name, suggestions),
		),
		Error: &UnknownToolError{
			Name:        call.Name,
			Suggestions: suggestions,
		},
	}
}

func unknownToolMessage(name string, suggestions []string) string {
	message := fmt.Sprintf("Tool %q does not exist and was not called.", name)
	if len(suggestions) > 0 {
		message += " Did you mean: " + strings.Join(suggestions, ", ") + "."
	}
	return message + " Call one of the tools declared for this turn."
}

const maxUnknownToolSuggestions = 3

type toolNameMatch struct {
	name      string
	primary   int
	secondary int
}

// suggestToolNames returns candidates from the first matching tier only:
// segment suffix, bounded edit distance, then exact namespace. Each tier has
// a deterministic total ordering so map iteration cannot affect the message.
func suggestToolNames(name string, toolsByName map[string]Tool) []string {
	querySegments := splitToolName(name)

	var suffixMatches []toolNameMatch
	for candidate := range toolsByName {
		candidateSegments := splitToolName(candidate)
		segments, chars := commonSegmentSuffix(querySegments, candidateSegments)
		if segments >= 2 || (segments == 1 && chars >= 6) {
			suffixMatches = append(suffixMatches, toolNameMatch{
				name:      candidate,
				primary:   segments,
				secondary: chars,
			})
		}
	}
	if len(suffixMatches) > 0 {
		sort.Slice(suffixMatches, func(i, j int) bool {
			if suffixMatches[i].primary != suffixMatches[j].primary {
				return suffixMatches[i].primary > suffixMatches[j].primary
			}
			if suffixMatches[i].secondary != suffixMatches[j].secondary {
				return suffixMatches[i].secondary > suffixMatches[j].secondary
			}
			return suffixMatches[i].name < suffixMatches[j].name
		})
		return matchNames(suffixMatches)
	}

	maxDistance := min(3, utf8.RuneCountInString(name)/4)
	var editMatches []toolNameMatch
	if maxDistance > 0 {
		for candidate := range toolsByName {
			distance := levenshteinDistance(name, candidate)
			if distance <= maxDistance {
				editMatches = append(editMatches, toolNameMatch{
					name:    candidate,
					primary: distance,
				})
			}
		}
	}
	if len(editMatches) > 0 {
		sort.Slice(editMatches, func(i, j int) bool {
			if editMatches[i].primary != editMatches[j].primary {
				return editMatches[i].primary < editMatches[j].primary
			}
			return editMatches[i].name < editMatches[j].name
		})
		return matchNames(editMatches)
	}

	if len(querySegments) < 2 {
		return nil
	}
	queryNamespace := querySegments[:len(querySegments)-1]
	var prefixMatches []toolNameMatch
	for candidate := range toolsByName {
		candidateSegments := splitToolName(candidate)
		if len(candidateSegments) < 2 {
			continue
		}
		candidateNamespace := candidateSegments[:len(candidateSegments)-1]
		if equalSegments(queryNamespace, candidateNamespace) {
			prefixMatches = append(prefixMatches, toolNameMatch{
				name:      candidate,
				primary:   len(candidateNamespace),
				secondary: segmentCharacterCount(candidateNamespace),
			})
		}
	}
	sort.Slice(prefixMatches, func(i, j int) bool {
		if prefixMatches[i].primary != prefixMatches[j].primary {
			return prefixMatches[i].primary > prefixMatches[j].primary
		}
		if prefixMatches[i].secondary != prefixMatches[j].secondary {
			return prefixMatches[i].secondary > prefixMatches[j].secondary
		}
		return prefixMatches[i].name < prefixMatches[j].name
	})
	return matchNames(prefixMatches)
}

func splitToolName(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
}

func commonSegmentSuffix(a, b []string) (segments, chars int) {
	for ai, bi := len(a)-1, len(b)-1; ai >= 0 && bi >= 0 && a[ai] == b[bi]; ai, bi = ai-1, bi-1 {
		segments++
		chars += utf8.RuneCountInString(a[ai])
	}
	return segments, chars
}

func segmentCharacterCount(segments []string) int {
	count := 0
	for _, segment := range segments {
		count += utf8.RuneCountInString(segment)
	}
	return count
}

func equalSegments(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func matchNames(matches []toolNameMatch) []string {
	limit := min(maxUnknownToolSuggestions, len(matches))
	if limit == 0 {
		return nil
	}
	names := make([]string, limit)
	for i := range limit {
		names[i] = matches[i].name
	}
	return names
}

func levenshteinDistance(a, b string) int {
	aRunes := []rune(a)
	bRunes := []rune(b)
	if len(aRunes) < len(bRunes) {
		aRunes, bRunes = bRunes, aRunes
	}
	previous := make([]int, len(bRunes)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, aRune := range aRunes {
		current := make([]int, len(bRunes)+1)
		current[0] = i + 1
		for j, bRune := range bRunes {
			cost := 0
			if aRune != bRune {
				cost = 1
			}
			current[j+1] = min(
				previous[j+1]+1,
				current[j]+1,
				previous[j]+cost,
			)
		}
		previous = current
	}
	return previous[len(bRunes)]
}
