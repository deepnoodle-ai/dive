package config

// LLMConfig is used to configure LLM related settings
type LLMConfig struct {
	CacheControl    string `yaml:"CacheControl,omitempty" json:"CacheControl,omitempty"`
	DefaultProvider string `yaml:"DefaultProvider,omitempty" json:"DefaultProvider,omitempty"`
	DefaultModel    string `yaml:"DefaultModel,omitempty" json:"DefaultModel,omitempty"`
}

// LoggingConfig is used to configure logging related settings
type LoggingConfig struct {
	Level string `yaml:"Level,omitempty" json:"Level,omitempty"`
}

// WorkflowConfig is used to configure Workflow related settings
type WorkflowConfig struct {
	DefaultWorkflow string `yaml:"DefaultWorkflow,omitempty" json:"DefaultWorkflow,omitempty"`
}

// DocumentsConfig is used to configure Documents related settings
type DocumentsConfig struct {
	Root string `yaml:"Root,omitempty" json:"Root,omitempty"`
}

// Config represents global configuration settings
type Config struct {
	LLM       LLMConfig       `yaml:"LLM,omitempty" json:"LLM,omitempty"`
	Logging   LoggingConfig   `yaml:"Logging,omitempty" json:"Logging,omitempty"`
	Workflows WorkflowConfig  `yaml:"Workflows,omitempty" json:"Workflows,omitempty"`
	Documents DocumentsConfig `yaml:"Documents,omitempty" json:"Documents,omitempty"`
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
	Enabled    bool           `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
	Parameters map[string]any `yaml:"Parameters,omitempty" json:"Parameters,omitempty"`
}

// Agent is a serializable representation of an Agent
type Agent struct {
	Name               string         `yaml:"Name,omitempty" json:"Name,omitempty"`
	Goal               string         `yaml:"Goal,omitempty" json:"Goal,omitempty"`
	Backstory          string         `yaml:"Backstory,omitempty" json:"Backstory,omitempty"`
	Provider           string         `yaml:"Provider,omitempty" json:"Provider,omitempty"`
	Model              string         `yaml:"Model,omitempty" json:"Model,omitempty"`
	CacheControl       string         `yaml:"CacheControl,omitempty" json:"CacheControl,omitempty"`
	Tools              []string       `yaml:"Tools,omitempty" json:"Tools,omitempty"`
	IsSupervisor       bool           `yaml:"IsSupervisor,omitempty" json:"IsSupervisor,omitempty"`
	Subordinates       []string       `yaml:"Subordinates,omitempty" json:"Subordinates,omitempty"`
	TaskTimeout        string         `yaml:"TaskTimeout,omitempty" json:"TaskTimeout,omitempty"`
	ChatTimeout        string         `yaml:"ChatTimeout,omitempty" json:"ChatTimeout,omitempty"`
	LogLevel           string         `yaml:"LogLevel,omitempty" json:"LogLevel,omitempty"`
	ToolConfig         map[string]any `yaml:"ToolConfig,omitempty" json:"ToolConfig,omitempty"`
	ToolIterationLimit int            `yaml:"ToolIterationLimit,omitempty" json:"ToolIterationLimit,omitempty"`
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

// Prompt is a serializable representation of a prompt
type Prompt struct {
	Name         string   `yaml:"Name,omitempty" json:"Name,omitempty"`
	Text         string   `yaml:"Text,omitempty" json:"Text,omitempty"`
	Context      []string `yaml:"Context,omitempty" json:"Context,omitempty"`
	Output       string   `yaml:"Output,omitempty" json:"Output,omitempty"`
	OutputFormat string   `yaml:"OutputFormat,omitempty" json:"OutputFormat,omitempty"`
}

// Step represents a single step in a workflow
type Step struct {
	Type       string         `yaml:"Type,omitempty" json:"Type,omitempty"`
	Name       string         `yaml:"Name,omitempty" json:"Name,omitempty"`
	Agent      string         `yaml:"Agent,omitempty" json:"Agent,omitempty"`
	Prompt     interface{}    `yaml:"Prompt,omitempty" json:"Prompt,omitempty"`
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
	Enabled  bool   `yaml:"Enabled,omitempty" json:"Enabled,omitempty"`
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
