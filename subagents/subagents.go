package subagents

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"gopkg.in/yaml.v3"
)

// SubagentDefinition represents a subagent configuration
type SubagentDefinition struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools,omitempty"`
	Model       string   `yaml:"model,omitempty"`
	SystemPrompt string  `yaml:"-"` // Content after frontmatter
}

// SubagentManager manages subagents
type SubagentManager struct {
	userAgents    map[string]*SubagentDefinition
	projectAgents map[string]*SubagentDefinition
}

// NewSubagentManager creates a new subagent manager
func NewSubagentManager() *SubagentManager {
	return &SubagentManager{
		userAgents:    make(map[string]*SubagentDefinition),
		projectAgents: make(map[string]*SubagentDefinition),
	}
}

// Load loads all subagent definitions
func (sm *SubagentManager) Load() error {
	// Load user subagents
	if err := sm.loadUserSubagents(); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Load project subagents
	if err := sm.loadProjectSubagents(); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// loadUserSubagents loads subagents from user directory
func (sm *SubagentManager) loadUserSubagents() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	agentsDir := filepath.Join(homeDir, ".dive", "agents")
	return sm.loadSubagentsFromDir(agentsDir, sm.userAgents)
}

// loadProjectSubagents loads subagents from project directory
func (sm *SubagentManager) loadProjectSubagents() error {
	agentsDir := filepath.Join(".dive", "agents")
	return sm.loadSubagentsFromDir(agentsDir, sm.projectAgents)
}

// loadSubagentsFromDir loads subagent definitions from a directory
func (sm *SubagentManager) loadSubagentsFromDir(dir string, target map[string]*SubagentDefinition) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		subagent, err := sm.loadSubagentFile(path)
		if err != nil {
			fmt.Printf("Warning: Failed to load subagent from %s: %v\n", path, err)
			continue
		}

		target[subagent.Name] = subagent
	}

	return nil
}

// loadSubagentFile loads a single subagent definition file
func (sm *SubagentManager) loadSubagentFile(path string) (*SubagentDefinition, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	// Check for YAML frontmatter
	firstLine, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}

	if strings.TrimSpace(firstLine) != "---" {
		return nil, fmt.Errorf("missing YAML frontmatter")
	}

	// Read YAML frontmatter
	var yamlContent strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		if strings.TrimSpace(line) == "---" {
			break
		}

		yamlContent.WriteString(line)

		if err == io.EOF {
			break
		}
	}

	// Parse YAML
	var subagent SubagentDefinition
	if err := yaml.Unmarshal([]byte(yamlContent.String()), &subagent); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Read the rest as system prompt
	var promptContent strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		promptContent.WriteString(line)

		if err == io.EOF {
			break
		}
	}

	subagent.SystemPrompt = strings.TrimSpace(promptContent.String())

	return &subagent, nil
}

// GetSubagent retrieves a subagent by name (project takes precedence)
func (sm *SubagentManager) GetSubagent(name string) (*SubagentDefinition, error) {
	// Check project agents first
	if agent, ok := sm.projectAgents[name]; ok {
		return agent, nil
	}

	// Check user agents
	if agent, ok := sm.userAgents[name]; ok {
		return agent, nil
	}

	return nil, fmt.Errorf("subagent '%s' not found", name)
}

// ListSubagents returns all available subagents
func (sm *SubagentManager) ListSubagents() map[string]*SubagentDefinition {
	result := make(map[string]*SubagentDefinition)

	// Add user agents
	for name, agent := range sm.userAgents {
		result[name] = agent
	}

	// Add project agents (overriding user agents with same name)
	for name, agent := range sm.projectAgents {
		result[name] = agent
	}

	return result
}

// CreateSubagent creates a new subagent definition file
func (sm *SubagentManager) CreateSubagent(def *SubagentDefinition, isProject bool) error {
	// Determine directory
	var dir string
	if isProject {
		dir = filepath.Join(".dive", "agents")
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(homeDir, ".dive", "agents")
	}

	// Create directory if needed
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create file
	filename := fmt.Sprintf("%s.md", def.Name)
	path := filepath.Join(dir, filename)

	// Build content
	var content strings.Builder

	// Write YAML frontmatter
	content.WriteString("---\n")
	yamlData, err := yaml.Marshal(def)
	if err != nil {
		return err
	}
	content.Write(yamlData)
	content.WriteString("---\n\n")
	content.WriteString(def.SystemPrompt)

	// Write file
	return os.WriteFile(path, []byte(content.String()), 0644)
}

