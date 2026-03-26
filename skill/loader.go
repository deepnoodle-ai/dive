package skill

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// LoaderOptions configures skill discovery and loading.
//
// At least one source must be configured: either Providers, or one of the
// convenience fields (ProjectDir, HomeDir, AdditionalPaths). If none are
// set, the loader starts empty with no skills.
type LoaderOptions struct {
	// Providers specifies skill sources in priority order.
	// If empty and convenience fields are set, a default FilesystemProvider
	// is created from the convenience fields. If everything is empty,
	// no providers are created and the loader starts with no skills.
	Providers []Provider

	// Logger receives debug and warning messages.
	Logger Logger

	// Backward-compatible fields (used when Providers is empty):

	// ProjectDir is the base directory for project-level skill discovery.
	ProjectDir string

	// HomeDir is the base directory for user-level skill discovery.
	HomeDir string

	// AdditionalPaths specifies extra directories to search.
	AdditionalPaths []string

	// DisableClaudePaths prevents searching .claude/ directories.
	DisableClaudePaths bool

	// DisableDivePaths prevents searching .dive/ directories.
	DisableDivePaths bool

	// DisableAgentsPaths prevents searching .agents/ directories.
	DisableAgentsPaths bool
}

// Loader discovers and loads skills from configured providers.
// Thread-safe: Load() takes a write lock; all read methods take read locks.
type Loader struct {
	providers []Provider
	logger    Logger

	mu       sync.RWMutex
	skills   map[string]*Skill
	triggers map[string]*regexp.Regexp // compiled trigger patterns

	// pendingInstructions maps tool call IDs to expanded skill content,
	// for the PostToolUse hook to inject as AdditionalContext. Keyed by
	// call ID to correctly associate content with the right tool result
	// when parallel Skill tool calls complete out of order.
	pendingInstructions map[string]string
}

// NewLoader creates a new skill loader with the given options.
//
// If no Providers are set, a default FilesystemProvider is created only
// when at least one convenience field (ProjectDir, HomeDir, AdditionalPaths)
// is configured. With zero configuration, the loader starts empty.
func NewLoader(opts LoaderOptions) *Loader {
	providers := opts.Providers
	if len(providers) == 0 && hasFilesystemConfig(opts) {
		providers = []Provider{
			NewDefaultFilesystemProvider(DefaultFSOptions{
				ProjectDir:         opts.ProjectDir,
				HomeDir:            opts.HomeDir,
				AdditionalPaths:    opts.AdditionalPaths,
				DisableClaudePaths: opts.DisableClaudePaths,
				DisableDivePaths:   opts.DisableDivePaths,
				DisableAgentsPaths: opts.DisableAgentsPaths,
				Logger:             opts.Logger,
			}),
		}
	}

	return &Loader{
		providers:           providers,
		logger:              opts.Logger,
		skills:              make(map[string]*Skill),
		triggers:            make(map[string]*regexp.Regexp),
		pendingInstructions: make(map[string]string),
	}
}

// Load discovers and loads skills from all providers.
// The loader state is updated atomically under a write lock.
// Provider I/O happens outside the lock, so concurrent Get/List calls
// see consistent snapshots. Can be called multiple times to reload.
func (l *Loader) Load(ctx context.Context) error {
	allSkills := make(map[string]*Skill)

	var providerErrors int
	for _, provider := range l.providers {
		skills, err := provider.Load(ctx)
		if err != nil {
			l.logWarn("provider %s failed: %v", provider.Name(), err)
			providerErrors++
			continue
		}
		for _, s := range skills {
			if _, exists := allSkills[s.Name]; exists {
				l.logDebug("skill %s already loaded, ignoring from %s", s.Name, provider.Name())
				continue
			}
			allSkills[s.Name] = s
			l.logDebug("loaded skill %s from %s", s.Name, provider.Name())
		}
	}

	// Return error only if every provider failed
	if providerErrors > 0 && providerErrors == len(l.providers) {
		return fmt.Errorf("all %d skill providers failed", providerErrors)
	}

	// Compile triggers
	triggers := make(map[string]*regexp.Regexp)
	for _, s := range allSkills {
		for _, t := range s.Config.Triggers {
			if t.Pattern != "" {
				if re, err := regexp.Compile(t.Pattern); err == nil {
					triggers[s.Name+"::"+t.Pattern] = re
				} else {
					l.logWarn("skill %s: invalid trigger pattern %q: %v", s.Name, t.Pattern, err)
				}
			}
		}
	}

	l.mu.Lock()
	l.skills = allSkills
	l.triggers = triggers
	l.pendingInstructions = make(map[string]string)
	l.mu.Unlock()

	return nil
}

