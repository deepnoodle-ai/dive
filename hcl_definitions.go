package dive

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/getstingrai/dive/slogger"
)

// HCLDefinition represents the top-level HCL structure
type HCLDefinition struct {
	Name        string           `hcl:"name,label"`
	Description string           `hcl:"description"`
	Agents      []*HCLAgent      `hcl:"agent,block"`
	Tasks       []*HCLTask       `hcl:"task,block"`
	Config      *HCLGlobalConfig `hcl:"config,block"`
	Variables   []*HCLVariable   `hcl:"variable,block"`
}

// HCLGlobalConfig contains global configuration settings
type HCLGlobalConfig struct {
	DefaultProvider string            `hcl:"default_provider,optional"`
	DefaultModel    string            `hcl:"default_model,optional"`
	LogLevel        string            `hcl:"log_level,optional"`
	CacheControl    string            `hcl:"cache_control,optional"`
	EnabledTools    []string          `hcl:"enabled_tools,optional"`
	ProviderConfigs map[string]string `hcl:"provider_configs,optional"`
}

// HCLAgent represents an agent definition in HCL
type HCLAgent struct {
	Name           string            `hcl:"name,label"`
	NameOverride   string            `hcl:"name,optional"`
	Role           *HCLRole          `hcl:"role,block"`
	Provider       string            `hcl:"provider,optional"`
	Model          string            `hcl:"model,optional"`
	Tools          []string          `hcl:"tools,optional"`
	CacheControl   string            `hcl:"cache_control,optional"`
	MaxActiveTasks int               `hcl:"max_active_tasks,optional"`
	TaskTimeout    string            `hcl:"task_timeout,optional"`
	ChatTimeout    string            `hcl:"chat_timeout,optional"`
	Config         map[string]string `hcl:"config,optional"`
}

// HCLRole represents a role definition in HCL
type HCLRole struct {
	Description   string   `hcl:"description"`
	IsSupervisor  bool     `hcl:"is_supervisor,optional"`
	Subordinates  []string `hcl:"subordinates,optional"`
	AcceptsChats  bool     `hcl:"accepts_chats,optional"`
	AcceptsEvents []string `hcl:"accepts_events,optional"`
	AcceptsWork   []string `hcl:"accepts_work,optional"`
}

// HCLTask represents a task definition in HCL
type HCLTask struct {
	Name           string   `hcl:"name,label"`
	Description    string   `hcl:"description"`
	ExpectedOutput string   `hcl:"expected_output,optional"`
	OutputFormat   string   `hcl:"output_format,optional"`
	AssignedAgent  string   `hcl:"assigned_agent,optional"`
	Dependencies   []string `hcl:"dependencies,optional"`
	MaxIterations  *int     `hcl:"max_iterations,optional"`
	OutputFile     string   `hcl:"output_file,optional"`
	Timeout        string   `hcl:"timeout,optional"`
	Context        string   `hcl:"context,optional"`
	Kind           string   `hcl:"kind,optional"`
}

// HCLVariable represents a variable definition in HCL
type HCLVariable struct {
	Name        string   `hcl:"name,label"`
	Type        string   `hcl:"type"`
	Description string   `hcl:"description,optional"`
	Default     hcl.Body `hcl:"default,optional"`
}

// VariableValues represents the values for variables
type VariableValues map[string]cty.Value

