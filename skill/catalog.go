package skill

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// BuildCatalog returns the skill catalog formatted for injection into
// conversation context. Only includes agent-invocable skills (not commands).
//
// The output is wrapped in <skills> tags, matching Claude Code's pattern
// of injecting skill metadata as structured blocks in conversation context
// rather than in the tool description.
//
// Returns an empty string if no agent-invocable skills are loaded.
func BuildCatalog(loader *Loader) string {
	skills := loader.Skills()
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	// The header is a completeness claim, not an additive one, so a later
	// catalog's facts conflict with any earlier catalog's — including skills
	// that were removed — and the priming rule resolves to the later block.
	sb.WriteString("Complete list of skills available for use with the Skill tool; any skill not listed here is unavailable:\n\n")

	for _, s := range skills {
		fmt.Fprintf(&sb, "- %s: %s\n", s.Name, s.Description)
		// Include file location so the agent knows where the skill lives on disk
		if s.FilePath != "" {
			fmt.Fprintf(&sb, "  Location: %s\n", s.FilePath)
		}
		// Add trigger hints if present
		for _, t := range s.Config.Triggers {
			if t.Keyword != "" {
				fmt.Fprintf(&sb, "  TRIGGER when: user mentions %q\n", t.Keyword)
			} else if t.Pattern != "" {
				fmt.Fprintf(&sb, "  TRIGGER when: input matches pattern %q\n", t.Pattern)
			}
		}
	}

	sb.WriteString("\nWhen a task matches a skill's description or trigger, invoke the Skill tool with the skill name before proceeding. Do not guess skill names — only use skills listed above.")

	return sb.String()
}

// CatalogHash returns a hash of the current skill catalog content.
// Used to detect when the catalog has changed and needs re-injection.
func CatalogHash(loader *Loader) string {
	catalog := BuildCatalog(loader)
	if catalog == "" {
		return ""
	}
	h := sha256.Sum256([]byte(catalog))
	return fmt.Sprintf("%x", h[:8])
}

// SkillRules returns system prompt text for skill usage instructions.
// This is appended to the agent's system prompt when skills are configured.
func SkillRules() string {
	return "You have access to specialized skills via the Skill tool. " +
		"Available skills are listed in <system-reminder name=\"skills\"> blocks in the conversation. " +
		"When a task matches a skill's description or trigger condition, " +
		"invoke the Skill tool with the skill name before proceeding. " +
		"Do not guess skill names — only use skills listed in system-reminder blocks."
}
