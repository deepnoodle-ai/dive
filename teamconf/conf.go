package teamconf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Team is a serializable representation of a dive.Team
type Team struct {
	Name        string           `yaml:"name,omitempty" json:"name,omitempty"`
	Description string           `yaml:"description,omitempty" json:"description,omitempty"`
	Agents      []Agent          `yaml:"agents,omitempty" json:"agents,omitempty"`
	Tasks       []Task           `yaml:"tasks,omitempty" json:"tasks,omitempty"`
	Tools       []ToolDefinition `yaml:"tools,omitempty" json:"tools,omitempty"`
	Config      Config           `yaml:"config,omitempty" json:"config,omitempty"`
	Variables   []Variable       `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// Config is a serializable high-level configuration for Dive
type Config struct {
	DefaultProvider string `yaml:"default_provider,omitempty" json:"default_provider,omitempty" hcl:"default_provider,optional"`
	DefaultModel    string `yaml:"default_model,omitempty" json:"default_model,omitempty" hcl:"default_model,optional"`
	LogLevel        string `yaml:"log_level,omitempty" json:"log_level,omitempty" hcl:"log_level,optional"`
	CacheControl    string `yaml:"cache_control,omitempty" json:"cache_control,omitempty" hcl:"cache_control,optional"`
}

// Variable is used to dynamically configure a dive.Team
type Variable struct {
	Name        string `yaml:"name,omitempty" json:"name,omitempty"`
	Type        string `yaml:"type,omitempty" json:"type,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string `yaml:"default,omitempty" json:"default,omitempty"`
}

// Agent is a serializable representation of a dive.Agent
type Agent struct {
	Name             string   `yaml:"name,omitempty" json:"name,omitempty" hcl:"name,label"`
	NameOverride     string   `yaml:"name_override,omitempty" json:"name_override,omitempty" hcl:"name,optional"`
	Description      string   `yaml:"description,omitempty" json:"description,omitempty" hcl:"description,optional"`
	Instructions     string   `yaml:"instructions,omitempty" json:"instructions,omitempty" hcl:"instructions,optional"`
	IsSupervisor     bool     `yaml:"is_supervisor,omitempty" json:"is_supervisor,omitempty" hcl:"is_supervisor,optional"`
	Subordinates     []string `yaml:"subordinates,omitempty" json:"subordinates,omitempty" hcl:"subordinates,optional"`
	AcceptedEvents   []string `yaml:"accepted_events,omitempty" json:"accepted_events,omitempty" hcl:"accepted_events,optional"`
	Provider         string   `yaml:"provider,omitempty" json:"provider,omitempty" hcl:"provider,optional"`
	Model            string   `yaml:"model,omitempty" json:"model,omitempty" hcl:"model,optional"`
	Tools            []string `yaml:"tools,omitempty" json:"tools,omitempty" hcl:"tools,optional"`
	CacheControl     string   `yaml:"cache_control,omitempty" json:"cache_control,omitempty" hcl:"cache_control,optional"`
	MaxActiveTasks   int      `yaml:"max_active_tasks,omitempty" json:"max_active_tasks,omitempty" hcl:"max_active_tasks,optional"`
	TaskTimeout      string   `yaml:"task_timeout,omitempty" json:"task_timeout,omitempty" hcl:"task_timeout,optional"`
	ChatTimeout      string   `yaml:"chat_timeout,omitempty" json:"chat_timeout,omitempty" hcl:"chat_timeout,optional"`
	GenerationLimit  int      `yaml:"generation_limit,omitempty" json:"generation_limit,omitempty" hcl:"generation_limit,optional"`
	TaskMessageLimit int      `yaml:"task_message_limit,omitempty" json:"task_message_limit,omitempty" hcl:"task_message_limit,optional"`
	LogLevel         string   `yaml:"log_level,omitempty" json:"log_level,omitempty" hcl:"log_level,optional"`
}

// Task is a serializable representation of a dive.Task
type Task struct {
	Name           string   `yaml:"name,omitempty" json:"name,omitempty" hcl:"name,label"`
	NameOverride   string   `yaml:"name_override,omitempty" json:"name_override,omitempty" hcl:"name,optional"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty" hcl:"description,optional"`
	ExpectedOutput string   `yaml:"expected_output,omitempty" json:"expected_output,omitempty" hcl:"expected_output,optional"`
	OutputFormat   string   `yaml:"output_format,omitempty" json:"output_format,omitempty" hcl:"output_format,optional"`
	AssignedAgent  string   `yaml:"assigned_agent,omitempty" json:"assigned_agent,omitempty" hcl:"assigned_agent,optional"`
	Dependencies   []string `yaml:"dependencies,omitempty" json:"dependencies,omitempty" hcl:"dependencies,optional"`
	OutputFile     string   `yaml:"output_file,omitempty" json:"output_file,omitempty" hcl:"output_file,optional"`
	Timeout        string   `yaml:"timeout,omitempty" json:"timeout,omitempty" hcl:"timeout,optional"`
	Context        string   `yaml:"context,omitempty" json:"context,omitempty" hcl:"context,optional"`
}

// ToolDefinition used for serializing tool configurations
type ToolDefinition map[string]interface{}

// LoadConfFile loads a Team configuration from a file. The file extension is
// used to determine the configuration format:
// - .json -> JSON
// - .yml or .yaml -> YAML
// - .hcl or .dive -> HCL
func LoadConfFile(filePath string) (*Team, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	filename := filepath.Base(filePath)

	var def Team

	switch filepath.Ext(filePath) {
	case ".json":
		return LoadConfJSON(data)
	case ".yml", ".yaml":
		return LoadConfYAML(data)
	case ".hcl", ".dive":
		return LoadConfHCL(data, filename)
	}

	return &def, nil
}

// LoadConfJSON loads a Team configuration from a JSON string
func LoadConfJSON(conf []byte) (*Team, error) {
	var def Team
	if err := json.Unmarshal([]byte(conf), &def); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return &def, nil
}

// LoadConfYAML loads a Team configuration from a YAML string
func LoadConfYAML(conf []byte) (*Team, error) {
	var def Team
	if err := yaml.Unmarshal([]byte(conf), &def); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return &def, nil
}

// LoadConfHCL loads a Team configuration from a HCL string
func LoadConfHCL(conf []byte, filename string) (*Team, error) {
	hclteam, err := LoadHCLDefinition(conf, filename, nil)
	if err != nil {
		return nil, err
	}
	// Convert HCLTeam to Team
	def := &Team{
		Name:        hclteam.Name,
		Description: hclteam.Description,
		Agents:      hclteam.Agents,
		Tasks:       hclteam.Tasks,
		Config:      hclteam.Config,
		// Variables:   hclteam.Variables,
		// Tools:       hclteam.Tools,
	}
	return def, nil
}