// LoadHCLDefinition loads an HCL definition from a file
func LoadHCLDefinition(filePath string, vars VariableValues) (*HCLDefinition, error) {
	parser := hclparse.NewParser()

	// Read and parse the HCL file
	file, diags := parser.ParseHCLFile(filePath)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	// Create evaluation context with functions
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(make(map[string]cty.Value)),
		},
		Functions: createStandardFunctions(),
	}

	// First pass: extract variable definitions
	var tempDef HCLDefinition
	// tempCtx := &hcl.EvalContext{
	// 	Functions: createStandardFunctions(),
	// }

	// Use a custom schema to handle variable blocks with default blocks
	content, diags := file.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
		},
	})

	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to extract variable blocks: %s", diags.Error())
	}

	// Create a map to hold variable values
	varValues := make(map[string]cty.Value)

	// Process variable blocks
	for _, block := range content.Blocks {
		if block.Type != "variable" {
			continue
		}

		varName := block.Labels[0]

		// Check if variable value is provided externally
		if value, exists := vars[varName]; exists {
			varValues[varName] = value
			continue
		}

		// Try to extract type, description, and default value
		varContent, diags := block.Body.Content(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "type", Required: true},
				{Name: "description", Required: false},
			},
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "default", LabelNames: []string{}},
			},
		})

		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to decode variable %s: %s", varName, diags.Error())
		}

		// Process default value if present
		for _, defaultBlock := range varContent.Blocks {
			if defaultBlock.Type != "default" {
				continue
			}

			defaultContent, diags := defaultBlock.Body.Content(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "value", Required: true},
				},
			})

			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode default value for variable %s: %s", varName, diags.Error())
			}

			if valueAttr, exists := defaultContent.Attributes["value"]; exists {
				val, diags := valueAttr.Expr.Value(evalCtx)
				if diags.HasErrors() {
					return nil, fmt.Errorf("failed to evaluate default value for variable %s: %s", varName, diags.Error())
				}
				varValues[varName] = val
			}
		}

		// Create a variable object for the definition
		variable := &HCLVariable{
			Name: varName,
		}

		if typeAttr, exists := varContent.Attributes["type"]; exists {
			typeVal, diags := typeAttr.Expr.Value(evalCtx)
			if !diags.HasErrors() && typeVal.Type() == cty.String {
				variable.Type = typeVal.AsString()
			}
		}

		if descAttr, exists := varContent.Attributes["description"]; exists {
			descVal, diags := descAttr.Expr.Value(evalCtx)
			if !diags.HasErrors() && descVal.Type() == cty.String {
				variable.Description = descVal.AsString()
			}
		}

		tempDef.Variables = append(tempDef.Variables, variable)
	}

	// Update the evaluation context with the variable values
	evalCtx.Variables["var"] = cty.ObjectVal(varValues)

	// Second pass: decode the full configuration with variables and functions
	var def HCLDefinition

	// Instead of using gohcl.DecodeBody directly, we need to use a custom decoder
	// that can handle variables in all contexts
	fullContent, diags := file.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "agent", LabelNames: []string{"name"}},
			{Type: "task", LabelNames: []string{"name"}},
			{Type: "config", LabelNames: []string{}},
			{Type: "variable", LabelNames: []string{"name"}},
		},
		Attributes: []hcl.AttributeSchema{
			{Name: "name", Required: false},
			{Name: "description", Required: false},
		},
	})

	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL content: %s", diags.Error())
	}

	// Process attributes at the top level
	for name, attr := range fullContent.Attributes {
		switch name {
		case "description":
			value, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate description: %s", diags.Error())
			}
			if value.Type() == cty.String {
				def.Description = value.AsString()
			}
		case "name":
			value, diags := attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate name: %s", diags.Error())
			}
			if value.Type() == cty.String {
				def.Name = value.AsString()
			}
		}
	}

	// Process blocks
	for _, block := range fullContent.Blocks {
		switch block.Type {
		case "variable":
			// Variables were already processed in the first pass
			// Just copy them to the final definition
			for _, v := range tempDef.Variables {
				if v.Name == block.Labels[0] {
					def.Variables = append(def.Variables, v)
					break
				}
			}

		case "config":
			var config HCLGlobalConfig
			if diags := gohcl.DecodeBody(block.Body, evalCtx, &config); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode config block: %s", diags.Error())
			}
			def.Config = &config

		case "agent":
			var agent HCLAgent
			agent.Name = block.Labels[0]

			if diags := gohcl.DecodeBody(block.Body, evalCtx, &agent); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode agent block: %s", diags.Error())
			}

			def.Agents = append(def.Agents, &agent)

		case "task":
			var task HCLTask
			task.Name = block.Labels[0]

			if diags := gohcl.DecodeBody(block.Body, evalCtx, &task); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode task block: %s", diags.Error())
			}

			def.Tasks = append(def.Tasks, &task)
		}
	}

	return &def, nil
}

