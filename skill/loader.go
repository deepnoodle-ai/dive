package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Logger is an optional interface for logging during skill loading.
// Implementations receive debug messages about skill discovery and
// warning messages about malformed or inaccessible skill files.
type Logger interface {
	// Debug logs informational messages about skill loading progress.
	Debug(msg string, args ...any)

	// Warn logs warning messages about non-fatal issues like malformed files.
	Warn(msg string, args ...any)
}

// LoaderOptions configures skill discovery and loading.
//
// The loader searches for skills in multiple locations with a defined
// priority order. Project-level skills take precedence over user-level
// skills, and the first skill found with a given name wins.
//
// Example:
//
//	opts := LoaderOptions{
//	    ProjectDir: "/path/to/project",
//	    Logger:     myLogger,
//	}
//	loader := NewLoader(opts)
//	loader.LoadSkills()
type LoaderOptions struct {
	// ProjectDir is the base directory for project-level skill discovery.
	// Skills are searched in ProjectDir/.dive/skills/ and ProjectDir/.claude/skills/.
	// If empty, defaults to the current working directory.
	ProjectDir string

	// HomeDir is the base directory for user-level skill discovery.
	// Skills are searched in HomeDir/.dive/skills/ and HomeDir/.claude/skills/.
	// If empty, defaults to os.UserHomeDir().
	HomeDir string

	// Logger receives debug and warning messages during skill loading.
	// If nil, no logging occurs. Useful for debugging skill discovery issues.
	Logger Logger

	// AdditionalPaths specifies extra directories to search for skills.
	// These paths are searched after the default paths, so skills in
	// default locations take precedence.
	AdditionalPaths []string

	// DisableClaudePaths prevents searching Claude-specific skill paths:
	//   - ProjectDir/.claude/skills/
	//   - HomeDir/.claude/skills/
	// Set this to true if you only want Dive-specific skill paths.
	DisableClaudePaths bool

	// DisableDivePaths prevents searching Dive-specific skill paths:
	//   - ProjectDir/.dive/skills/
	//   - HomeDir/.dive/skills/
	// Set this to true if you only want Claude-specific skill paths.
	DisableDivePaths bool
}

// Loader discovers and loads skills from configured paths.
//
// The Loader scans skill directories for SKILL.md files and standalone .md files,
// parses their content, and maintains a map of available skills. It is safe to
// call LoadSkills multiple times to reload skills (e.g., after file changes).
//
// Thread Safety: The Loader is NOT safe for concurrent use. If skills need to
// be accessed from multiple goroutines, external synchronization is required.
type Loader struct {
	opts   LoaderOptions
	skills map[string]*Skill
}

// NewLoader creates a new skill loader with the given options.
//
// After creating a loader, call LoadSkills to discover and parse skills from
// the configured paths. The loader starts with no skills loaded.
//
// Example:
//
//	loader := NewLoader(LoaderOptions{
//	    ProjectDir: "/path/to/project",
//	})
//	if err := loader.LoadSkills(); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Loaded %d skills\n", loader.SkillCount())
func NewLoader(opts LoaderOptions) *Loader {
	return &Loader{
		opts:   opts,
		skills: make(map[string]*Skill),
	}
}

// LoadSkills scans all configured paths and loads skills.
//
// Skills are loaded in priority order (first match wins for duplicate names):
//  1. ProjectDir/.dive/skills/
//  2. ProjectDir/.claude/skills/
//  3. HomeDir/.dive/skills/
//  4. HomeDir/.claude/skills/
//  5. AdditionalPaths (in order specified)
//
// This method clears any previously loaded skills before scanning, making it
// safe to call multiple times to reload skills after file changes.
//
// For each skills directory, the loader looks for:
//   - Subdirectories containing a SKILL.md file
//   - Standalone .md files (skill name derived from filename)
//
// Malformed skill files are logged (if a Logger is configured) but do not
// cause the overall load to fail. Missing directories are silently ignored.
//
// Returns an error only if the search paths cannot be determined (e.g.,
// current directory is inaccessible).
func (l *Loader) LoadSkills() error {
	l.skills = make(map[string]*Skill)

	paths, err := l.getSearchPaths()
	if err != nil {
		return fmt.Errorf("getting search paths: %w", err)
	}

	for _, searchPath := range paths {
		if err := l.loadSkillsFromPath(searchPath); err != nil {
			l.logWarn("failed to load skills from %s: %v", searchPath, err)
		}
	}

	return nil
}

