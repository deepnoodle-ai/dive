package environment

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/diveagents/dive/workflow"
)

// WorkflowAnalyzer analyzes workflows for non-deterministic operations and migration requirements
type WorkflowAnalyzer struct {
	// Patterns for identifying non-deterministic operations
	timePatterns   []*regexp.Regexp
	randomPatterns []*regexp.Regexp
	httpPatterns   []*regexp.Regexp
	filePatterns   []*regexp.Regexp
}

// AnalysisResult contains the results of analyzing a workflow
type AnalysisResult struct {
	WorkflowName             string                      `json:"workflow_name"`
	NonDeterministicOps      []NonDeterministicOperation `json:"non_deterministic_operations"`
	MigrationSuggestions     []MigrationSuggestion       `json:"migration_suggestions"`
	CompatibilityLevel       CompatibilityLevel          `json:"compatibility_level"`
	EstimatedMigrationEffort string                      `json:"estimated_migration_effort"`
}

// NonDeterministicOperation represents a detected non-deterministic operation
type NonDeterministicOperation struct {
	Type        string `json:"type"` // "time", "random", "http", "file", "external"
	StepName    string `json:"step_name"`
	Location    string `json:"location"` // "prompt", "condition", "action_params"
	Pattern     string `json:"pattern"`  // The actual pattern found
	Severity    string `json:"severity"` // "high", "medium", "low"
	Description string `json:"description"`
}

// MigrationSuggestion provides guidance on how to migrate specific operations
type MigrationSuggestion struct {
	OperationType   string   `json:"operation_type"`
	CurrentPattern  string   `json:"current_pattern"`
	SuggestedFix    string   `json:"suggested_fix"`
	ExampleCode     string   `json:"example_code"`
	RequiredChanges []string `json:"required_changes"`
}

// CompatibilityLevel indicates how compatible a workflow is with deterministic execution
type CompatibilityLevel string

const (
	CompatibilityFullyCompatible   CompatibilityLevel = "fully_compatible"
	CompatibilityMostlyCompatible  CompatibilityLevel = "mostly_compatible"
	CompatibilityRequiresMigration CompatibilityLevel = "requires_migration"
	CompatibilityNotCompatible     CompatibilityLevel = "not_compatible"
)

// NewWorkflowAnalyzer creates a new workflow analyzer
func NewWorkflowAnalyzer() *WorkflowAnalyzer {
	return &WorkflowAnalyzer{
		timePatterns: []*regexp.Regexp{
			regexp.MustCompile(`time\.Now\(\)`),
			regexp.MustCompile(`time\.Unix\w*\(`),
			regexp.MustCompile(`\.Format\(.*time.*\)`),
			regexp.MustCompile(`Date\(\)`),
			regexp.MustCompile(`Timestamp\(\)`),
			regexp.MustCompile(`getTime\(\)`),
			regexp.MustCompile(`currentTime`),
		},
		randomPatterns: []*regexp.Regexp{
			regexp.MustCompile(`rand\.\w+\(`),
			regexp.MustCompile(`Math\.random\(\)`),
			regexp.MustCompile(`random\(\)`),
			regexp.MustCompile(`uuid\.\w+\(`),
			regexp.MustCompile(`generateId\(\)`),
			regexp.MustCompile(`RandomString\(`),
			regexp.MustCompile(`RandomInt\(`),
		},
		httpPatterns: []*regexp.Regexp{
			regexp.MustCompile(`http\.\w+\(`),
			regexp.MustCompile(`fetch\(`),
			regexp.MustCompile(`axios\.\w+\(`),
			regexp.MustCompile(`\.get\(.*http`),
			regexp.MustCompile(`\.post\(.*http`),
			regexp.MustCompile(`WebSearch\(`),
			regexp.MustCompile(`WebFetch\(`),
		},
		filePatterns: []*regexp.Regexp{
			regexp.MustCompile(`os\.\w+\(`),
			regexp.MustCompile(`fs\.\w+\(`),
			regexp.MustCompile(`readFile\(`),
			regexp.MustCompile(`writeFile\(`),
			regexp.MustCompile(`Document\.Read\(`),
			regexp.MustCompile(`Document\.Write\(`),
		},
	}
}