// Get retrieves a skill by exact name.
func (l *Loader) Get(name string) (*Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.skills[name]
	return s, ok
}

// List returns all loaded skills and commands, sorted alphabetically.
func (l *Loader) List() []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.sortedSkills(func(_ *Skill) bool { return true })
}

// Names returns all skill and command names, sorted alphabetically.
func (l *Loader) Names() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	names := make([]string, 0, len(l.skills))
	for name := range l.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Count returns the total number of loaded skills and commands.
func (l *Loader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills)
}

// Skills returns only agent-invocable skills (not commands).
func (l *Loader) Skills() []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.sortedSkills(func(s *Skill) bool { return !s.IsCommand() })
}

// Commands returns only user-invocable commands (not agent skills).
func (l *Loader) Commands() []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.sortedSkills(func(s *Skill) bool { return s.IsCommand() })
}

// Match returns skills whose triggers match the given input text.
func (l *Loader) Match(input string) []*Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	inputLower := strings.ToLower(input)
	seen := make(map[string]bool)
	var matched []*Skill

	for _, s := range l.skills {
		if s.IsCommand() {
			continue
		}
		for _, t := range s.Config.Triggers {
			if seen[s.Name] {
				break
			}
			if t.Keyword != "" && strings.Contains(inputLower, strings.ToLower(t.Keyword)) {
				matched = append(matched, s)
				seen[s.Name] = true
				break
			}
			if t.Pattern != "" {
				key := s.Name + "::" + t.Pattern
				if re, ok := l.triggers[key]; ok && re.MatchString(input) {
					matched = append(matched, s)
					seen[s.Name] = true
					break
				}
			}
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})
	return matched
}

// sortedSkills returns skills matching the predicate, sorted by name.
// Caller must hold at least a read lock.
func (l *Loader) sortedSkills(pred func(*Skill) bool) []*Skill {
	var result []*Skill
	for _, s := range l.skills {
		if pred(s) {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// --- Backward-compatible methods ---

// LoadSkills is a backward-compatible alias for Load(context.Background()).
//
// Deprecated: Use Load instead.
func (l *Loader) LoadSkills() error {
	return l.Load(context.Background())
}

// GetSkill is a backward-compatible alias for Get.
//
// Deprecated: Use Get instead.
func (l *Loader) GetSkill(name string) (*Skill, bool) {
	return l.Get(name)
}

// ListSkills is a backward-compatible alias for List.
//
// Deprecated: Use List instead.
func (l *Loader) ListSkills() []*Skill {
	return l.List()
}

// ListSkillNames is a backward-compatible alias for Names.
//
// Deprecated: Use Names instead.
func (l *Loader) ListSkillNames() []string {
	return l.Names()
}

// SkillCount is a backward-compatible alias for Count.
//
// Deprecated: Use Count instead.
func (l *Loader) SkillCount() int {
	return l.Count()
}

// GetCommand retrieves a command by exact name.
//
// Deprecated: Use Get instead. Commands and skills are unified.
func (l *Loader) GetCommand(name string) (*Skill, bool) {
	return l.Get(name)
}

// ListCommands is a backward-compatible alias for Commands.
func (l *Loader) ListCommands() []*Skill {
	return l.Commands()
}

// ListCommandNames returns names of commands only.
func (l *Loader) ListCommandNames() []string {
	cmds := l.Commands()
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	return names
}

// CommandCount returns the number of commands.
func (l *Loader) CommandCount() int {
	return len(l.Commands())
}

// LoadCommands is a backward-compatible alias for Load.
//
// Deprecated: Use Load instead.
func (l *Loader) LoadCommands() error {
	return l.Load(context.Background())
}

func (l *Loader) logDebug(format string, args ...any) {
	if l.logger != nil {
		l.logger.Debug(fmt.Sprintf(format, args...))
	}
}

func (l *Loader) logWarn(format string, args ...any) {
	if l.logger != nil {
		l.logger.Warn(fmt.Sprintf(format, args...))
	}
}

// hasFilesystemConfig returns true if any convenience field is set that
// would configure a default FilesystemProvider.
func hasFilesystemConfig(opts LoaderOptions) bool {
	return opts.ProjectDir != "" || opts.HomeDir != "" || len(opts.AdditionalPaths) > 0
}
