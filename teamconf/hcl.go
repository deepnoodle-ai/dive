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
	Name        string   `hcl:"name,label"`
	Type        string   `hcl:"type"`
	Description string   `hcl:"description,optional"`
	Default     hcl.Body `hcl:"default,block"`
}

// VariableValues represents the values for variables
type VariableValues map[string]cty.Value

// LoadHCLDefinition loads an HCL definition from a string
func LoadHCLDefinition(conf []byte, filename string, vars VariableValues) (*HCLTeam, error) {
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
	}

	// Update the evaluation context with the variable values
	evalCtx.Variables["var"] = cty.ObjectVal(varValues)

	// Second pass: decode the full configuration with variables and functions
	var def HCLTeam

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
			if diags := gohcl.DecodeBody(block.Body, evalCtx, &agent); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode agent block: %s", diags.Error())
			}
			if agent.NameOverride != "" {
				agent.Name = agent.NameOverride
			}
			def.Agents = append(def.Agents, agent)

		case "task":
			var task Task
			task.Name = block.Labels[0]
			if diags := gohcl.DecodeBody(block.Body, evalCtx, &task); diags.HasErrors() {
				return nil, fmt.Errorf("failed to decode task block: %s", diags.Error())
			}
			if task.NameOverride != "" {
				task.Name = task.NameOverride
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
