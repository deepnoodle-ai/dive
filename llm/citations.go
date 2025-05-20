package llm

// CitationSettings contains settings for citations in a message.
type CitationSettings struct {
	Enabled bool `json:"enabled"`
}

type Citation interface {
	IsCitation() bool
}

// DocumentCitation is a citation to a specific part of a document.
type DocumentCitation struct {
	Type           string `json:"type"` // "char_location"
	CitedText      string `json:"cited_text,omitempty"`
	DocumentIndex  int    `json:"document_index,omitempty"`
	DocumentTitle  string `json:"document_title,omitempty"`
	StartCharIndex int    `json:"start_char_index,omitempty"`
	EndCharIndex   int    `json:"end_char_index,omitempty"`
}

func (c *DocumentCitation) IsCitation() bool {
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
	EncryptedIndex string `json:"encrypted_index"`
	CitedText      string `json:"cited_text"`
}

func (c *WebSearchResultLocation) IsCitation() bool {
	return true
}
