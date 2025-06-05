package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client wraps the mcp-go client library with OAuth support
type Client struct {
	client             *client.Client
	config             *ServerConfig
	oauthConfig        *OAuthConfig
	tokenStore         client.TokenStore
	tools              []mcp.Tool
	resources          []mcp.Resource
	serverCapabilities *mcp.ServerCapabilities
	connected          bool
}

// NewClient creates a new MCP client instance with OAuth support
func NewClient(cfg *ServerConfig) (*Client, error) {
	client := &Client{
		config:    cfg,
		connected: false,
	}
	if cfg.IsOAuthEnabled() {
		// Set up OAuth configuration with defaults
		oauthConfig := &OAuthConfig{
			ClientID:    "dive",
			RedirectURI: "http://localhost:8085/oauth/callback",
			PKCEEnabled: true,
			Scopes:      []string{"mcp.read", "mcp.write"},
		}
		// Override with provided values if they exist
		if cfg.OAuth.ClientSecret != "" {
			oauthConfig.ClientSecret = cfg.OAuth.ClientSecret
		}
		if cfg.OAuth.RedirectURI != "" {
			oauthConfig.RedirectURI = cfg.OAuth.RedirectURI
		}
		if len(cfg.OAuth.Scopes) > 0 {
			oauthConfig.Scopes = cfg.OAuth.Scopes
		}
		if cfg.OAuth.ExtraParams != nil {
			oauthConfig.ExtraParams = cfg.OAuth.ExtraParams
		}
		if cfg.OAuth.TokenStore != nil {
			oauthConfig.TokenStore = cfg.OAuth.TokenStore
		}
		// PKCEEnabled defaults to true, only override if explicitly set to false
		oauthConfig.PKCEEnabled = cfg.OAuth.PKCEEnabled || oauthConfig.PKCEEnabled

		client.oauthConfig = oauthConfig
	}
	return client, nil
}

// Connect establishes connection to the MCP server with OAuth support
func (c *Client) Connect(ctx context.Context) error {
	// For OAuth-enabled HTTP clients, create the client first
	if c.config.Type == "http" && c.config.IsOAuthEnabled() {
		if err := c.connectWithOAuth(); err != nil {
			return fmt.Errorf("failed to create oauth mcp client for server %s: %w", c.config.Name, err)
		}
	} else {
		// Non-OAuth client creation
		var err error
		switch c.config.Type {
		case "http":
			if c.config.URL == "" {
				return fmt.Errorf("url is required for http mcp server")
			}
			c.client, err = client.NewStreamableHttpClient(c.config.URL)
		case "stdio":
			if c.config.Command == "" {
				return fmt.Errorf("command is required for stdio mcp server")
			}
			// For stdio, URL contains the command to execute
			envMap := c.config.Env
			args := c.config.Args

			// Perform environment variable substitution on args
			expandedArgs := make([]string, len(args))
			for i, arg := range args {
				expandedArgs[i] = os.ExpandEnv(arg)
			}

			// Convert environment map to slice of "KEY=VALUE" strings with substitution
			env := make([]string, 0, len(envMap))
			for key, value := range envMap {
				expandedValue := os.ExpandEnv(value)
				env = append(env, fmt.Sprintf("%s=%s", key, expandedValue))
			}
			c.client, err = client.NewStdioMCPClient(c.config.Command, env, expandedArgs...)
		default:
			return fmt.Errorf("unsupported mcp server type: %s", c.config.Type)
		}
		if err != nil {
			return fmt.Errorf("failed to create mcp client for server %s: %w", c.config.Name, err)
		}
	}

	// Start the client (OAuth flow may happen here)
	if err := c.client.Start(ctx); err != nil {
		// Check if this is an OAuth authorization error
		if c.config.IsOAuthEnabled() && c.isOAuthAuthorizationError(err) {
			if authErr := c.handleOAuthAuthorization(ctx, err); authErr != nil {
				return fmt.Errorf("OAuth authorization failed for server %s: %w", c.config.Name, authErr)
			}
			// Retry start after OAuth flow with the same client instance
			if err := c.client.Start(ctx); err != nil {
				return fmt.Errorf("failed to start mcp client for server %s after OAuth: %w", c.config.Name, err)
			}
		} else {
			return fmt.Errorf("failed to start mcp client for server %s: %w", c.config.Name, err)
		}
	}
	if err := c.initializeConnection(ctx); err != nil {
		return err
	}
	c.connected = true
	return nil
}