// DeleteSubagent deletes a subagent definition
func (sm *SubagentManager) DeleteSubagent(name string, isProject bool) error {
	// Determine path
	var dir string
	if isProject {
		dir = filepath.Join(".dive", "agents")
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(homeDir, ".dive", "agents")
	}

	filename := fmt.Sprintf("%s.md", name)
	path := filepath.Join(dir, filename)

	return os.Remove(path)
}

// CreateAgentFromSubagent creates a Dive agent from a subagent definition
func (sm *SubagentManager) CreateAgentFromSubagent(def *SubagentDefinition, baseAgent dive.Agent) (dive.Agent, error) {
	// Build agent options
	opts := dive.AgentOptions{
		Name:         def.Name,
		Instructions: def.SystemPrompt,
	}

	// Handle model selection
	switch def.Model {
	case "inherit":
		// Use the same model as the main conversation
		// This would need to be passed in or configured
	case "sonnet", "opus", "haiku":
		// Map to actual model names based on configuration
		// This would need provider-specific mapping
	default:
		if def.Model != "" {
			// Use specified model directly
		}
	}

	// Create agent with tools
	// Note: This would need actual tool initialization based on def.Tools

	return dive.NewAgent(opts)
}

// GenerateSubagentPrompt generates a prompt for Claude to create a subagent
func GenerateSubagentPrompt(name, purpose string) string {
	return fmt.Sprintf(`Create a specialized subagent named "%s" for the following purpose:
%s

Please provide:
1. A clear, action-oriented description field
2. Appropriate tools the subagent should have access to
3. A detailed system prompt that:
   - Clearly defines the subagent's role and expertise
   - Provides specific instructions on how to approach tasks
   - Includes best practices and constraints
   - Specifies output format expectations

The subagent should be focused, efficient, and excel at its specific task.`, name, purpose)
}

// Built-in subagent templates
var BuiltInSubagents = map[string]*SubagentDefinition{
	"code-reviewer": {
		Name:        "code-reviewer",
		Description: "Expert code review specialist. Proactively reviews code for quality, security, and maintainability.",
		Tools:       []string{"Read", "Grep", "Glob", "Bash"},
		Model:       "inherit",
		SystemPrompt: `You are a senior code reviewer ensuring high standards of code quality and security.

When invoked:
1. Run git diff to see recent changes
2. Focus on modified files
3. Begin review immediately

Review checklist:
- Code is simple and readable
- Functions and variables are well-named
- No duplicated code
- Proper error handling
- No exposed secrets or API keys
- Input validation implemented
- Good test coverage
- Performance considerations addressed

Provide feedback organized by priority:
- Critical issues (must fix)
- Warnings (should fix)
- Suggestions (consider improving)

Include specific examples of how to fix issues.`,
	},
	"debugger": {
		Name:        "debugger",
		Description: "Debugging specialist for errors, test failures, and unexpected behavior.",
		Tools:       []string{"Read", "Edit", "Bash", "Grep", "Glob"},
		Model:       "inherit",
		SystemPrompt: `You are an expert debugger specializing in root cause analysis.

When invoked:
1. Capture error message and stack trace
2. Identify reproduction steps
3. Isolate the failure location
4. Implement minimal fix
5. Verify solution works

Debugging process:
- Analyze error messages and logs
- Check recent code changes
- Form and test hypotheses
- Add strategic debug logging
- Inspect variable states

For each issue, provide:
- Root cause explanation
- Evidence supporting the diagnosis
- Specific code fix
- Testing approach
- Prevention recommendations

Focus on fixing the underlying issue, not just symptoms.`,
	},
	"test-runner": {
		Name:        "test-runner",
		Description: "Test automation expert. Proactively runs tests and fixes failures.",
		Tools:       []string{"Read", "Edit", "Bash", "Grep"},
		Model:       "inherit",
		SystemPrompt: `You are a test automation expert.

When invoked:
1. Identify the appropriate test commands
2. Run tests relevant to recent changes
3. Analyze any failures
4. Fix failing tests while preserving intent
5. Ensure all tests pass

Testing approach:
- Run unit tests first for quick feedback
- Follow with integration tests
- Check test coverage if available
- Validate test assertions are meaningful

For test failures:
- Determine if it's a test issue or code issue
- Fix the root cause, not just the symptom
- Ensure tests remain maintainable
- Add new tests if gaps are found

Report:
- Test execution summary
- Failures fixed
- Coverage improvements
- Recommendations for additional tests`,
	},
}