package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalCitation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Citation
		wantErr  bool
	}{
		{
			name: "document citation",
			input: `{
				"type": "char_location",
				"cited_text": "This is a cited text",
				"document_index": 1,
				"document_title": "Test Document",
				"start_char_index": 10,
				"end_char_index": 30
			}`,
			expected: &CharLocation{
				Type:           "char_location",
				CitedText:      "This is a cited text",
				DocumentIndex:  1,
				DocumentTitle:  "Test Document",
				StartCharIndex: 10,
				EndCharIndex:   30,
			},
			wantErr: false,
		},
		{
			name: "web search result location",
			input: `{
				"type": "web_search_result_location",
				"url": "https://example.com/page",
				"title": "Example Page",
				"encrypted_index": "test-encrypted-index",
				"cited_text": "This is cited from a web page"
			}`,
			expected: &WebSearchResultLocation{
				Type:           "web_search_result_location",
				URL:            "https://example.com/page",
				Title:          "Example Page",
				EncryptedIndex: "test-encrypted-index",
				CitedText:      "This is cited from a web page",
			},
			wantErr: false,
		},
		{
			name:     "invalid json",
			input:    `{ invalid json }`,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "unknown citation type",
			input:    `{ "type": "unknown_type" }`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unmarshalCitation([]byte(tt.input))

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
			assert.True(t, result.IsCitation())
		})
	}
}

func TestUnmarshalCitations(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Citation
		wantErr  bool
	}{
		{
			name: "multiple citations",
			input: `[
				{
					"type": "char_location",
					"cited_text": "Document text",
					"document_index": 1,
					"document_title": "Test Document",
					"start_char_index": 10,
					"end_char_index": 30
				},
				{
					"type": "web_search_result_location",
					"url": "https://example.com/page",
					"title": "Example Page",
					"encrypted_index": "test-encrypted-index",
					"cited_text": "Web page text"
				}
			]`,
			expected: []Citation{
				&CharLocation{
					Type:           "char_location",
					CitedText:      "Document text",
					DocumentIndex:  1,
					DocumentTitle:  "Test Document",
					StartCharIndex: 10,
					EndCharIndex:   30,
				},
				&WebSearchResultLocation{
					Type:           "web_search_result_location",
					URL:            "https://example.com/page",
					Title:          "Example Page",
					EncryptedIndex: "test-encrypted-index",
					CitedText:      "Web page text",
				},
			},
			wantErr: false,
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "invalid json",
			input:    `not a json array`,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "invalid citation in array",
			input: `[
				{
					"type": "char_location",
					"cited_text": "Valid citation"
				},
				{
					"type": "unknown_type"
				}
			]`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := unmarshalCitations([]byte(tt.input))

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCharLocation_IsCitation(t *testing.T) {
	citation := &CharLocation{}
	assert.True(t, citation.IsCitation())
}

func TestWebSearchResultLocation_IsCitation(t *testing.T) {
	citation := &WebSearchResultLocation{}
	assert.True(t, citation.IsCitation())
}
