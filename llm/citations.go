package llm

import (
	"encoding/json"
	"fmt"
)

// CitationType identifies the kind of citation attached to a text content block.
type CitationType string

const (
	CitationTypeCharLocation            CitationType = "char_location"
	CitationTypeWebSearchResultLocation CitationType = "web_search_result_location"
	CitationTypeURLCitation             CitationType = "url_citation"
)

// CitationSettings contains settings for citations in a message.
type CitationSettings struct {
	Enabled bool `json:"enabled"`
}

// Citation is a reference to source material attached to generated text.
type Citation interface {
	IsCitation() bool
}

// CharLocation is a citation to a specific part of a document.
type CharLocation struct {
	Type           string `json:"type"` // "char_location"
	CitedText      string `json:"cited_text,omitempty"`
	DocumentIndex  int    `json:"document_index,omitempty"`
	DocumentTitle  string `json:"document_title,omitempty"`
	StartCharIndex int    `json:"start_char_index,omitempty"`
	EndCharIndex   int    `json:"end_char_index,omitempty"`
}

func (c *CharLocation) IsCitation() bool {
	return true
}

/*
   {
     "type": "web_search_result_location",
     "url": "https://en.wikipedia.org/wiki/Claude_Shannon",
     "title": "Claude Shannon - Wikipedia",
     "encrypted_index": "Eo8BCioIAhgBIiQyYjQ0OWJmZi1lNm..",
     "cited_text": "Claude Elwood Shannon (April 30, 1916 – ..."
   }
*/

// WebSearchResultLocation is a citation to a specific part of a web page.
type WebSearchResultLocation struct {
	Type           string `json:"type"` // "web_search_result_location"
	URL            string `json:"url"`
	Title          string `json:"title"`
	EncryptedIndex string `json:"encrypted_index,omitempty"`
	CitedText      string `json:"cited_text,omitempty"`
}

func (c *WebSearchResultLocation) IsCitation() bool {
	return true
}

type citationTypeIndicator struct {
	Type CitationType `json:"type"`
}

func unmarshalCitation(data []byte) (Citation, error) {
	var ct citationTypeIndicator
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}
	switch ct.Type {
	case CitationTypeCharLocation:
		var c *CharLocation
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		return c, nil
	case CitationTypeWebSearchResultLocation:
		var c *WebSearchResultLocation
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		return c, nil
	default:
		return nil, fmt.Errorf("unknown citation type: %s", ct.Type)
	}
}

func unmarshalCitations(data []byte) ([]Citation, error) {
	var results []Citation
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	for _, item := range items {
		citation, err := unmarshalCitation(item)
		if err != nil {
			return nil, err
		}
		results = append(results, citation)
	}
	return results, nil
}
