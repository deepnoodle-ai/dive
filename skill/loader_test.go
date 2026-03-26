package skill

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestLoader_Load(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project .dive/skills directory
	projectDiveSkills := filepath.Join(tmpDir, "project", ".dive", "skills")
	assert.NoError(t, os.MkdirAll(projectDiveSkills, 0755))

	// Create project .claude/skills directory
	projectClaudeSkills := filepath.Join(tmpDir, "project", ".claude", "skills")
	assert.NoError(t, os.MkdirAll(projectClaudeSkills, 0755))

	// Create project .dive/commands directory
	projectDiveCommands := filepath.Join(tmpDir, "project", ".dive", "commands")
	assert.NoError(t, os.MkdirAll(projectDiveCommands, 0755))

	// Create home .dive/skills directory
	homeDiveSkills := filepath.Join(tmpDir, "home", ".dive", "skills")
	assert.NoError(t, os.MkdirAll(homeDiveSkills, 0755))

	// Create skill in directory format
	skillDir := filepath.Join(projectDiveSkills, "code-reviewer")
	assert.NoError(t, os.MkdirAll(skillDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: code-reviewer
description: Review code.
allowed-tools:
  - Read
  - Grep
---

Instructions for code review.`), 0644))

	// Create standalone skill file
	assert.NoError(t, os.WriteFile(filepath.Join(projectClaudeSkills, "helper.md"), []byte(`---
name: helper
description: A helper skill.
---

Helper instructions.`), 0644))

	// Create command (no description)
	assert.NoError(t, os.WriteFile(filepath.Join(projectDiveCommands, "commit.md"), []byte(`# Commit

Create a git commit with a descriptive message.`), 0644))

	// Create skill in home directory (should be lower priority)
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveSkills, "helper.md"), []byte(`---
name: helper
description: Home helper - should be ignored.
---

Home helper instructions.`), 0644))

	// Create another home skill
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveSkills, "personal.md"), []byte(`---
name: personal
description: Personal skill.
---

Personal instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})

	err := loader.Load(context.Background())
	assert.NoError(t, err)

	// Total: code-reviewer, helper, commit, personal
	assert.Equal(t, 4, loader.Count())

	// Check code-reviewer skill
	s, ok := loader.Get("code-reviewer")
	assert.True(t, ok)
	assert.Equal(t, "code-reviewer", s.Name)
	assert.Equal(t, "Review code.", s.Description)

	// Check helper skill (project one should win)
	s, ok = loader.Get("helper")
	assert.True(t, ok)
	assert.Equal(t, "A helper skill.", s.Description)

	// Check personal skill
	s, ok = loader.Get("personal")
	assert.True(t, ok)
	assert.Equal(t, "Personal skill.", s.Description)

	// Check commit command
	s, ok = loader.Get("commit")
	assert.True(t, ok)
	assert.True(t, s.IsCommand())

	// Check non-existent
	_, ok = loader.Get("non-existent")
	assert.False(t, ok)

	// Check List returns sorted
	all := loader.List()
	assert.Equal(t, 4, len(all))
	assert.Equal(t, "code-reviewer", all[0].Name)
	assert.Equal(t, "commit", all[1].Name)
	assert.Equal(t, "helper", all[2].Name)
	assert.Equal(t, "personal", all[3].Name)

	// Check Skills returns only agent-invocable
	skills := loader.Skills()
	assert.Equal(t, 3, len(skills))
	for _, s := range skills {
		assert.False(t, s.IsCommand())
	}

	// Check Commands returns only user-invocable
	cmds := loader.Commands()
	assert.Equal(t, 1, len(cmds))
	assert.Equal(t, "commit", cmds[0].Name)
}

func TestLoader_DisablePaths(t *testing.T) {
	tmpDir := t.TempDir()

	diveSkills := filepath.Join(tmpDir, ".dive", "skills")
	claudeSkills := filepath.Join(tmpDir, ".claude", "skills")
	assert.NoError(t, os.MkdirAll(diveSkills, 0755))
	assert.NoError(t, os.MkdirAll(claudeSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(diveSkills, "dive-skill.md"), []byte(`---
name: dive-skill
description: Dive skill.
---

Dive instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(claudeSkills, "claude-skill.md"), []byte(`---
name: claude-skill
description: Claude skill.
---

Claude instructions.`), 0644))

	t.Run("disable Claude paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:         tmpDir,
			HomeDir:            "/nonexistent",
			DisableClaudePaths: true,
		})
		assert.NoError(t, loader.Load(context.Background()))

		_, ok := loader.Get("dive-skill")
		assert.True(t, ok)

		_, ok = loader.Get("claude-skill")
		assert.False(t, ok)
	})

	t.Run("disable Dive paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:       tmpDir,
			HomeDir:          "/nonexistent",
			DisableDivePaths: true,
		})
		assert.NoError(t, loader.Load(context.Background()))

		_, ok := loader.Get("dive-skill")
		assert.False(t, ok)

		_, ok = loader.Get("claude-skill")
		assert.True(t, ok)
	})
}

func TestLoader_AdditionalPaths(t *testing.T) {
	tmpDir := t.TempDir()
	customSkills := filepath.Join(tmpDir, "custom-skills")
	assert.NoError(t, os.MkdirAll(customSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(customSkills, "custom.md"), []byte(`---
name: custom
description: Custom skill.
---

Custom instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir:         "/nonexistent",
		HomeDir:            "/nonexistent",
		AdditionalPaths:    []string{customSkills},
		DisableClaudePaths: true,
		DisableDivePaths:   true,
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, ok := loader.Get("custom")
	assert.True(t, ok)
	assert.Equal(t, "Custom skill.", s.Description)
}

func TestLoader_MissingDirectories(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		ProjectDir: "/nonexistent/project",
		HomeDir:    "/nonexistent/home",
	})
	err := loader.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, loader.Count())
}

