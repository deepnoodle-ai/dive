package slashcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Logger is an optional interface for logging during command loading.
type Logger interface {
	// Debug logs informational messages about command loading progress.
	Debug(msg string, args ...any)

	// Warn logs warning messages about non-fatal issues like malformed files.
	Warn(msg string, args ...any)
}

// LoaderOptions configures command discovery and loading.
//
// The loader searches for commands in multiple locations with a defined
// priority order. Project-level commands take precedence over user-level
// commands, and the first command found with a given name wins.
type LoaderOptions struct {
	// ProjectDir is the base directory for project-level command discovery.
	// Commands are searched in ProjectDir/.dive/commands/ and ProjectDir/.claude/commands/.
	// If empty, defaults to the current working directory.
	ProjectDir string

	// HomeDir is the base directory for user-level command discovery.
	// Commands are searched in HomeDir/.dive/commands/ and HomeDir/.claude/commands/.
	// If empty, defaults to os.UserHomeDir().
	HomeDir string

	// Logger receives debug and warning messages during command loading.
	Logger Logger

	// AdditionalPaths specifies extra directories to search for commands.
	// These paths are searched after the default paths.
	AdditionalPaths []string

	// DisableClaudePaths prevents searching Claude-specific command paths.
	DisableClaudePaths bool

	// DisableDivePaths prevents searching Dive-specific command paths.
	DisableDivePaths bool
}

// Loader discovers and loads slash commands from configured paths.
//
// The Loader scans command directories for markdown files, parses their
// content, and maintains a map of available commands. It is safe to call
// LoadCommands multiple times to reload commands (e.g., after file changes).
//
// Thread Safety: The Loader is NOT safe for concurrent use. If commands need
// to be accessed from multiple goroutines, external synchronization is required.
//
// Example:
//
//	loader := slashcmd.NewLoader(slashcmd.LoaderOptions{
//	    ProjectDir: "/path/to/project",
//	})
//	if err := loader.LoadCommands(); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Loaded %d commands\n", loader.CommandCount())
type Loader struct {
	opts     LoaderOptions
	commands map[string]*Command
}

// NewLoader creates a new command loader with the given options.
//
// After creating a loader, call LoadCommands to discover and parse commands
// from the configured paths. The loader starts with no commands loaded.
func NewLoader(opts LoaderOptions) *Loader {
	return &Loader{
		opts:     opts,
		commands: make(map[string]*Command),
	}
}

// LoadCommands scans all configured paths and loads commands.
//
// Commands are loaded in priority order (first match wins for duplicate names):
//  1. ProjectDir/.dive/commands/
//  2. ProjectDir/.claude/commands/
//  3. HomeDir/.dive/commands/
//  4. HomeDir/.claude/commands/
//  5. AdditionalPaths (in order specified)
//
// This method clears any previously loaded commands before scanning.
func (l *Loader) LoadCommands() error {
	l.commands = make(map[string]*Command)

	paths, err := l.getSearchPaths()
	if err != nil {
		return fmt.Errorf("getting search paths: %w", err)
	}

	for _, searchPath := range paths {
		source := l.determineSource(searchPath)
		if err := l.loadCommandsFromPath(searchPath, source); err != nil {
			l.logWarn("failed to load commands from %s: %v", searchPath, err)
		}
	}

	return nil
}

// GetCommand retrieves a command by its exact name.
//
// The name must match exactly (case-sensitive) the command's Name field.
// Returns the command and true if found, or nil and false if not found.
//
// Example:
//
//	if cmd, ok := loader.GetCommand("review"); ok {
//	    expanded := cmd.ExpandArguments("**/*.go")
//	    fmt.Println(expanded)
//	}
func (l *Loader) GetCommand(name string) (*Command, bool) {
	cmd, ok := l.commands[name]
	return cmd, ok
}

// ListCommands returns all loaded commands, sorted alphabetically by name.
//
// The returned slice is a new allocation; modifying it does not affect
// the loader's internal state. Returns an empty slice if no commands are loaded.
func (l *Loader) ListCommands() []*Command {
	commands := make([]*Command, 0, len(l.commands))
	for _, cmd := range l.commands {
		commands = append(commands, cmd)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	return commands
}

// ListCommandNames returns the names of all loaded commands, sorted alphabetically.
func (l *Loader) ListCommandNames() []string {
	names := make([]string, 0, len(l.commands))
	for name := range l.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CommandCount returns the number of loaded commands.
func (l *Loader) CommandCount() int {
	return len(l.commands)
}

// getSearchPaths returns the paths to search for commands in priority order.
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
		paths = append(paths, filepath.Join(projectDir, ".dive", "commands"))
	}
	if !l.opts.DisableClaudePaths {
		paths = append(paths, filepath.Join(projectDir, ".claude", "commands"))
	}

	// User-level paths
	if homeDir != "" {
		if !l.opts.DisableDivePaths {
			paths = append(paths, filepath.Join(homeDir, ".dive", "commands"))
		}
		if !l.opts.DisableClaudePaths {
			paths = append(paths, filepath.Join(homeDir, ".claude", "commands"))
		}
	}

	// Additional custom paths
	paths = append(paths, l.opts.AdditionalPaths...)

	return paths, nil
}

// determineSource determines if a path is project-level or user-level.
func (l *Loader) determineSource(searchPath string) string {
	homeDir := l.opts.HomeDir
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}

	if homeDir != "" && strings.HasPrefix(searchPath, homeDir) {
		return "user"
	}
	return "project"
}

// loadCommandsFromPath loads commands from a single directory path.
func (l *Loader) loadCommandsFromPath(searchPath, source string) error {
	entries, err := os.ReadDir(searchPath)
	if os.IsNotExist(err) {
		l.logDebug("command path does not exist: %s", searchPath)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Look for COMMAND.md in subdirectory
			cmdPath := filepath.Join(searchPath, entry.Name(), "COMMAND.md")
			l.loadCommandFile(cmdPath, source)
		} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			// Load standalone command file
			cmdPath := filepath.Join(searchPath, entry.Name())
			l.loadCommandFile(cmdPath, source)
		}
	}

	return nil
}

// loadCommandFile attempts to load a single command file.
func (l *Loader) loadCommandFile(filePath, source string) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return
	}

	cmd, err := ParseCommandFile(filePath)
	if err != nil {
		l.logWarn("failed to parse command file %s: %v", filePath, err)
		return
	}

	// Set the source
	cmd.Source = source

	// First command with a given name wins
	if _, exists := l.commands[cmd.Name]; exists {
		l.logDebug("command %s already loaded, ignoring %s", cmd.Name, filePath)
		return
	}

	l.commands[cmd.Name] = cmd
	l.logDebug("loaded command %s from %s", cmd.Name, filePath)
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