// BuildTeamFromHCL builds a Team from an HCL definition
func BuildTeamFromHCL(ctx context.Context, def *HCLDefinition) (*DiveTeam, []*Task, error) {
	// Set up default configuration
	logLevel := "info"
	if def.Config != nil && def.Config.LogLevel != "" {
		logLevel = def.Config.LogLevel
	}

	// Create logger
	logger := slogger.New(slogger.LevelFromString(logLevel))

	// Initialize tools
	var enabledTools []string
	if def.Config != nil {
		enabledTools = def.Config.EnabledTools
	}
	toolsMap, err := initializeTools(enabledTools, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tools: %w", err)
	}

	// Create agents
	agents := make([]Agent, 0, len(def.Agents))
	for _, agentDef := range def.Agents {
		var globalConfig YAMLGlobalConfig
		if def.Config != nil {
			globalConfig = YAMLGlobalConfig{
				DefaultProvider: def.Config.DefaultProvider,
				DefaultModel:    def.Config.DefaultModel,
				LogLevel:        def.Config.LogLevel,
				CacheControl:    def.Config.CacheControl,
				EnabledTools:    def.Config.EnabledTools,
				ProviderConfigs: def.Config.ProviderConfigs,
			}
		}

		// Convert HCL agent to YAML agent for compatibility with existing code
		yamlAgent := YAMLAgent{
			Name:           agentDef.Name,
			Provider:       agentDef.Provider,
			Model:          agentDef.Model,
			Tools:          agentDef.Tools,
			CacheControl:   agentDef.CacheControl,
			MaxActiveTasks: agentDef.MaxActiveTasks,
			TaskTimeout:    agentDef.TaskTimeout,
			ChatTimeout:    agentDef.ChatTimeout,
			Config:         agentDef.Config,
		}

		if agentDef.Role != nil {
			yamlAgent.Role = YAMLRole{
				Description:   agentDef.Role.Description,
				IsSupervisor:  agentDef.Role.IsSupervisor,
				Subordinates:  agentDef.Role.Subordinates,
				AcceptsChats:  agentDef.Role.AcceptsChats,
				AcceptsEvents: agentDef.Role.AcceptsEvents,
				AcceptsWork:   agentDef.Role.AcceptsWork,
			}
		}

		agent, err := buildAgent(yamlAgent, globalConfig, toolsMap, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	// Create tasks
	tasks := make([]*Task, 0, len(def.Tasks))
	for _, taskDef := range def.Tasks {
		// Convert HCL task to YAML task for compatibility
		yamlTask := YAMLTask{
			Name:           taskDef.Name,
			Description:    taskDef.Description,
			ExpectedOutput: taskDef.ExpectedOutput,
			OutputFormat:   taskDef.OutputFormat,
			AssignedAgent:  taskDef.AssignedAgent,
			Dependencies:   taskDef.Dependencies,
			MaxIterations:  taskDef.MaxIterations,
			OutputFile:     taskDef.OutputFile,
			Timeout:        taskDef.Timeout,
			Context:        taskDef.Context,
			Kind:           taskDef.Kind,
		}

		task, err := buildTask(yamlTask, agents)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build task %s: %w", taskDef.Name, err)
		}
		tasks = append(tasks, task)
	}

	// Create team
	team, err := NewTeam(TeamOptions{
		Name:        def.Name,
		Description: def.Description,
		Agents:      agents,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create team: %w", err)
	}

	return team, tasks, nil
}

// LoadAndRunHCLTeam loads an HCL definition and runs the team
func LoadAndRunHCLTeam(ctx context.Context, filePath string, vars VariableValues) ([]*TaskResult, error) {
	def, err := LoadHCLDefinition(filePath, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to load HCL definition: %w", err)
	}

	team, tasks, err := BuildTeamFromHCL(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("failed to build team: %w", err)
	}

	if err := team.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start team: %w", err)
	}
	defer team.Stop(ctx)

	results, err := team.Work(ctx, tasks...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute work: %w", err)
	}
	return results, nil
}

// Helper functions for variable handling

// createStandardFunctions creates a set of standard functions available in HCL
func createStandardFunctions() map[string]function.Function {
	return map[string]function.Function{
		"env": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "name",
					Type: cty.String,
				},
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				name := args[0].AsString()
				value := os.Getenv(name)
				return cty.StringVal(value), nil
			},
		}),
		"concat": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name:         "lists",
					Type:         cty.DynamicPseudoType,
					AllowUnknown: true,
					AllowNull:    true,
					AllowMarked:  true,
				},
			},
			VarParam: &function.Parameter{
				Name:         "lists",
				Type:         cty.DynamicPseudoType,
				AllowUnknown: true,
				AllowNull:    true,
				AllowMarked:  true,
			},
			Type: function.StaticReturnType(cty.DynamicPseudoType),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				if len(args) == 0 {
					return cty.ListValEmpty(cty.DynamicPseudoType), nil
				}

				// Determine the element type from the first argument
				firstArgType := args[0].Type()
				if !firstArgType.IsListType() && !firstArgType.IsTupleType() {
					return cty.NilVal, fmt.Errorf("all arguments must be lists or tuples")
				}

				var result []cty.Value
				for _, arg := range args {
					if !arg.Type().IsListType() && !arg.Type().IsTupleType() {
						return cty.NilVal, fmt.Errorf("all arguments must be lists or tuples")
					}
					for it := arg.ElementIterator(); it.Next(); {
						_, v := it.Element()
						result = append(result, v)
					}
				}

				if len(result) == 0 {
					return cty.ListValEmpty(cty.DynamicPseudoType), nil
				}

				return cty.ListVal(result), nil
			},
		}),
		"format": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "format",
					Type: cty.String,
				},
			},
			VarParam: &function.Parameter{
				Name:      "args",
				Type:      cty.DynamicPseudoType,
				AllowNull: true,
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				format := args[0].AsString()
				formatArgs := make([]interface{}, len(args)-1)
				for i, arg := range args[1:] {
					switch {
					case arg.Type() == cty.String:
						formatArgs[i] = arg.AsString()
					case arg.Type() == cty.Number:
						formatArgs[i] = arg.AsBigFloat()
					case arg.Type() == cty.Bool:
						formatArgs[i] = arg.True()
					default:
						return cty.NilVal, fmt.Errorf("unsupported argument type %s", arg.Type().FriendlyName())
					}
				}
				return cty.StringVal(fmt.Sprintf(format, formatArgs...)), nil
			},
		}),
		"replace": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "str",
					Type: cty.String,
				},
				{
					Name: "search",
					Type: cty.String,
				},
				{
					Name: "replace",
					Type: cty.String,
				},
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				str := args[0].AsString()
				search := args[1].AsString()
				replace := args[2].AsString()

				result := strings.ReplaceAll(str, search, replace)
				return cty.StringVal(result), nil
			},
		}),
	}
}

