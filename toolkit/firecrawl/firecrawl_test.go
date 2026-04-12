package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/fetch"
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
			name:       "bad request",
			statusCode: 400,
			wantErr:    true,
			errCode:    400,
		},
		{
			name:       "unauthorized",
			statusCode: 401,
			wantErr:    true,
			errCode:    401,
		},
		{
			name:       "payment required",
			statusCode: 402,
			wantErr:    true,
			errCode:    402,
		},
		{
			name:       "not found",
			statusCode: 404,
			wantErr:    true,
			errCode:    404,
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
		{
			name:       "gateway unavailable",
			statusCode: 503,
			wantErr:    true,
			errCode:    503,
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

func TestClient_Fetch_V2DefaultsAreNotForced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody scrapeRequestBody
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		assert.Nil(t, reqBody.OnlyMainContent)
		assert.Nil(t, reqBody.RemoveBase64Images)
		assert.Nil(t, reqBody.BlockAds)
		assert.Nil(t, reqBody.Proxy)
		assert.Nil(t, reqBody.StoreInCache)

		response := scrapeResponse{
			Success: true,
			Data: &document{
				Markdown: "# Defaults",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := New(WithAPIKey("test"), WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
	assert.NoError(t, err)
}

func TestClient_Fetch_OnlyMainContentFalseIsOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody scrapeRequestBody
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)
		assert.Nil(t, reqBody.OnlyMainContent)

		response := scrapeResponse{
			Success: true,
			Data: &document{
				Markdown: "# Main content",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := New(WithAPIKey("test"), WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.Fetch(context.Background(), &fetch.Request{
		URL:             "https://example.com",
		OnlyMainContent: false,
	})
	assert.NoError(t, err)
}

func TestClient_Fetch_RawHTMLFormatRenamed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody scrapeRequestBody
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		assert.NoError(t, err)

		// wonton's "raw_html" must be translated to Firecrawl's "rawHtml".
		assert.Len(t, reqBody.Formats, 1)
		formatStr, ok := reqBody.Formats[0].(string)
		assert.True(t, ok)
		assert.Equal(t, "rawHtml", formatStr)

		response := scrapeResponse{
			Success: true,
			Data: &document{
				RawHTML:  stringPtr("<html><body>raw</body></html>"),
				Metadata: &documentMetadata{SourceURL: "https://example.com"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, err := New(WithAPIKey("test"), WithBaseURL(server.URL))
	assert.NoError(t, err)

	output, err := client.Fetch(context.Background(), &fetch.Request{
		URL:     "https://example.com",
		Formats: []string{"raw_html"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "<html><body>raw</body></html>", output.RawHTML)
}

func TestKeywordsField_Unmarshal(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{"array", `["go","testing"]`, []string{"go", "testing"}},
		{"comma string", `"go, testing,  ai "`, []string{"go", "testing", "ai"}},
		{"empty string", `""`, []string{}},
		{"null", `null`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var k keywordsField
			err := k.UnmarshalJSON([]byte(tt.json))
			assert.NoError(t, err)
			assert.Equal(t, tt.want, []string(k))
		})
	}
}

func TestClient_Fetch_KeywordsMappedToMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a metadata blob with keywords as a comma-separated string,
		// which is the most common shape from <meta name="keywords">.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"success": true,
			"data": {
				"markdown": "# Hi",
				"metadata": {
					"title": "Hi",
					"keywords": "go, testing, ai",
					"sourceURL": "https://example.com",
					"statusCode": 200
				}
			}
		}`))
	}))
	defer server.Close()

	client, err := New(WithAPIKey("test"), WithBaseURL(server.URL))
	assert.NoError(t, err)

	output, err := client.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
	assert.NoError(t, err)
	assert.Equal(t, []string{"go", "testing", "ai"}, output.Metadata.Keywords)
}

func TestClient_Fetch_UnauthorizedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	client, err := New(WithAPIKey("test"), WithBaseURL(server.URL))
	assert.NoError(t, err)

	_, err = client.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
	assert.Error(t, err)
	assert.True(t, fetch.IsRequestError(err))
	assert.Equal(t, 401, err.(*fetch.RequestError).StatusCode())
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