// connectWithOAuth creates an OAuth-enabled HTTP client
func (c *Client) connectWithOAuth() error {
	if c.oauthConfig == nil {
		return fmt.Errorf("OAuth configuration is nil")
	}
	if c.tokenStore == nil {
		c.tokenStore = client.NewMemoryTokenStore()
	}
	var err error
	c.client, err = client.NewOAuthStreamableHttpClient(c.config.URL, client.OAuthConfig{
		ClientID:     c.oauthConfig.ClientID,
		ClientSecret: c.oauthConfig.ClientSecret,
		RedirectURI:  c.oauthConfig.RedirectURI,
		Scopes:       c.oauthConfig.Scopes,
		TokenStore:   c.tokenStore,
		PKCEEnabled:  c.oauthConfig.PKCEEnabled,
	})
	return err
}

// isOAuthAuthorizationError checks if an error indicates OAuth authorization is required
func (c *Client) isOAuthAuthorizationError(err error) bool {
	if c.client == nil {
		return false
	}
	// Only use the official method to check for OAuth errors
	// Don't use string matching as it can cause false positives
	isOAuthError := client.IsOAuthAuthorizationRequiredError(err)
	return isOAuthError
}

// handleOAuthAuthorization handles the OAuth authorization flow
func (c *Client) handleOAuthAuthorization(ctx context.Context, err error) error {
	// Get the OAuth handler from the error
	oauthHandler := client.GetOAuthHandler(err)
	if oauthHandler == nil {
		return fmt.Errorf("oauth handler unavailable")
	}

	// Start a local server to handle the OAuth callback
	callbackChan := make(chan map[string]string, 1)
	server := c.startCallbackServer(callbackChan)
	defer func() {
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			log.Printf("Error shutting down callback server: %v", shutdownErr)
		}
	}()

	// Generate PKCE code verifier and challenge if enabled
	var codeVerifier, codeChallenge string
	var state string
	var genErr error

	if c.oauthConfig.PKCEEnabled {
		codeVerifier, genErr = client.GenerateCodeVerifier()
		if genErr != nil {
			return fmt.Errorf("failed to generate code verifier: %w", genErr)
		}
		codeChallenge = client.GenerateCodeChallenge(codeVerifier)
	}
	state, genErr = client.GenerateState()
	if genErr != nil {
		return fmt.Errorf("failed to generate state: %w", genErr)
	}
	if err := oauthHandler.RegisterClient(ctx, fmt.Sprintf("dive-%s", c.config.Name)); err != nil {
		return fmt.Errorf("failed to register OAuth client: %w", err)
	}
	authURL, err := oauthHandler.GetAuthorizationURL(ctx, state, codeChallenge)
	if err != nil {
		return fmt.Errorf("failed to get authorization URL: %w", err)
	}

	if err := c.openBrowser(authURL); err != nil {
		log.Printf("Failed to open browser automatically: %v", err)
		log.Printf("Please open the following URL in your browser: %s", authURL)
	}

	// Wait for authorization callback
	params := <-callbackChan

	if params["state"] != state {
		return fmt.Errorf("state mismatch: expected %s, got %s", state, params["state"])
	}

	code := params["code"]
	if code == "" {
		return fmt.Errorf("no authorization code received")
	}
	if err := oauthHandler.ProcessAuthorizationResponse(ctx, code, state, codeVerifier); err != nil {
		return fmt.Errorf("failed to process authorization response: %w", err)
	}
	return nil
}

// startCallbackServer starts a local HTTP server to handle the OAuth callback
func (c *Client) startCallbackServer(callbackChan chan<- map[string]string) *http.Server {
	server := &http.Server{
		Addr: ":8085", // Use fixed port 8085
	}

	http.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		// Extract query parameters
		params := make(map[string]string)
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				params[key] = values[0]
			}
		}

		// Send parameters to the channel
		select {
		case callbackChan <- params:
		default:
			// Channel is full, ignore
		}

		// Respond to the user
		w.Header().Set("Content-Type", "text/html")
		_, err := w.Write([]byte(`
			<html>
				<body>
					<h1>Authorization Successful</h1>
					<p>You can now close this window and return to the application.</p>
					<script>window.close();</script>
				</body>
			</html>
		`))
		if err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return server
}

// openBrowser opens the default browser to the specified URL
func (c *Client) openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return exec.Command(cmd, args...).Start()
}

