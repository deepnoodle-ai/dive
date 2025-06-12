package config

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/mcp"
)

// MCPToolApprovalFilter is used to configure the approval filter for MCP tools.
type MCPToolApprovalFilter struct {
	Always []string `yaml:"Always,omitempty" json:"Always,omitempty"`
	Never  []string `yaml:"Never,omitempty" json:"Never,omitempty"`
}

// MCPToolConfiguration customizes tool behavior for MCP servers.
type MCPToolConfiguration struct {
	Enabled        *bool                  `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
	AllowedTools   []string               `yaml:"AllowedTools" json:"AllowedTools"`
	ApprovalMode   string                 `yaml:"ApprovalMode,omitempty" json:"ApprovalMode,omitempty"`
	ApprovalFilter *MCPToolApprovalFilter `yaml:"ApprovalFilter,omitempty" json:"ApprovalFilter,omitempty"`
}

// MCPOAuthConfig represents OAuth 2.0 configuration for MCP servers
type MCPOAuthConfig struct {
	ClientID     string            `yaml:"ClientID" json:"ClientID"`
	ClientSecret string            `yaml:"ClientSecret,omitempty" json:"ClientSecret,omitempty"`
	RedirectURI  string            `yaml:"RedirectURI" json:"RedirectURI"`
	Scopes       []string          `yaml:"Scopes,omitempty" json:"Scopes,omitempty"`
	PKCEEnabled  *bool             `yaml:"PKCEEnabled,omitempty" json:"PKCEEnabled,omitempty"`
	TokenStore   *MCPTokenStore    `yaml:"TokenStore,omitempty" json:"TokenStore,omitempty"`
	ExtraParams  map[string]string `yaml:"ExtraParams,omitempty" json:"ExtraParams,omitempty"`
}

// MCPTokenStore represents token storage configuration
type MCPTokenStore struct {
	Type string `yaml:"Type" json:"Type"`                     // "memory", "file", "keychain"
	Path string `yaml:"Path,omitempty" json:"Path,omitempty"` // For file storage
}

// MCPServer represents a server that can be used to provide tools to agents.
type MCPServer struct {
	Type               string                `yaml:"Type" json:"Type"`
	Name               string                `yaml:"Name" json:"Name"`
	Command            string                `yaml:"Command,omitempty" json:"Command,omitempty"`
	URL                string                `yaml:"URL,omitempty" json:"URL,omitempty"`
	Env                map[string]string     `yaml:"Env,omitempty" json:"Env,omitempty"`
	Args               []string              `yaml:"Args,omitempty" json:"Args,omitempty"`
	AuthorizationToken string                `yaml:"AuthorizationToken,omitempty" json:"AuthorizationToken,omitempty"`
	OAuth              *MCPOAuthConfig       `yaml:"OAuth,omitempty" json:"OAuth,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `yaml:"ToolConfiguration,omitempty" json:"ToolConfiguration,omitempty"`
	Headers            map[string]string     `yaml:"Headers,omitempty" json:"Headers,omitempty"`
}

// Provider is used to configure an LLM provider
type Provider struct {
	Name           string            `yaml:"Name" json:"Name"`
	Caching        *bool             `yaml:"Caching,omitempty" json:"Caching,omitempty"`
	Features       []string          `yaml:"Features,omitempty" json:"Features,omitempty"`
	RequestHeaders map[string]string `yaml:"RequestHeaders,omitempty" json:"RequestHeaders,omitempty"`
}

// Config represents global configuration settings
type Config struct {
	DefaultProvider  string     `yaml:"DefaultProvider,omitempty" json:"DefaultProvider,omitempty"`
	DefaultModel     string     `yaml:"DefaultModel,omitempty" json:"DefaultModel,omitempty"`
	DefaultWorkflow  string     `yaml:"DefaultWorkflow,omitempty" json:"DefaultWorkflow,omitempty"`
	ConfirmationMode string     `yaml:"ConfirmationMode,omitempty" json:"ConfirmationMode,omitempty"`
	LogLevel         string     `yaml:"LogLevel,omitempty" json:"LogLevel,omitempty"`
	Providers        []Provider `yaml:"Providers,omitempty" json:"Providers,omitempty"`
}