// ParseHCLWithVars parses an HCL string with variables
func ParseHCLWithVars(hclStr string, vars VariableValues) (map[string]cty.Value, error) {
	file, diags := hclsyntax.ParseConfig([]byte(hclStr), "inline.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	// Create evaluation context with variables
	evalCtx := &hcl.EvalContext{
		Variables: make(map[string]cty.Value),
		Functions: createStandardFunctions(),
	}

	// Add provided variables to context
	for k, v := range vars {
		evalCtx.Variables[k] = v
	}

	// Extract attributes
	attrs, diags := file.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to extract attributes: %s", diags.Error())
	}

	// Evaluate each attribute
	result := make(map[string]cty.Value)
	for name, attr := range attrs {
		val, diags := attr.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to evaluate attribute %s: %s", name, diags.Error())
		}
		result[name] = val
	}

	return result, nil
}

// StringWithVars evaluates a string with variables
func StringWithVars(input string, vars VariableValues) (string, error) {
	// Wrap the input in an HCL attribute for parsing
	hclStr := fmt.Sprintf("value = %s", input)

	result, err := ParseHCLWithVars(hclStr, vars)
	if err != nil {
		return "", err
	}

	val, ok := result["value"]
	if !ok {
		return "", fmt.Errorf("failed to evaluate string with variables")
	}

	if val.Type() != cty.String {
		return "", fmt.Errorf("evaluated expression is not a string")
	}

	return val.AsString(), nil
}
