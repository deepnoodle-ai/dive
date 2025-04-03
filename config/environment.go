package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/goccy/go-yaml"
)

// Environment is a serializable representation of an AI agent environment
type Environment struct {
	Name        string     `yaml:"Name,omitempty" json:"Name,omitempty"`
	Description string     `yaml:"Description,omitempty" json:"Description,omitempty"`
	Version     string     `yaml:"Version,omitempty" json:"Version,omitempty"`
	Config      Config     `yaml:"Config,omitempty" json:"Config,omitempty"`
	Tools       []Tool     `yaml:"Tools,omitempty" json:"Tools,omitempty"`
	Documents   []Document `yaml:"Documents,omitempty" json:"Documents,omitempty"`
	Agents      []Agent    `yaml:"Agents,omitempty" json:"Agents,omitempty"`
	Workflows   []Workflow `yaml:"Workflows,omitempty" json:"Workflows,omitempty"`
	Triggers    []Trigger  `yaml:"Triggers,omitempty" json:"Triggers,omitempty"`
	Schedules   []Schedule `yaml:"Schedules,omitempty" json:"Schedules,omitempty"`
}

// Save writes an Environment configuration to a file. The file extension is used to
// determine the configuration format:
// - .json -> JSON
// - .yml or .yaml -> YAML
func (env *Environment) Save(path string) error {
	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return env.SaveJSON(path)
	case ".yml", ".yaml":
		return env.SaveYAML(path)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// SaveYAML writes an Environment configuration to a YAML file
func (env *Environment) SaveYAML(path string) error {
	data, err := yaml.Marshal(env)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveJSON writes an Environment configuration to a JSON file
func (env *Environment) SaveJSON(path string) error {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Write an Environment configuration to a writer in YAML format
func (env *Environment) Write(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(env)
}

// Build creates a new Environment from the configuration
func (env *Environment) Build(opts ...BuildOption) (*environment.Environment, error) {
	buildOpts := &BuildOptions{}
	for _, opt := range opts {
		opt(buildOpts)
	}

	var logger slogger.Logger = slogger.DefaultLogger
	if buildOpts.Logger != nil {
		logger = buildOpts.Logger
	} else if env.Config.Logging.Level != "" {
		levelStr := env.Config.Logging.Level
		if !isValidLogLevel(levelStr) {
			return nil, fmt.Errorf("invalid log level: %s", levelStr)
		}
		level := slogger.LevelFromString(levelStr)
		logger = slogger.New(level)
	}

	// Tools
	toolsMap, err := initializeTools(env.Tools)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tools: %w", err)
	}

	// Agents
	agents := make([]dive.Agent, 0, len(env.Agents))
	for _, agentDef := range env.Agents {
		agent, err := buildAgent(agentDef, env.Config, toolsMap, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	// Workflows
	var workflows []*workflow.Workflow
	for _, workflowDef := range env.Workflows {
		workflow, err := buildWorkflow(workflowDef, agents)
		if err != nil {
			return nil, fmt.Errorf("failed to build workflow %s: %w", workflowDef.Name, err)
		}
		workflows = append(workflows, workflow)
	}

	// Triggers
	var triggers []*environment.Trigger
	for _, triggerDef := range env.Triggers {
		trigger, err := buildTrigger(triggerDef)
		if err != nil {
			return nil, fmt.Errorf("failed to build trigger %s: %w", triggerDef.Name, err)
		}
		triggers = append(triggers, trigger)
	}

	// Documents
	if buildOpts.DocumentsDir != "" && buildOpts.DocumentsRepo != nil {
		return nil, fmt.Errorf("documents dir and repo cannot both be set")
	}
	var docRepo dive.DocumentRepository
	if buildOpts.DocumentsRepo != nil {
		docRepo = buildOpts.DocumentsRepo
	} else {
		dir := buildOpts.DocumentsDir
		if dir == "" {
			if env.Config.Documents.Root != "" {
				dir = env.Config.Documents.Root
			} else {
				dir = "."
			}
		}
		docRepo, err = agent.NewFileDocumentRepository(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to create document repository: %w", err)
		}
	}
	if docRepo != nil {
		namedDocuments := make(map[string]*dive.DocumentMetadata, len(env.Documents))
		for _, doc := range env.Documents {
			namedDocuments[doc.Name] = &dive.DocumentMetadata{
				Name: doc.Name,
				Path: doc.Path,
			}
		}
		for _, doc := range env.Documents {
			if err := docRepo.RegisterDocument(context.Background(), doc.Name, doc.Path); err != nil {
				return nil, fmt.Errorf("failed to register document %s: %w", doc.Name, err)
			}
		}
	}

	var threadRepo dive.ThreadRepository
	if buildOpts.ThreadRepo != nil {
		threadRepo = buildOpts.ThreadRepo
	} else {
		threadRepo = agent.NewMemoryThreadRepository()
	}

	// Environment
	result, err := environment.New(environment.Options{
		Name:               env.Name,
		Description:        env.Description,
		Agents:             agents,
		Workflows:          workflows,
		Triggers:           triggers,
		Logger:             logger,
		DocumentRepository: docRepo,
		ThreadRepository:   threadRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}
	return result, nil
}
