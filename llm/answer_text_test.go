package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMessageAnswerText(t *testing.T) {
	phase := func(text, p string) *TextContent {
		return &TextContent{Text: text, Metadata: ProviderMetadata{textPhaseMetadataKey: p}}
	}
	tests := []struct {
		name    string
		content []Content
		want    string
	}{
		{"empty", nil, ""},
		{"no text", []Content{&ThinkingContent{Thinking: "hmm"}}, ""},
		{
			"citation fragments join with no separator",
			[]Content{&TextContent{Text: "Born on "}, &TextContent{Text: "April 30"}, &TextContent{Text: "."}},
			"Born on April 30.",
		},
		{
			"other content separates passages",
			[]Content{
				&TextContent{Text: "Searching."},
				&ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search"},
				&WebSearchToolResultContent{ToolUseID: "srvtoolu_1"},
				&TextContent{Text: "Found it."},
			},
			"Searching.\n\nFound it.",
		},
		{
			"empty blocks neither join nor separate",
			[]Content{&TextContent{Text: "a"}, &TextContent{Text: ""}, &TextContent{Text: "b"}},
			"ab",
		},
		{
			"a phase change separates passages",
			[]Content{phase("Checking the docs.", "commentary"), phase("The answer is 42.", "final_answer")},
			"Checking the docs.\n\nThe answer is 42.",
		},
		{
			"same phase joins",
			[]Content{phase("The answer ", "final_answer"), phase("is 42.", "final_answer")},
			"The answer is 42.",
		},
		{
			"unlabeled then labeled separates",
			[]Content{&TextContent{Text: "a"}, phase("b", "final_answer")},
			"a\n\nb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Message{Role: Assistant, Content: tt.content}
			assert.Equal(t, tt.want, m.AnswerText())
		})
	}
}