// Variable represents a workflow-level input parameter
type Variable struct {
	Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
	Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
	Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
	Default     string `yaml:"Default,omitempty" json:"Default,omitempty"`
}

// Tool represents an external capability that can be used by agents
type Tool struct {
	Name       string         `yaml:"Name,omitempty" json:"Name,omitempty"`
	Enabled    *bool          `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
	Parameters map[string]any `yaml:"Parameters,omitempty" json:"Parameters,omitempty"`
}

// Content carries or points to a piece of content that can be used as context.
type Content struct {
	Text        string `yaml:"Text,omitempty" json:"Text,omitempty"`
	Path        string `yaml:"Path,omitempty" json:"Path,omitempty"`
	URL         string `yaml:"URL,omitempty" json:"URL,omitempty"`
	Document    string `yaml:"Document,omitempty" json:"Document,omitempty"`
	Dynamic     string `yaml:"Dynamic,omitempty" json:"Dynamic,omitempty"`
	DynamicFrom string `yaml:"DynamicFrom,omitempty" json:"DynamicFrom,omitempty"`
}

// Agent is a serializable representation of an Agent
type Agent struct {
	Name               string         `yaml:"Name,omitempty" json:"Name,omitempty"`
	Goal               string         `yaml:"Goal,omitempty" json:"Goal,omitempty"`
	Instructions       string         `yaml:"Instructions,omitempty" json:"Instructions,omitempty"`
	IsSupervisor       bool           `yaml:"IsSupervisor,omitempty" json:"IsSupervisor,omitempty"`
	Subordinates       []string       `yaml:"Subordinates,omitempty" json:"Subordinates,omitempty"`
	Provider           string         `yaml:"Provider,omitempty" json:"Provider,omitempty"`
	Model              string         `yaml:"Model,omitempty" json:"Model,omitempty"`
	Tools              []string       `yaml:"Tools,omitempty" json:"Tools,omitempty"`
	ResponseTimeout    any            `yaml:"ResponseTimeout,omitempty" json:"ResponseTimeout,omitempty"`
	ToolConfig         map[string]any `yaml:"ToolConfig,omitempty" json:"ToolConfig,omitempty"`
	ToolIterationLimit int            `yaml:"ToolIterationLimit,omitempty" json:"ToolIterationLimit,omitempty"`
	DateAwareness      *bool          `yaml:"DateAwareness,omitempty" json:"DateAwareness,omitempty"`
	SystemPrompt       string         `yaml:"SystemPrompt,omitempty" json:"SystemPrompt,omitempty"`
	ModelSettings      *ModelSettings `yaml:"ModelSettings,omitempty" json:"ModelSettings,omitempty"`
	Context            []Content      `yaml:"Context,omitempty" json:"Context,omitempty"`
}

// ModelSettings is used to configure an Agent LLM
type ModelSettings struct {
	Temperature       *float64            `yaml:"Temperature,omitempty" json:"Temperature,omitempty"`
	PresencePenalty   *float64            `yaml:"PresencePenalty,omitempty" json:"PresencePenalty,omitempty"`
	FrequencyPenalty  *float64            `yaml:"FrequencyPenalty,omitempty" json:"FrequencyPenalty,omitempty"`
	ReasoningBudget   *int                `yaml:"ReasoningBudget,omitempty" json:"ReasoningBudget,omitempty"`
	ReasoningEffort   llm.ReasoningEffort `yaml:"ReasoningEffort,omitempty" json:"ReasoningEffort,omitempty"`
	MaxTokens         *int                `yaml:"MaxTokens,omitempty" json:"MaxTokens,omitempty"`
	ToolChoice        *llm.ToolChoice     `yaml:"ToolChoice,omitempty" json:"ToolChoice,omitempty"`
	ParallelToolCalls *bool               `yaml:"ParallelToolCalls,omitempty" json:"ParallelToolCalls,omitempty"`
	Features          []string            `yaml:"Features,omitempty" json:"Features,omitempty"`
	RequestHeaders    map[string]string   `yaml:"RequestHeaders,omitempty" json:"RequestHeaders,omitempty"`
	Caching           *bool               `yaml:"Caching,omitempty" json:"Caching,omitempty"`
}

// Input represents an input parameter for a task or workflow
type Input struct {
	Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
	Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
	Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
	Required    bool   `yaml:"Required,omitempty" json:"Required,omitempty"`
	Default     any    `yaml:"Default,omitempty" json:"Default,omitempty"`
	As          string `yaml:"As,omitempty" json:"As,omitempty"`
}

// Output represents an output parameter for a task or workflow
type Output struct {
	Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
	Type        string `yaml:"Type,omitempty" json:"Type,omitempty"`
	Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
	Format      string `yaml:"Format,omitempty" json:"Format,omitempty"`
	Default     any    `yaml:"Default,omitempty" json:"Default,omitempty"`
	Document    string `yaml:"Document,omitempty" json:"Document,omitempty"`
}

// Step represents a single step in a workflow
type Step struct {
	Type       string         `yaml:"Type,omitempty" json:"Type,omitempty"`
	Name       string         `yaml:"Name,omitempty" json:"Name,omitempty"`
	Agent      string         `yaml:"Agent,omitempty" json:"Agent,omitempty"`
	Prompt     string         `yaml:"Prompt,omitempty" json:"Prompt,omitempty"`
	Store      string         `yaml:"Store,omitempty" json:"Store,omitempty"`
	Action     string         `yaml:"Action,omitempty" json:"Action,omitempty"`
	Parameters map[string]any `yaml:"Parameters,omitempty" json:"Parameters,omitempty"`
	Each       *EachBlock     `yaml:"Each,omitempty" json:"Each,omitempty"`
	Next       []NextStep     `yaml:"Next,omitempty" json:"Next,omitempty"`
	Seconds    float64        `yaml:"Seconds,omitempty" json:"Seconds,omitempty"`
	End        bool           `yaml:"End,omitempty" json:"End,omitempty"`
	Content    []Content      `yaml:"Content,omitempty" json:"Content,omitempty"`
}

// EachBlock represents iteration configuration for a step
type EachBlock struct {
	Items any    `yaml:"Items,omitempty" json:"Items,omitempty"`
	As    string `yaml:"As,omitempty" json:"As,omitempty"`
}

// NextStep represents the next step in a workflow with optional conditions
type NextStep struct {
	Step      string `yaml:"Step,omitempty" json:"Step,omitempty"`
	Condition string `yaml:"Condition,omitempty" json:"Condition,omitempty"`
}

// Workflow represents a workflow definition
type Workflow struct {
	Name        string    `yaml:"Name,omitempty" json:"Name,omitempty"`
	Description string    `yaml:"Description,omitempty" json:"Description,omitempty"`
	Inputs      []Input   `yaml:"Inputs,omitempty" json:"Inputs,omitempty"`
	Output      *Output   `yaml:"Output,omitempty" json:"Output,omitempty"`
	Triggers    []Trigger `yaml:"Triggers,omitempty" json:"Triggers,omitempty"`
	Steps       []Step    `yaml:"Steps,omitempty" json:"Steps,omitempty"`
}

// Trigger represents a trigger definition
type Trigger struct {
	Name   string                 `yaml:"Name,omitempty" json:"Name,omitempty"`
	Type   string                 `yaml:"Type,omitempty" json:"Type,omitempty"`
	Config map[string]interface{} `yaml:"Config,omitempty" json:"Config,omitempty"`
}

// Schedule represents a schedule definition
type Schedule struct {
	Name     string `yaml:"Name,omitempty" json:"Name,omitempty"`
	Cron     string `yaml:"Cron,omitempty" json:"Cron,omitempty"`
	Workflow string `yaml:"Workflow,omitempty" json:"Workflow,omitempty"`
	Enabled  *bool  `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
}

