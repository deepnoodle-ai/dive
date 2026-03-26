package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Provider loads skills from a source. Implementations handle filesystem,
// HTTP, or custom backends.
type Provider interface {
	// Name identifies this provider for logging and debugging.
	Name() string

	// Load returns all skills available from this provider.
	Load(ctx context.Context) ([]*Skill, error)
}

// FilesystemOptions configures a FilesystemProvider.
type FilesystemOptions struct {
	// Paths to search for skills and commands.
	Paths []string

	// UserPaths lists paths that should mark loaded skills with Source="user".
	// Paths not in this list are marked Source="project".
	UserPaths []string

	// Logger receives debug/warning messages.
	Logger Logger
}

// filesystemProvider implements Provider for local filesystem scanning.
type filesystemProvider struct {
	opts      FilesystemOptions
	userPaths map[string]bool
}

// NewFilesystemProvider creates a provider that loads skills from the given paths.
// Each path is scanned for subdirectories containing SKILL.md or COMMAND.md,
// and standalone .md files.
func NewFilesystemProvider(opts FilesystemOptions) Provider {
	up := make(map[string]bool, len(opts.UserPaths))
	for _, p := range opts.UserPaths {
		up[p] = true
	}
	return &filesystemProvider{opts: opts, userPaths: up}
}

func (p *filesystemProvider) Name() string {
	return "filesystem"
}

func (p *filesystemProvider) Load(_ context.Context) ([]*Skill, error) {
	var skills []*Skill
	for _, searchPath := range p.opts.Paths {
		loaded, err := p.loadFromPath(searchPath)
		if err != nil {
			p.logWarn("failed to load from %s: %v", searchPath, err)
			continue
		}
		skills = append(skills, loaded...)
	}
	return skills, nil
}

func (p *filesystemProvider) loadFromPath(searchPath string) ([]*Skill, error) {
	entries, err := os.ReadDir(searchPath)
	if os.IsNotExist(err) {
		p.logDebug("path does not exist: %s", searchPath)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	source := "project"
	if p.userPaths[searchPath] {
		source = "user"
	}

	var skills []*Skill
	for _, entry := range entries {
		if entry.IsDir() {
			// Look for SKILL.md or COMMAND.md in subdirectory
			for _, marker := range []string{"SKILL.md", "COMMAND.md"} {
				markerPath := filepath.Join(searchPath, entry.Name(), marker)
				if s := p.loadFile(markerPath); s != nil {
					s.Source = source
					skills = append(skills, s)
					break
				}
			}
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			skillPath := filepath.Join(searchPath, entry.Name())
			if s := p.loadFile(skillPath); s != nil {
				s.Source = source
				skills = append(skills, s)
			}
		}
	}

	return skills, nil
}

func (p *filesystemProvider) loadFile(filePath string) *Skill {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	s, err := ParseFile(filePath)
	if err != nil {
		p.logWarn("failed to parse %s: %v", filePath, err)
		return nil
	}

	return s
}

func (p *filesystemProvider) logDebug(format string, args ...any) {
	if p.opts.Logger != nil {
		p.opts.Logger.Debug(fmt.Sprintf(format, args...))
	}
}

func (p *filesystemProvider) logWarn(format string, args ...any) {
	if p.opts.Logger != nil {
		p.opts.Logger.Warn(fmt.Sprintf(format, args...))
	}
}

// DefaultFSOptions configures the default filesystem provider.
type DefaultFSOptions struct {
	ProjectDir         string
	HomeDir            string
	AdditionalPaths    []string
	DisableClaudePaths bool
	DisableDivePaths   bool
	DisableAgentsPaths bool
	Logger             Logger
}

// NewDefaultFilesystemProvider creates a provider that scans standard skill
// and command directories in priority order:
//  1. projectDir/.dive/skills/
//  2. projectDir/.dive/commands/
//  3. projectDir/.agents/skills/ (generic, tool-agnostic standard)
//  4. projectDir/.claude/skills/
//  5. projectDir/.claude/commands/
//  6. homeDir/.dive/skills/
//  7. homeDir/.dive/commands/
//  8. homeDir/.agents/skills/
//  9. homeDir/.claude/skills/
//  10. homeDir/.claude/commands/
//  11. additionalPaths...
//
func NewDefaultFilesystemProvider(opts DefaultFSOptions) Provider {
	projectDir := opts.ProjectDir
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			projectDir = "."
		}
	}

	homeDir := opts.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			homeDir = ""
		}
	}

	var paths []string
	var userPaths []string

	// Project-level paths
	if !opts.DisableDivePaths {
		paths = append(paths,
			filepath.Join(projectDir, ".dive", "skills"),
			filepath.Join(projectDir, ".dive", "commands"),
		)
	}
	if !opts.DisableAgentsPaths {
		paths = append(paths,
			filepath.Join(projectDir, ".agents", "skills"),
		)
	}
	if !opts.DisableClaudePaths {
		paths = append(paths,
			filepath.Join(projectDir, ".claude", "skills"),
			filepath.Join(projectDir, ".claude", "commands"),
		)
	}

	// User-level paths
	if homeDir != "" {
		if !opts.DisableDivePaths {
			homeDiveSkills := filepath.Join(homeDir, ".dive", "skills")
			homeDiveCommands := filepath.Join(homeDir, ".dive", "commands")
			paths = append(paths, homeDiveSkills, homeDiveCommands)
			userPaths = append(userPaths, homeDiveSkills, homeDiveCommands)
		}
		if !opts.DisableAgentsPaths {
			homeAgentsSkills := filepath.Join(homeDir, ".agents", "skills")
			paths = append(paths, homeAgentsSkills)
			userPaths = append(userPaths, homeAgentsSkills)
		}
		if !opts.DisableClaudePaths {
			homeClaudeSkills := filepath.Join(homeDir, ".claude", "skills")
			homeClaudeCommands := filepath.Join(homeDir, ".claude", "commands")
			paths = append(paths, homeClaudeSkills, homeClaudeCommands)
			userPaths = append(userPaths, homeClaudeSkills, homeClaudeCommands)
		}
	}

	// Additional custom paths
	paths = append(paths, opts.AdditionalPaths...)

	return NewFilesystemProvider(FilesystemOptions{
		Paths:     paths,
		UserPaths: userPaths,
		Logger:    opts.Logger,
	})
}
