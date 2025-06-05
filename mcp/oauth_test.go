package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewMCPClient_WithoutOAuth(t *testing.T) {
	config := &ServerConfig{
		Type:  "http",
		Name:  "test-server",
		URL:   "https://example.com",
		OAuth: nil,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Nil(t, client.oauthConfig)
	require.Equal(t, config, client.config)
	require.False(t, client.connected)
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
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.oauthConfig)
	require.Equal(t, "dive", client.oauthConfig.ClientID)
	require.Equal(t, "http://localhost:8085/oauth/callback", client.oauthConfig.RedirectURI)
	require.True(t, client.oauthConfig.PKCEEnabled)
	require.Equal(t, []string{"mcp.read", "mcp.write"}, client.oauthConfig.Scopes)
}

func TestMCPClient_OAuth_IsConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	require.False(t, client.IsConnected())

	// Simulate connection
	client.connected = true
	require.True(t, client.IsConnected())
}

func TestMCPClient_OAuth_Close(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	client.connected = true
	err = client.Close()
	require.NoError(t, err)
}

func TestMCPClient_OAuth_ListTools_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	require.Error(t, err)
	require.Nil(t, tools)
	require.Contains(t, err.Error(), "not connected")
}

func TestMCPClient_OAuth_ListResources_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	resources, err := client.ListResources(ctx)
	require.Error(t, err)
	require.Nil(t, resources)
	require.Contains(t, err.Error(), "not connected")
}

func TestMCPClient_OAuth_CallTool_NotConnected(t *testing.T) {
	config := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "https://example.com",
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := client.CallTool(ctx, "test-tool", map[string]interface{}{})
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "not connected")
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
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)

			if tt.expectOAuth {
				require.NotNil(t, client.oauthConfig)
			} else {
				require.Nil(t, client.oauthConfig)
			}
		})
	}
}
