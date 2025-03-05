package teamconf

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// HCLTeam represents the top-level HCL structure
type HCLTeam struct {
	Name        string        `hcl:"name,optional"`
	Description string        `hcl:"description,optional"`
	Agents      []Agent       `hcl:"agent,block"`
	Tasks       []Task        `hcl:"task,block"`
	Config      Config        `hcl:"config,block"`
	Variables   []HCLVariable `hcl:"variable,block"`
	Tools       []HCLTool     `hcl:"tool,block"`
}

// HCLTool represents a tool definition in HCL
type HCLTool struct {
	Name    string `hcl:"name,label"`
	Enabled bool   `hcl:"enabled,optional"`
}

// HCLVariable represents a variable definition in HCL
type HCLVariable struct {
	Name        string `hcl:"name,label"`
	Type        string `hcl:"type"`
	Description string `hcl:"description,optional"`
	Default     string `hcl:"default,optional"`
}

// VariableValues represents the values for variables
type VariableValues map[string]cty.Value

// LoadHCLDefinition loads an HCL definition from a string
func LoadHCLDefinition(conf []byte, filename string, vars map[string]interface{}) (*HCLTeam, error) {
	parser := hclparse.NewParser()

	// Read and parse the HCL string
	file, diags := parser.ParseHCL([]byte(conf), filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse hcl: %s", diags.Error())
	}

	// Create evaluation context with functions
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(make(map[string]cty.Value)),
		},
		Functions: createStandardFunctions(),
	}

	variableValues := make(VariableValues, len(vars))
	for k, v := range vars {
		switch v.(type) {
		case string:
			variableValues[k] = cty.StringVal(v.(string))
		case int:
			variableValues[k] = cty.NumberIntVal(int64(v.(int)))
		case float64:
			variableValues[k] = cty.NumberFloatVal(v.(float64))
		case bool:
			variableValues[k] = cty.BoolVal(v.(bool))
		default:
			return nil, fmt.Errorf("unsupported variable type: %T (key: %s)", v, k)
		}
	}

	// First pass: extract variable definitions
	content, diags := file.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "variable", LabelNames: []string{"name"}},
			{Type: "agent", LabelNames: []string{"name"}},
			{Type: "task", LabelNames: []string{"name"}},
			{Type: "config", LabelNames: []string{}},
			{Type: "tool", LabelNames: []string{"name"}},
		},
		Attributes: []hcl.AttributeSchema{
			{Name: "name", Required: false},
			{Name: "description", Required: false},
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
		if value, exists := variableValues[varName]; exists {
			varValues[varName] = value
			continue
		}

		// Try to extract type, description, and default value
		varContent, diags := block.Body.Content(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "type", Required: true},
				{Name: "description", Required: false},
				{Name: "default", Required: false},
			},
		})
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to decode variable %s: %s", varName, diags.Error())
		}

		// Process default value if present
		if defaultAttr, exists := varContent.Attributes["default"]; exists {
			val, diags := defaultAttr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate default value for variable %s: %s", varName, diags.Error())
			}
			varValues[varName] = val
		} else {
			return nil, fmt.Errorf("variable %q is required", varName)
		}
	}

	// Error if any variables were passed that are not defined in the HCL
	for k := range variableValues {
		if _, exists := varValues[k]; !exists {
			return nil, fmt.Errorf("unknown variable provided: %q", k)
		}
	}

	// Update the evaluation context with the variable values
	evalCtx.Variables["var"] = cty.ObjectVal(varValues)

	// Second pass: decode the full configuration with variables and functions
	var def HCLTeam

	// Create maps to store task and agent references
	taskRefs := make(map[string]cty.Value)
	agentRefs := make(map[string]cty.Value)
	toolRefs := make(map[string]cty.Value)

	// First pass through blocks to collect task, agent, and tool names for references
	for _, block := range content.Blocks {
		switch block.Type {
		case "task":
			taskName := block.Labels[0]
			taskRefs[taskName] = cty.StringVal(taskName)
		case "agent":
			agentName := block.Labels[0]
			agentRefs[agentName] = cty.StringVal(agentName)
		case "tool":
			toolName := block.Labels[0]
			toolRefs[toolName] = cty.StringVal(toolName)
		}
	}

	// Add references to evaluation context
	evalCtx.Variables["tasks"] = cty.ObjectVal(taskRefs)
	evalCtx.Variables["agents"] = cty.ObjectVal(agentRefs)
	evalCtx.Variables["tools"] = cty.ObjectVal(toolRefs)

	// Use a custom schema to handle blocks with variables
	fullContent, diags := file.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "agent", LabelNames: []string{"name"}},
			{Type: "task", LabelNames: []string{"name"}},
			{Type: "config", LabelNames: []string{}},
			{Type: "variable", LabelNames: []string{"name"}},
			{Type: "tool", LabelNames: []string{"name"}},
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
			// Skip variables, they were already processed
			continue

		case "config":
			var config Config
			if diags := gohcl.DecodeBody(block.Body, evalCtx, &config); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode config block: %s", diags.Error())
			}
			def.Config = config

		case "agent":
			var agent Agent
			agent.Name = block.Labels[0]

			// Get the full content first
			content, diags := block.Body.Content(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "description", Required: false},
					{Name: "instructions", Required: false},
					{Name: "is_supervisor", Required: false},
					{Name: "subordinates", Required: false},
					{Name: "accepted_events", Required: false},
					{Name: "provider", Required: false},
					{Name: "model", Required: false},
					{Name: "tools", Required: false},
					{Name: "cache_control", Required: false},
					{Name: "max_active_tasks", Required: false},
					{Name: "task_timeout", Required: false},
					{Name: "chat_timeout", Required: false},
					{Name: "tool_iteration_limit", Required: false},
					{Name: "log_level", Required: false},
				},
			})
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode agent block: %s", diags.Error())
			}

			// Process each attribute
			for name, attr := range content.Attributes {
				val, diags := attr.Expr.Value(evalCtx)
				if diags.HasErrors() {
					return nil, fmt.Errorf("failed to evaluate %s: %s", name, diags.Error())
				}

				switch name {
				case "description":
					if val.Type() == cty.String {
						agent.Description = val.AsString()
					}
				case "instructions":
					if val.Type() == cty.String {
						agent.Instructions = val.AsString()
					}
				case "is_supervisor":
					if val.Type() == cty.Bool {
						agent.IsSupervisor = val.True()
					}
				case "subordinates":
					if !val.CanIterateElements() {
						return nil, fmt.Errorf("subordinates must be a list")
					}
					for it := val.ElementIterator(); it.Next(); {
						_, v := it.Element()
						if v.Type() != cty.String {
							return nil, fmt.Errorf("subordinate must be a string")
						}
						agent.Subordinates = append(agent.Subordinates, v.AsString())
					}
				case "accepted_events":
					if !val.CanIterateElements() {
						return nil, fmt.Errorf("accepted_events must be a list")
					}
					for it := val.ElementIterator(); it.Next(); {
						_, v := it.Element()
						if v.Type() != cty.String {
							return nil, fmt.Errorf("accepted event must be a string")
						}
						agent.AcceptedEvents = append(agent.AcceptedEvents, v.AsString())
					}
				case "provider":
					if val.Type() == cty.String {
						agent.Provider = val.AsString()
					}
				case "model":
					if val.Type() == cty.String {
						agent.Model = val.AsString()
					}
				case "tools":
					if !val.CanIterateElements() {
						return nil, fmt.Errorf("tools must be a list")
					}
					for it := val.ElementIterator(); it.Next(); {
						_, v := it.Element()
						if v.Type() != cty.String {
							return nil, fmt.Errorf("tool must be a string")
						}
						agent.Tools = append(agent.Tools, v.AsString())
					}
				case "cache_control":
					if val.Type() == cty.String {
						agent.CacheControl = val.AsString()
					}
				case "task_timeout":
					if val.Type() == cty.String {
						agent.TaskTimeout = val.AsString()
					}
				case "chat_timeout":
					if val.Type() == cty.String {
						agent.ChatTimeout = val.AsString()
					}
				case "tool_iteration_limit":
					if val.Type() == cty.Number {
						v, _ := val.AsBigFloat().Int64()
						agent.ToolIterationLimit = int(v)
					}
				case "log_level":
					if val.Type() == cty.String {
						agent.LogLevel = val.AsString()
					}
				}
			}

			def.Agents = append(def.Agents, agent)

		case "task":
			var task Task
			task.Name = block.Labels[0]

			// Get the full content first
			content, diags := block.Body.Content(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "description", Required: false},
					{Name: "expected_output", Required: false},
					{Name: "output_format", Required: false},
					{Name: "assigned_agent", Required: false},
					{Name: "dependencies", Required: false},
					{Name: "output_file", Required: false},
					{Name: "timeout", Required: false},
					{Name: "context", Required: false},
				},
			})
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode task block: %s", diags.Error())
			}

			// Process each attribute
			for name, attr := range content.Attributes {
				val, diags := attr.Expr.Value(evalCtx)
				if diags.HasErrors() {
					return nil, fmt.Errorf("failed to evaluate %s: %s", name, diags.Error())
				}

				switch name {
				case "description":
					if val.Type() == cty.String {
						task.Description = val.AsString()
					}
				case "expected_output":
					if val.Type() == cty.String {
						task.ExpectedOutput = val.AsString()
					}
				case "output_format":
					if val.Type() == cty.String {
						task.OutputFormat = val.AsString()
					}
				case "assigned_agent":
					if val.Type() == cty.String {
						task.AssignedAgent = val.AsString()
					}
				case "dependencies":
					if !val.CanIterateElements() {
						return nil, fmt.Errorf("dependencies must be a list")
					}
					for it := val.ElementIterator(); it.Next(); {
						_, v := it.Element()
						if v.Type() != cty.String {
							return nil, fmt.Errorf("dependency must be a string")
						}
						task.Dependencies = append(task.Dependencies, v.AsString())
					}
				case "output_file":
					if val.Type() == cty.String {
						task.OutputFile = val.AsString()
					}
				case "timeout":
					if val.Type() == cty.String {
						task.Timeout = val.AsString()
					}
				case "context":
					if val.Type() == cty.String {
						task.Context = val.AsString()
					}
				}
			}

			def.Tasks = append(def.Tasks, task)

		case "tool":
			var tool HCLTool
			tool.Name = block.Labels[0]
			if diags := gohcl.DecodeBody(block.Body, evalCtx, &tool); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode tool block: %s", diags.Error())
			}
			// By default, a tool defined in HCL is enabled
			if !tool.Enabled {
				tool.Enabled = true
			}
			def.Tools = append(def.Tools, tool)
		}
	}
	return &def, nil
}
