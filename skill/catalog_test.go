package skill

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestBuildCatalog(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code for issues.",
				FilePath:    "/home/user/.dive/skills/reviewer/SKILL.md",
				Config: SkillConfig{
					Description: "Review code for issues.",
					Triggers: []Trigger{
						{Keyword: "review"},
						{Pattern: "review .+"},
					},
				},
			},
			"deploy": {
				Name:        "deploy",
				Description: "Deploy to an environment.",
				Config:      SkillConfig{Description: "Deploy to an environment."},
			},
			"commit": {
				Name: "commit",
				// No description — this is a command, should be excluded
			},
		},
	}

	catalog := BuildCatalog(loader)

	assert.Contains(t, catalog, "The following skills are available")
	assert.Contains(t, catalog, "reviewer: Review code for issues.")
	assert.Contains(t, catalog, "deploy: Deploy to an environment.")
	assert.Contains(t, catalog, "Location: /home/user/.dive/skills/reviewer/SKILL.md")
	assert.Contains(t, catalog, `TRIGGER when: user mentions "review"`)
	assert.Contains(t, catalog, `TRIGGER when: input matches pattern "review .+"`)
	// Skill without FilePath should not have Location line
	assert.NotContains(t, catalog, "Location: \n")
	// Command should be excluded
	assert.NotContains(t, catalog, "commit")
}

func TestBuildCatalog_Empty(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{}}
	assert.Equal(t, "", BuildCatalog(loader))
}

func TestBuildCatalog_OnlyCommands(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"commit": {Name: "commit"}, // No description = command
		},
	}
	assert.Equal(t, "", BuildCatalog(loader))
}

func TestCatalogHash(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"reviewer": {
				Name:        "reviewer",
				Description: "Review code.",
				Config:      SkillConfig{Description: "Review code."},
			},
		},
	}

	hash1 := CatalogHash(loader)
	assert.NotEqual(t, "", hash1)

	// Same content = same hash
	hash2 := CatalogHash(loader)
	assert.Equal(t, hash1, hash2)

	// Different content = different hash
	loader.skills["deploy"] = &Skill{
		Name:        "deploy",
		Description: "Deploy.",
		Config:      SkillConfig{Description: "Deploy."},
	}
	hash3 := CatalogHash(loader)
	assert.NotEqual(t, hash1, hash3)
}

func TestCatalogHash_Empty(t *testing.T) {
	loader := &Loader{skills: map[string]*Skill{}}
	assert.Equal(t, "", CatalogHash(loader))
}

func TestSkillRules(t *testing.T) {
	rules := SkillRules()
	assert.Contains(t, rules, "Skill tool")
	assert.Contains(t, rules, "system-reminder")
	// Should be reasonably concise
	assert.True(t, len(rules) < 500)
}
