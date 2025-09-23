package web

import "context"

type SearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

type SearchOutput struct {
	Items []*SearchItem `json:"items"`
}

type SearchItem struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Image       string `json:"image,omitempty"`
}

type Searcher interface {
	Search(ctx context.Context, input *SearchInput) (*SearchOutput, error)
}