// AnalyzeWorkflow performs a comprehensive analysis of a workflow
func (w *WorkflowAnalyzer) AnalyzeWorkflow(workflow *workflow.Workflow) (*AnalysisResult, error) {
	result := &AnalysisResult{
		WorkflowName:         workflow.Name(),
		NonDeterministicOps:  []NonDeterministicOperation{},
		MigrationSuggestions: []MigrationSuggestion{},
	}

	// Analyze each step
	for _, step := range workflow.Steps() {
		w.analyzeStep(step, result)
	}

	// Determine compatibility level
	result.CompatibilityLevel = w.determineCompatibilityLevel(result.NonDeterministicOps)
	result.EstimatedMigrationEffort = w.estimateMigrationEffort(result.NonDeterministicOps)

	// Generate migration suggestions
	result.MigrationSuggestions = w.generateMigrationSuggestions(result.NonDeterministicOps)

	return result, nil
}

// analyzeStep analyzes a single workflow step for non-deterministic operations
func (w *WorkflowAnalyzer) analyzeStep(step *workflow.Step, result *AnalysisResult) {
	stepName := step.Name()

	// Analyze prompt for non-deterministic operations
	if prompt := step.Prompt(); prompt != "" {
		w.analyzeText(prompt, stepName, "prompt", result)
	}

	// Analyze conditions
	for _, edge := range step.Next() {
		if condition := edge.Condition; condition != "" {
			w.analyzeText(condition, stepName, "condition", result)
		}
	}

	// Analyze action parameters
	if step.Type() == "action" {
		for paramName, paramValue := range step.Parameters() {
			if strValue, ok := paramValue.(string); ok {
				location := fmt.Sprintf("action_params.%s", paramName)
				w.analyzeText(strValue, stepName, location, result)
			}
		}
	}

	// Analyze Each block
	if each := step.Each(); each != nil {
		if itemsStr, ok := each.Items.(string); ok {
			w.analyzeText(itemsStr, stepName, "each.items", result)
		}
	}
}

// analyzeText analyzes a text string for non-deterministic patterns
func (w *WorkflowAnalyzer) analyzeText(text, stepName, location string, result *AnalysisResult) {
	// Check for time-related operations
	w.checkPatterns(text, stepName, location, "time", w.timePatterns, result)

	// Check for random operations
	w.checkPatterns(text, stepName, location, "random", w.randomPatterns, result)

	// Check for HTTP operations
	w.checkPatterns(text, stepName, location, "http", w.httpPatterns, result)

	// Check for file operations
	w.checkPatterns(text, stepName, location, "file", w.filePatterns, result)
}

// checkPatterns checks text against a set of regex patterns
func (w *WorkflowAnalyzer) checkPatterns(text, stepName, location, opType string, patterns []*regexp.Regexp, result *AnalysisResult) {
	for _, pattern := range patterns {
		if matches := pattern.FindAllString(text, -1); len(matches) > 0 {
			for _, match := range matches {
				op := NonDeterministicOperation{
					Type:        opType,
					StepName:    stepName,
					Location:    location,
					Pattern:     match,
					Severity:    w.determineSeverity(opType, match),
					Description: w.getOperationDescription(opType, match),
				}
				result.NonDeterministicOps = append(result.NonDeterministicOps, op)
			}
		}
	}
}

// determineSeverity determines the severity of a non-deterministic operation
func (w *WorkflowAnalyzer) determineSeverity(opType, pattern string) string {
	switch opType {
	case "time", "random":
		return "high" // These are core determinism issues
	case "http":
		if strings.Contains(pattern, "WebSearch") || strings.Contains(pattern, "WebFetch") {
			return "medium" // These should use operations
		}
		return "high" // Direct HTTP calls are problematic
	case "file":
		if strings.Contains(pattern, "Document.") {
			return "medium" // Document operations should use operations
		}
		return "high" // Direct file operations are problematic
	default:
		return "medium"
	}
}

// getOperationDescription provides a description of the non-deterministic operation
func (w *WorkflowAnalyzer) getOperationDescription(opType, pattern string) string {
	switch opType {
	case "time":
		return fmt.Sprintf("Time-dependent operation '%s' should use deterministicNow() or deterministicUnix()", pattern)
	case "random":
		return fmt.Sprintf("Random operation '%s' should use deterministicRandom() or deterministicRandomInt()", pattern)
	case "http":
		return fmt.Sprintf("HTTP operation '%s' should be wrapped in an operation for deterministic replay", pattern)
	case "file":
		return fmt.Sprintf("File operation '%s' should be wrapped in an operation for deterministic replay", pattern)
	default:
		return fmt.Sprintf("Operation '%s' may not be deterministic", pattern)
	}
}

// determineCompatibilityLevel determines the overall compatibility level
func (w *WorkflowAnalyzer) determineCompatibilityLevel(ops []NonDeterministicOperation) CompatibilityLevel {
	if len(ops) == 0 {
		return CompatibilityFullyCompatible
	}

	highSeverityCount := 0
	mediumSeverityCount := 0

	for _, op := range ops {
		switch op.Severity {
		case "high":
			highSeverityCount++
		case "medium":
			mediumSeverityCount++
		}
	}

	if highSeverityCount > 5 {
		return CompatibilityNotCompatible
	} else if highSeverityCount > 0 {
		return CompatibilityRequiresMigration
	} else if mediumSeverityCount > 0 {
		return CompatibilityMostlyCompatible
	}

	return CompatibilityFullyCompatible
}

