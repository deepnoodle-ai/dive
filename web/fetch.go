package web

import (
	"context"
	"fmt"
)

type FetchInput struct {
	URL             string            `json:"url"`
	Formats         []string          `json:"formats,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeTags     []string          `json:"include_tags,omitempty"`
	ExcludeTags     []string          `json:"exclude_tags,omitempty"`
	OnlyMainContent bool              `json:"only_main_content,omitempty"`
	WaitFor         int               `json:"wait_for,omitempty"`
	Timeout         int               `json:"timeout,omitempty"`
	ParsePDF        bool              `json:"parse_pdf,omitempty"`
}

type Fetcher interface {
	Fetch(ctx context.Context, input *FetchInput) (*Document, error)
}

type FetchError struct {
	StatusCode int
	Err        error
}

func NewFetchError(statusCode int, err error) *FetchError {
	return &FetchError{StatusCode: statusCode, Err: err}
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch failed with status code %d: %s", e.StatusCode, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

func (e *FetchError) IsRecoverable() bool {
	return e.StatusCode == 429 || // Too Many Requests
		e.StatusCode == 500 || // Internal Server Error
		e.StatusCode == 502 || // Bad Gateway
		e.StatusCode == 503 || // Service Unavailable
		e.StatusCode == 504 // Gateway Timeout
}