// GetSkill retrieves a skill by its exact name.
//
// The name must match exactly (case-sensitive) the skill's Name field.
// Returns the skill and true if found, or nil and false if not found.
//
// Example:
//
//	if skill, ok := loader.GetSkill("code-reviewer"); ok {
//	    fmt.Println(skill.Instructions)
//	}
func (l *Loader) GetSkill(name string) (*Skill, bool) {
	skill, ok := l.skills[name]
	return skill, ok
}

// ListSkills returns all loaded skills, sorted alphabetically by name.
//
// The returned slice is a new allocation; modifying it does not affect
// the loader's internal state. The Skill pointers reference the same
// Skill objects held by the loader.
//
// Returns an empty slice if no skills are loaded.
func (l *Loader) ListSkills() []*Skill {
	skills := make([]*Skill, 0, len(l.skills))
	for _, skill := range l.skills {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}

// ListSkillNames returns the names of all loaded skills, sorted alphabetically.
//
// This is useful for displaying available skills to users or for
// constructing error messages with available options.
//
// Returns an empty slice if no skills are loaded.
func (l *Loader) ListSkillNames() []string {
	names := make([]string, 0, len(l.skills))
	for name := range l.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SkillCount returns the number of loaded skills.
//
// This is equivalent to len(loader.ListSkills()) but more efficient
// as it doesn't allocate a slice.
func (l *Loader) SkillCount() int {
	return len(l.skills)
}

// getSearchPaths returns the paths to search for skills in priority order.
// Priority (first match wins for same-named skills):
// 1. ./.dive/skills/ (project Dive)
// 2. ./.claude/skills/ (project Claude)
// 3. ~/.dive/skills/ (personal Dive)
// 4. ~/.claude/skills/ (personal Claude)
// 5. AdditionalPaths
func (l *Loader) getSearchPaths() ([]string, error) {
	var paths []string

	projectDir := l.opts.ProjectDir
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting current directory: %w", err)
		}
	}

	homeDir := l.opts.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			l.logWarn("could not determine home directory: %v", err)
			homeDir = ""
		}
	}

	// Project-level paths (highest priority)
	if !l.opts.DisableDivePaths {
		paths = append(paths, filepath.Join(projectDir, ".dive", "skills"))
	}
	if !l.opts.DisableClaudePaths {
		paths = append(paths, filepath.Join(projectDir, ".claude", "skills"))
	}

	// User-level paths
	if homeDir != "" {
		if !l.opts.DisableDivePaths {
			paths = append(paths, filepath.Join(homeDir, ".dive", "skills"))
		}
		if !l.opts.DisableClaudePaths {
			paths = append(paths, filepath.Join(homeDir, ".claude", "skills"))
		}
	}

	// Additional custom paths
	paths = append(paths, l.opts.AdditionalPaths...)

	return paths, nil
}

// loadSkillsFromPath loads skills from a single directory path.
//
// It scans the directory for:
//   - Subdirectories containing a SKILL.md file (directory-based skills)
//   - Files ending in .md (standalone skill files)
//
// Nested directories beyond the immediate children are not scanned.
// Non-.md files are ignored. Missing directories return nil (not an error).
func (l *Loader) loadSkillsFromPath(searchPath string) error {
	entries, err := os.ReadDir(searchPath)
	if os.IsNotExist(err) {
		l.logDebug("skill path does not exist: %s", searchPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Look for SKILL.md in subdirectory
			skillPath := filepath.Join(searchPath, entry.Name(), "SKILL.md")
			l.loadSkillFile(skillPath)
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			// Load standalone skill file
			skillPath := filepath.Join(searchPath, entry.Name())
			l.loadSkillFile(skillPath)
		}
	}

	return nil
}

// loadSkillFile attempts to load a single skill file.
//
// If the file does not exist, the function returns silently.
// If parsing fails, a warning is logged but the function continues.
// If a skill with the same name already exists, the new skill is ignored
// (first-loaded wins).
func (l *Loader) loadSkillFile(filePath string) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return
	}

	skill, err := ParseSkillFile(filePath)
	if err != nil {
		l.logWarn("failed to parse skill file %s: %v", filePath, err)
		return
	}

	// First skill with a given name wins
	if _, exists := l.skills[skill.Name]; exists {
		l.logDebug("skill %s already loaded, ignoring %s", skill.Name, filePath)
		return
	}

	l.skills[skill.Name] = skill
	l.logDebug("loaded skill %s from %s", skill.Name, filePath)
}

func (l *Loader) logDebug(format string, args ...any) {
	if l.opts.Logger != nil {
		l.opts.Logger.Debug(fmt.Sprintf(format, args...))
	}
}

func (l *Loader) logWarn(format string, args ...any) {
	if l.opts.Logger != nil {
		l.opts.Logger.Warn(fmt.Sprintf(format, args...))
	}
}