// Document represents a document that can be referenced by agents and tasks
type Document struct {
	ID          string `yaml:"ID,omitempty" json:"ID,omitempty"`
	Name        string `yaml:"Name,omitempty" json:"Name,omitempty"`
	Description string `yaml:"Description,omitempty" json:"Description,omitempty"`
	Path        string `yaml:"Path,omitempty" json:"Path,omitempty"`
	Content     string `yaml:"Content,omitempty" json:"Content,omitempty"`
	ContentType string `yaml:"ContentType,omitempty" json:"ContentType,omitempty"`
}

// ToLLMConfig converts config.MCPServer to llm.MCPServerConfig
func (s MCPServer) ToLLMConfig() *llm.MCPServerConfig {
	config := &llm.MCPServerConfig{
		Type:               s.Type,
		Command:            s.Command,
		URL:                s.URL,
		Name:               s.Name,
		AuthorizationToken: s.AuthorizationToken,
		Headers:            s.Headers,
	}
	if s.OAuth != nil {
		config.OAuth = &llm.MCPOAuthConfig{
			ClientID:     s.OAuth.ClientID,
			ClientSecret: s.OAuth.ClientSecret,
			RedirectURI:  s.OAuth.RedirectURI,
			Scopes:       s.OAuth.Scopes,
			PKCEEnabled:  s.OAuth.PKCEEnabled != nil && *s.OAuth.PKCEEnabled,
			ExtraParams:  s.OAuth.ExtraParams,
		}
		if s.OAuth.TokenStore != nil {
			config.OAuth.TokenStore = &llm.MCPTokenStore{
				Type: s.OAuth.TokenStore.Type,
				Path: s.OAuth.TokenStore.Path,
			}
		}
	}
	if s.ToolConfiguration != nil {
		config.ToolConfiguration = &llm.MCPToolConfiguration{
			Enabled:      s.ToolConfiguration.Enabled == nil || *s.ToolConfiguration.Enabled,
			AllowedTools: s.ToolConfiguration.AllowedTools,
		}
		if s.ToolConfiguration.ApprovalFilter != nil {
			config.ToolConfiguration.ApprovalFilter = &llm.MCPToolApprovalFilter{
				Always: s.ToolConfiguration.ApprovalFilter.Always,
				Never:  s.ToolConfiguration.ApprovalFilter.Never,
			}
		}
		config.ToolConfiguration.ApprovalMode = s.ToolConfiguration.ApprovalMode
	}
	return config
}