// initializeConnection initializes the MCP connection
func (c *Client) initializeConnection(ctx context.Context) error {
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initResponse, err := c.client.Initialize(initCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "dive",
				Version: "1.0.0",
			},
		},
	})
	if err != nil {
		// Check if this is an OAuth authorization error during initialization
		if c.config.IsOAuthEnabled() && c.isOAuthAuthorizationError(err) {
			if authErr := c.handleOAuthAuthorization(ctx, err); authErr != nil {
				return NewMCPError(
					"initialize",
					c.config.Name,
					fmt.Errorf("OAuth authorization failed: %w", authErr),
				)
			}
			// Retry initialization after OAuth flow - no need to restart client
			initResponse, err = c.client.Initialize(initCtx, mcp.InitializeRequest{
				Params: mcp.InitializeParams{
					ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
					ClientInfo: mcp.Implementation{
						Name:    "dive",
						Version: "1.0.0",
					},
				},
			})
			if err != nil {
				if initCtx.Err() == context.DeadlineExceeded {
					return NewMCPError(
						"initialize",
						c.config.Name,
						fmt.Errorf("initialization timeout after 30s: %w", ErrInitializationFailed),
					)
				}
				return NewMCPError(
					"initialize",
					c.config.Name,
					fmt.Errorf("%w: %v", ErrInitializationFailed, err),
				)
			}
		} else {
			// Non-OAuth error or OAuth not enabled
			if initCtx.Err() == context.DeadlineExceeded {
				return NewMCPError(
					"initialize",
					c.config.Name,
					fmt.Errorf("initialization timeout after 30s: %w", ErrInitializationFailed),
				)
			}
			return NewMCPError(
				"initialize",
				c.config.Name,
				fmt.Errorf("%w: %v", ErrInitializationFailed, err),
			)
		}
	}

	c.serverCapabilities = &initResponse.Capabilities
	return nil
}

// ListTools retrieves available tools from the MCP server. Filters the tools
// based on the server's ToolConfiguration.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if !c.connected {
		return nil, NewMCPError("list_tools", c.config.Name, ErrNotConnected)
	}
	response, err := c.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, NewMCPError("list_tools", c.config.Name, err)
	}
	tools := c.filterTools(response.Tools)
	c.tools = tools
	return tools, nil
}

// CallTool executes a tool on the MCP server
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if !c.connected {
		return nil, NewMCPError("call_tool", c.config.Name, ErrNotConnected)
	}
	response, err := c.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})
	if err != nil {
		return nil, NewMCPError("call_tool", c.config.Name, err)
	}
	return response, nil
}

// ListResources retrieves available resources from the MCP server
func (c *Client) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	if !c.connected {
		return nil, NewMCPError("list_resources", c.config.Name, ErrNotConnected)
	}
	if c.serverCapabilities == nil || c.serverCapabilities.Resources == nil {
		return nil, NewMCPError("list_resources", c.config.Name, ErrUnsupportedOperation)
	}
	response, err := c.client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, NewMCPError("list_resources", c.config.Name, err)
	}
	c.resources = response.Resources
	return response.Resources, nil
}

// ReadResource reads a specific resource from the MCP server
func (c *Client) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	if !c.connected {
		return nil, NewMCPError("read_resource", c.config.Name, ErrNotConnected)
	}
	if c.serverCapabilities == nil || c.serverCapabilities.Resources == nil {
		return nil, NewMCPError("read_resource", c.config.Name, ErrUnsupportedOperation)
	}
	response, err := c.client.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		return nil, NewMCPError("read_resource", c.config.Name, err)
	}
	return response, nil
}

// GetResources returns the cached list of resources
func (c *Client) GetResources() []mcp.Resource {
	return c.resources
}

// GetServerCapabilities returns the server capabilities
func (c *Client) GetServerCapabilities() *mcp.ServerCapabilities {
	return c.serverCapabilities
}

// GetTools returns the cached list of tools
func (c *Client) GetTools() []mcp.Tool {
	return c.tools
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	return c.connected
}

// Close closes the MCP client connection
func (c *Client) Close() error {
	if c.client != nil && c.connected {
		c.connected = false
		// Note: The mcp-go client doesn't seem to have a Close method
		// This might need to be updated based on the actual API
	}
	return nil
}

// filterTools filters tools based on the server's ToolConfiguration
func (c *Client) filterTools(tools []mcp.Tool) []mcp.Tool {
	// If tools are explicitly disabled, return no tools
	if !c.config.IsToolEnabled() {
		return []mcp.Tool{}
	}

	// If no allowed tools specified, return all tools
	allowedTools := c.config.GetAllowedTools()
	if len(allowedTools) == 0 {
		return tools
	}

	// Filter tools based on AllowedTools list
	allowedMap := make(map[string]bool)
	for _, toolName := range allowedTools {
		allowedMap[toolName] = true
	}

	var filteredTools []mcp.Tool
	for _, tool := range tools {
		if allowedMap[tool.Name] {
			filteredTools = append(filteredTools, tool)
		}
	}
	return filteredTools
}