// estimateMigrationEffort estimates the effort required to migrate a workflow
func (w *WorkflowAnalyzer) estimateMigrationEffort(ops []NonDeterministicOperation) string {
	totalOps := len(ops)

	if totalOps == 0 {
		return "No migration needed"
	} else if totalOps <= 3 {
		return "Low effort (1-2 hours)"
	} else if totalOps <= 10 {
		return "Medium effort (4-8 hours)"
	} else if totalOps <= 20 {
		return "High effort (1-2 days)"
	} else {
		return "Very high effort (3+ days)"
	}
}

// generateMigrationSuggestions generates specific migration suggestions
func (w *WorkflowAnalyzer) generateMigrationSuggestions(ops []NonDeterministicOperation) []MigrationSuggestion {
	suggestions := []MigrationSuggestion{}
	seen := make(map[string]bool)

	for _, op := range ops {
		key := fmt.Sprintf("%s_%s", op.Type, op.Pattern)
		if seen[key] {
			continue
		}
		seen[key] = true

		suggestion := w.createMigrationSuggestion(op)
		if suggestion != nil {
			suggestions = append(suggestions, *suggestion)
		}
	}

	return suggestions
}

// createMigrationSuggestion creates a specific migration suggestion for an operation
func (w *WorkflowAnalyzer) createMigrationSuggestion(op NonDeterministicOperation) *MigrationSuggestion {
	switch op.Type {
	case "time":
		return &MigrationSuggestion{
			OperationType:  "time",
			CurrentPattern: op.Pattern,
			SuggestedFix:   "Replace with deterministic time functions",
			ExampleCode:    w.getTimeFixExample(op.Pattern),
			RequiredChanges: []string{
				"Import deterministic runtime functions in script globals",
				"Replace time.Now() with deterministicNow()",
				"Replace time.Unix() with deterministicUnix()",
			},
		}
	case "random":
		return &MigrationSuggestion{
			OperationType:  "random",
			CurrentPattern: op.Pattern,
			SuggestedFix:   "Replace with deterministic random functions",
			ExampleCode:    w.getRandomFixExample(op.Pattern),
			RequiredChanges: []string{
				"Import deterministic runtime functions in script globals",
				"Replace rand.* with deterministicRandom()",
				"Replace random number generation with deterministicRandomInt()",
			},
		}
	case "http":
		return &MigrationSuggestion{
			OperationType:  "http",
			CurrentPattern: op.Pattern,
			SuggestedFix:   "Wrap in operation or use built-in actions",
			ExampleCode:    "Use WebSearch or WebFetch actions instead of direct HTTP calls",
			RequiredChanges: []string{
				"Convert to action step using WebSearch/WebFetch",
				"Or wrap in custom operation if needed",
			},
		}
	case "file":
		return &MigrationSuggestion{
			OperationType:  "file",
			CurrentPattern: op.Pattern,
			SuggestedFix:   "Use Document operations or wrap in operation",
			ExampleCode:    "Use Document.Read/Write actions or wrap file operations",
			RequiredChanges: []string{
				"Convert to Document.Read/Write actions",
				"Or wrap in custom operation for deterministic replay",
			},
		}
	default:
		return nil
	}
}

// getTimeFixExample provides example code for fixing time operations
func (w *WorkflowAnalyzer) getTimeFixExample(pattern string) string {
	switch {
	case strings.Contains(pattern, "time.Now()"):
		return "Replace: time.Now() → deterministicNow()"
	case strings.Contains(pattern, "time.Unix"):
		return "Replace: time.Unix*() → deterministicUnix()"
	default:
		return "Use deterministicNow() for current time access"
	}
}

// getRandomFixExample provides example code for fixing random operations
func (w *WorkflowAnalyzer) getRandomFixExample(pattern string) string {
	switch {
	case strings.Contains(pattern, "rand."):
		return "Replace: rand.*() → deterministicRandom() or deterministicRandomInt()"
	case strings.Contains(pattern, "Math.random"):
		return "Replace: Math.random() → deterministicRandom()"
	case strings.Contains(pattern, "uuid"):
		return "Replace: uuid.*() → deterministicRandomString(36, \"0123456789abcdef-\")"
	default:
		return "Use deterministicRandom() or deterministicRandomInt() for random values"
	}
}