// ToMCPConfig converts config.MCPServer to mcp.ServerConfig
func (s MCPServer) ToMCPConfig() *mcp.ServerConfig {
	config := &mcp.ServerConfig{
		Type:               s.Type,
		Command:            s.Command,
		URL:                s.URL,
		Name:               s.Name,
		Env:                s.Env,
		Args:               s.Args,
		AuthorizationToken: s.AuthorizationToken,
		Headers:            s.Headers,
	}
	if s.OAuth != nil {
		config.OAuth = &mcp.OAuthConfig{
			ClientID:     s.OAuth.ClientID,
			ClientSecret: s.OAuth.ClientSecret,
			RedirectURI:  s.OAuth.RedirectURI,
			Scopes:       s.OAuth.Scopes,
			PKCEEnabled:  s.OAuth.PKCEEnabled != nil && *s.OAuth.PKCEEnabled,
			ExtraParams:  s.OAuth.ExtraParams,
		}
		if s.OAuth.TokenStore != nil {
			config.OAuth.TokenStore = &mcp.TokenStore{
				Type: s.OAuth.TokenStore.Type,
				Path: s.OAuth.TokenStore.Path,
			}
		}
	}
	if s.ToolConfiguration != nil {
		config.ToolConfiguration = &mcp.ToolConfiguration{
			Enabled:      s.ToolConfiguration.Enabled == nil || *s.ToolConfiguration.Enabled,
			AllowedTools: s.ToolConfiguration.AllowedTools,
		}
		if s.ToolConfiguration.ApprovalFilter != nil {
			config.ToolConfiguration.ApprovalFilter = &mcp.ToolApprovalFilter{
				Always: s.ToolConfiguration.ApprovalFilter.Always,
				Never:  s.ToolConfiguration.ApprovalFilter.Never,
			}
		}
		config.ToolConfiguration.ApprovalMode = s.ToolConfiguration.ApprovalMode
	}
	return config
}

// IsOAuthEnabled returns true if OAuth is configured for this server
func (s MCPServer) IsOAuthEnabled() bool {
	return s.OAuth != nil
}

func isValidLogLevel(level string) bool {
	return level == "debug" || level == "info" || level == "warn" || level == "error"
}

func boolPtr(b bool) *bool {
	return &b
}