func TestLoader_ZeroConfig(t *testing.T) {
	// With zero configuration, loader starts empty — no filesystem scanning
	loader := NewLoader(LoaderOptions{})
	err := loader.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, loader.Count())
}

func TestLoader_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()

	projectDive := filepath.Join(tmpDir, "project", ".dive", "skills")
	projectClaude := filepath.Join(tmpDir, "project", ".claude", "skills")
	homeDive := filepath.Join(tmpDir, "home", ".dive", "skills")
	homeClaude := filepath.Join(tmpDir, "home", ".claude", "skills")

	for _, dir := range []string{projectDive, projectClaude, homeDive, homeClaude} {
		assert.NoError(t, os.MkdirAll(dir, 0755))
	}

	assert.NoError(t, os.WriteFile(filepath.Join(projectDive, "priority.md"), []byte(`---
name: priority
description: From project .dive (should win).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(projectClaude, "priority.md"), []byte(`---
name: priority
description: From project .claude (second).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeDive, "priority.md"), []byte(`---
name: priority
description: From home .dive (third).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeClaude, "priority.md"), []byte(`---
name: priority
description: From home .claude (lowest).
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, ok := loader.Get("priority")
	assert.True(t, ok)
	assert.Equal(t, "From project .dive (should win).", s.Description)
}

func TestLoader_Reload(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill1.md"), []byte(`---
name: skill1
description: First skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})

	assert.NoError(t, loader.Load(context.Background()))
	assert.Equal(t, 1, loader.Count())

	// Add another skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill2.md"), []byte(`---
name: skill2
description: Second skill.
---
Instructions.`), 0644))

	assert.NoError(t, loader.Load(context.Background()))
	assert.Equal(t, 2, loader.Count())

	// Remove a skill
	assert.NoError(t, os.Remove(filepath.Join(skillsDir, "skill1.md")))

	assert.NoError(t, loader.Load(context.Background()))
	assert.Equal(t, 1, loader.Count())
	_, ok := loader.Get("skill1")
	assert.False(t, ok)
	_, ok = loader.Get("skill2")
	assert.True(t, ok)
}

func TestLoader_Match(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Review code.
triggers:
  - keyword: review
  - pattern: "review .+"
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "deploy.md"), []byte(`---
name: deploy
description: Deploy to env.
triggers:
  - keyword: deploy
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "helper.md"), []byte(`---
name: helper
description: General helper.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	t.Run("keyword match", func(t *testing.T) {
		matches := loader.Match("please review my code")
		assert.Equal(t, 1, len(matches))
		assert.Equal(t, "reviewer", matches[0].Name)
	})

	t.Run("pattern match", func(t *testing.T) {
		matches := loader.Match("review main.go")
		assert.Equal(t, 1, len(matches))
		assert.Equal(t, "reviewer", matches[0].Name)
	})

	t.Run("multiple matches", func(t *testing.T) {
		matches := loader.Match("review and deploy")
		assert.Equal(t, 2, len(matches))
	})

	t.Run("no matches", func(t *testing.T) {
		matches := loader.Match("write some tests")
		assert.Equal(t, 0, len(matches))
	})
}

func TestLoader_ActiveSkill(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		ProjectDir: "/nonexistent",
		HomeDir:    "/nonexistent",
	})

	assert.Nil(t, loader.ActiveSkill())

	s := &Skill{Name: "test"}
	loader.SetActiveSkill(s)
	assert.Equal(t, "test", loader.ActiveSkill().Name)

	loader.SetActiveSkill(nil)
	assert.Nil(t, loader.ActiveSkill())
}

func TestLoader_ActiveSkillClearedOnReload(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "test.md"), []byte(`---
name: test
description: Test skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, _ := loader.Get("test")
	loader.SetActiveSkill(s)
	assert.NotNil(t, loader.ActiveSkill())

	// Reload should clear active skill
	assert.NoError(t, loader.Load(context.Background()))
	assert.Nil(t, loader.ActiveSkill())
}

