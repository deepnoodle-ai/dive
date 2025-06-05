package config

import "github.com/diveagents/dive/llm"

type MCPToolConfiguration struct {
	Enabled      *bool    `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
	AllowedTools []string `yaml:"AllowedTools" json:"AllowedTools"`
}

type MCPServer struct {
	Type               string                `yaml:"Type" json:"Type"`
	Name               string                `yaml:"Name" json:"Name"`
	URL                string                `yaml:"URL,omitempty" json:"URL,omitempty"`
	AuthorizationToken string                `yaml:"AuthorizationToken,omitempty" json:"AuthorizationToken,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `yaml:"ToolConfiguration,omitempty" json:"ToolConfiguration,omitempty"`
}

// Provider is used to configure an LLM provider
type Provider struct {
	Name           string            `yaml:"Name" json:"Name"`
	Caching        *bool             `yaml:"Caching,omitempty" json:"Caching,omitempty"`
	Features       []string          `yaml:"Features,omitempty" json:"Features,omitempty"`
	RequestHeaders map[string]string `yaml:"RequestHeaders,omitempty" json:"RequestHeaders,omitempty"`
	MCPServers     []MCPServer       `yaml:"MCPServers,omitempty" json:"MCPServers,omitempty"`
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
	MCPServers        []MCPServer         `yaml:"MCPServers,omitempty" json:"MCPServers,omitempty"`
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

func isValidLogLevel(level string) bool {
	return level == "debug" || level == "info" || level == "warn" || level == "error"
}

func boolPtr(b bool) *bool {
	return &b
}
