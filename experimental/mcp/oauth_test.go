package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestNewMCPClient_WithoutOAuth(t *testing.T) {
	config := &ServerConfig{
		Type:  "http",
		Name:  "test-server",
		URL:   "https://example.com",
		OAuth: nil,
	}

	client, err := NewClient(config)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Nil(t, client.oauthConfig)
	assert.Equal(t, config, client.config)
	assert.False(t, client.connected)
}

func TestNewMCPClient_WithOAuth(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "oauth-server",
		URL:  "https://example.com",
		OAuth: &OAuthConfig{
			ClientID: "test-client",
		},
	}

	client, err := NewClient(config)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.NotNil(t, client.oauthConfig)
	assert.Equal(t, "dive", client.oauthConfig.ClientID)
	assert.Equal(t, "http://localhost:8085/oauth/callback", client.oauthConfig.RedirectURI)
	assert.True(t, client.oauthConfig.PKCEEnabled)
	assert.Equal(t, []string{"mcp.read", "mcp.write"}, client.oauthConfig.Scopes)
}

func TestMCPClient_OAuth_IsConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	assert.NoError(t, err)
	assert.False(t, client.IsConnected())

	// Simulate connection
	client.connected = true
	assert.True(t, client.IsConnected())
}

func TestMCPClient_OAuth_Close(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	assert.NoError(t, err)

	client.connected = true
	err = client.Close()
	assert.NoError(t, err)
}

func TestMCPClient_OAuth_ListTools_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	assert.NoError(t, err)

	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	assert.Error(t, err)
	assert.Nil(t, tools)
	assert.Contains(t, err.Error(), "not connected")
}

func TestMCPClient_OAuth_ListResources_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	assert.NoError(t, err)

	ctx := context.Background()
	resources, err := client.ListResources(ctx)
	assert.Error(t, err)
	assert.Nil(t, resources)
	assert.Contains(t, err.Error(), "not connected")
}

func TestMCPClient_OAuth_CallTool_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	assert.NoError(t, err)

	ctx := context.Background()
	result, err := client.CallTool(ctx, "test-tool", map[string]interface{}{})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not connected")
}

func TestMCPClient_OAuthConfiguration_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *ServerConfig
		expectOAuth bool
		expectError bool
	}{
		{
			name: "no OAuth configuration",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "https://example.com",
			},
			expectOAuth: false,
			expectError: false,
		},
		{
			name: "OAuth enabled",
			config: &ServerConfig{
				Type: "http",
				Name: "oauth-server",
				URL:  "https://example.com",
				OAuth: &OAuthConfig{
					ClientID: "test-client",
				},
			},
			expectOAuth: true,
			expectError: false,
		},
		{
			name: "stdio with OAuth (should work)",
			config: &ServerConfig{
				Type: "stdio",
				Name: "stdio-oauth-server",
				URL:  "test-command",
				OAuth: &OAuthConfig{
					ClientID: "test-client",
				},
			},
			expectOAuth: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, client)

			if tt.expectOAuth {
				assert.NotNil(t, client.oauthConfig)
			} else {
				assert.Nil(t, client.oauthConfig)
			}
		})
	}
}

func TestOAuthCallbackHandler_Success(t *testing.T) {
	callbackChan := make(chan map[string]string, 1)
	handler := oauthCallbackHandler(callbackChan)

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=abc123&state=xyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Authorization Successful")

	params := <-callbackChan
	assert.Equal(t, "abc123", params["code"])
	assert.Equal(t, "xyz", params["state"])
}

func TestOAuthCallbackHandler_Error(t *testing.T) {
	callbackChan := make(chan map[string]string, 1)
	handler := oauthCallbackHandler(callbackChan)

	req := httptest.NewRequest(http.MethodGet,
		"/oauth/callback?error=access_denied&error_description=User+denied+access&state=xyz", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Authorization Failed")
	assert.Contains(t, body, "access_denied")
	assert.Contains(t, body, "User denied access")

	// The error parameters are still forwarded so the flow can surface them
	params := <-callbackChan
	assert.Equal(t, "access_denied", params["error"])
	assert.Equal(t, "User denied access", params["error_description"])
}

func TestOAuthCallbackHandler_ErrorEscapesHTML(t *testing.T) {
	callbackChan := make(chan map[string]string, 1)
	handler := oauthCallbackHandler(callbackChan)

	req := httptest.NewRequest(http.MethodGet,
		"/oauth/callback?error=%3Cscript%3Ealert(1)%3C%2Fscript%3E", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "<script>alert(1)</script>")
	assert.Contains(t, body, "&lt;script&gt;")
}

func TestStartCallbackServer_MultipleFlows(t *testing.T) {
	// Previously the callback route was registered on the package-global
	// http.DefaultServeMux, so a second OAuth flow in the same process
	// panicked with a duplicate registration. Each server now gets its own mux.
	config := &ServerConfig{Type: "http", Name: "test-server", URL: "https://example.com"}
	client, err := NewClient(config)
	assert.NoError(t, err)

	server1 := client.startCallbackServer(make(chan map[string]string, 1))
	assert.NoError(t, server1.Shutdown(context.Background()))

	server2 := client.startCallbackServer(make(chan map[string]string, 1))
	assert.NoError(t, server2.Shutdown(context.Background()))
}