func TestLoader_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill.md"), []byte(`---
name: test-skill
description: A skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			loader.Get("test-skill")
			loader.List()
			loader.Names()
			loader.Count()
			loader.Skills()
			loader.Commands()
			loader.Match("test")
			loader.ActiveSkill()
		}()
	}
	wg.Wait()
}

func TestLoader_BackwardCompat(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill.md"), []byte(`---
name: test
description: A skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})

	// Use deprecated methods
	assert.NoError(t, loader.LoadSkills())
	assert.Equal(t, 1, loader.SkillCount())

	s, ok := loader.GetSkill("test")
	assert.True(t, ok)
	assert.Equal(t, "test", s.Name)

	skills := loader.ListSkills()
	assert.Equal(t, 1, len(skills))

	names := loader.ListSkillNames()
	assert.Equal(t, 1, len(names))
	assert.Equal(t, "test", names[0])
}

func TestLoader_CustomProvider(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		Providers: []Provider{
			&mockProvider{
				skills: []*Skill{
					{Name: "mock-skill", Description: "From mock.", Instructions: "Do things."},
				},
			},
		},
	})

	assert.NoError(t, loader.Load(context.Background()))
	assert.Equal(t, 1, loader.Count())

	s, ok := loader.Get("mock-skill")
	assert.True(t, ok)
	assert.Equal(t, "From mock.", s.Description)
}

type mockProvider struct {
	skills []*Skill
}

func (p *mockProvider) Name() string                              { return "mock" }
func (p *mockProvider) Load(_ context.Context) ([]*Skill, error) { return p.skills, nil }

func TestLoader_SourceField(t *testing.T) {
	tmpDir := t.TempDir()

	projectSkills := filepath.Join(tmpDir, "project", ".dive", "skills")
	homeSkills := filepath.Join(tmpDir, "home", ".dive", "skills")
	assert.NoError(t, os.MkdirAll(projectSkills, 0755))
	assert.NoError(t, os.MkdirAll(homeSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(projectSkills, "proj.md"), []byte(`---
name: proj
description: Project skill.
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeSkills, "home.md"), []byte(`---
name: home
description: Home skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})
	assert.NoError(t, loader.Load(context.Background()))

	proj, ok := loader.Get("proj")
	assert.True(t, ok)
	assert.Equal(t, "project", proj.Source)

	home, ok := loader.Get("home")
	assert.True(t, ok)
	assert.Equal(t, "user", home.Source)
}

func TestLoader_AgentsPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .agents/skills/ directory (generic standard)
	agentsSkills := filepath.Join(tmpDir, ".agents", "skills")
	assert.NoError(t, os.MkdirAll(agentsSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(agentsSkills, "shared.md"), []byte(`---
name: shared
description: A shared cross-tool skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, ok := loader.Get("shared")
	assert.True(t, ok)
	assert.Equal(t, "A shared cross-tool skill.", s.Description)
}

func TestLoader_AgentsPathPriority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create same skill in .dive/skills/ and .agents/skills/
	diveSkills := filepath.Join(tmpDir, ".dive", "skills")
	agentsSkills := filepath.Join(tmpDir, ".agents", "skills")
	assert.NoError(t, os.MkdirAll(diveSkills, 0755))
	assert.NoError(t, os.MkdirAll(agentsSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(diveSkills, "overlap.md"), []byte(`---
name: overlap
description: From .dive (should win).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(agentsSkills, "overlap.md"), []byte(`---
name: overlap
description: From .agents (should lose).
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, ok := loader.Get("overlap")
	assert.True(t, ok)
	assert.Equal(t, "From .dive (should win).", s.Description)
}

func TestLoader_DisableAgentsPaths(t *testing.T) {
	tmpDir := t.TempDir()

	agentsSkills := filepath.Join(tmpDir, ".agents", "skills")
	assert.NoError(t, os.MkdirAll(agentsSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(agentsSkills, "agents-skill.md"), []byte(`---
name: agents-skill
description: Agents skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir:         tmpDir,
		HomeDir:            "/nonexistent",
		DisableAgentsPaths: true,
	})
	assert.NoError(t, loader.Load(context.Background()))

	_, ok := loader.Get("agents-skill")
	assert.False(t, ok)
}

func TestLoader_CommandDirectorySkillMd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a COMMAND.md in a subdirectory of commands/
	cmdDir := filepath.Join(tmpDir, ".dive", "commands", "review")
	assert.NoError(t, os.MkdirAll(cmdDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(cmdDir, "COMMAND.md"), []byte(`---
description: Review code.
---

Review the code.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	s, ok := loader.Get("review")
	assert.True(t, ok)
	assert.Equal(t, "Review code.", s.Description)
}
