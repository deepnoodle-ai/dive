package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/wonton/fetch"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestClient_Fetch_V2API(t *testing.T) {
	// Mock server that returns a v2 API response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/scrape", r.URL.Path)
		assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse and validate the request body
		var reqBody scrapeRequestBody
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Equal(t, "https://example.com", reqBody.URL)
		assert.NotEmpty(t, reqBody.Formats)

		// Return a mock v2 response
		response := scrapeResponse{
			Success: true,
			Data: &document{
				Markdown: "# Example\n\nThis is a test page.",
				HTML:     stringPtr("<h1>Example</h1><p>This is a test page.</p>"),
				Summary:  stringPtr("A test page with example content."),
				Links:    []string{"https://example.com/link1", "https://example.com/link2"},
				Metadata: &documentMetadata{
					Title:       "Example Page",
					Description: "An example page for testing",
					Language:    stringPtr("en"),
					SourceURL:   "https://example.com",
					StatusCode:  200,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Create client with test server
	client, err := New(
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
	)
	assert.NoError(t, err)

	// Test the fetch operation
	req := &fetch.Request{
		URL:     "https://example.com",
		Formats: []string{"markdown", "html", "summary", "links"},
	}

	output, err := client.Fetch(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, output)

	// Verify the response
	assert.Equal(t, "# Example\n\nThis is a test page.", output.Markdown)
	assert.Equal(t, "<h1>Example</h1><p>This is a test page.</p>", output.HTML)
	assert.Equal(t, "A test page with example content.", output.Summary)
	assert.Len(t, output.Links, 2)
	assert.Equal(t, "https://example.com/link1", output.Links[0].URL)
	assert.Equal(t, "https://example.com/link2", output.Links[1].URL)
	assert.Equal(t, "Example Page", output.Metadata.Title)
	assert.Equal(t, "An example page for testing", output.Metadata.Description)
	assert.Equal(t, "https://example.com", output.Metadata.Canonical)
	assert.Equal(t, 200, output.StatusCode)
}

func TestClient_Fetch_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
		errCode    int
	}{
		{
			name:       "payment required",
			statusCode: 402,
			wantErr:    true,
			errCode:    402,
		},
		{
			name:       "rate limit exceeded",
			statusCode: 429,
			wantErr:    true,
			errCode:    429,
		},
		{
			name:       "server error",
			statusCode: 500,
			wantErr:    true,
			errCode:    500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"error": "test error"}`))
			}))
			defer server.Close()

			client, err := New(
				WithAPIKey("test-api-key"),
				WithBaseURL(server.URL),
			)
			assert.NoError(t, err)

			req := &fetch.Request{
				URL: "https://example.com",
			}

			_, err = client.Fetch(context.Background(), req)
			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, fetch.IsRequestError(err), "expected RequestError")
				reqErr := err.(*fetch.RequestError)
				assert.Equal(t, tt.errCode, reqErr.StatusCode())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestClient_Fetch_DefaultFormats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody scrapeRequestBody
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// Should default to markdown format when no formats specified
		assert.Len(t, reqBody.Formats, 1)
		formatStr, ok := reqBody.Formats[0].(string)
		assert.True(t, ok)
		assert.Equal(t, "markdown", formatStr)

		response := scrapeResponse{
			Success: true,
			Data: &document{
				Markdown: "# Default Format Test",
				Metadata: &documentMetadata{
					Title:     "Test",
					SourceURL: "https://example.com",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := New(
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
	)
	assert.NoError(t, err)

	req := &fetch.Request{
		URL: "https://example.com",
		// No formats specified - should default to markdown
	}

	output, err := client.Fetch(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, "# Default Format Test", output.Markdown)
}

func TestClient_New_MissingAPIKey(t *testing.T) {
	// Temporarily unset the environment variable
	t.Setenv("FIRECRAWL_API_KEY", "")

	_, err := New()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no api key provided")
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
