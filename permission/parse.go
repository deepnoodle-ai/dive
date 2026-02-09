package permission

import (
	"fmt"
	"strings"
)

// ParseRule parses a string like "Bash(go test *)" into a Rule.
// The format is: ToolPattern or ToolPattern(specifier).
func ParseRule(ruleType RuleType, spec string) (Rule, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Rule{}, fmt.Errorf("empty rule spec")
	}

	// Check for parameterized pattern: ToolName(specifier)
	if idx := strings.Index(spec, "("); idx > 0 && strings.HasSuffix(spec, ")") {
		toolPattern := strings.TrimSpace(spec[:idx])
		specifier := strings.TrimSpace(spec[idx+1 : len(spec)-1])
		if toolPattern == "" {
			return Rule{}, fmt.Errorf("empty tool pattern in rule spec: %s", spec)
		}
		if specifier == "" {
			return Rule{}, fmt.Errorf("empty specifier in rule spec: %s", spec)
		}
		return Rule{
			Type:      ruleType,
			Tool:      toolPattern,
			Specifier: specifier,
		}, nil
	}

	// Simple tool pattern
	return Rule{
		Type: ruleType,
		Tool: spec,
	}, nil
}

// ParseRuleWithSpecifier parses a tool pattern and specifier into a Rule.
func ParseRuleWithSpecifier(ruleType RuleType, toolPattern, specifier string) Rule {
	return Rule{
		Type:      ruleType,
		Tool:      toolPattern,
		Specifier: specifier,
	}
}
