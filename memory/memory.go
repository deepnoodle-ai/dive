package memory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Memory represents the memory system for Dive, inspired by Claude Code's CLAUDE.md
type Memory struct {
	files   []MemoryFile
	content map[string]string
	imports map[string][]string
}

// MemoryFile represents a single memory file
type MemoryFile struct {
	Path     string
	Type     MemoryType
	Content  string
	Priority int // Higher priority = loaded first
}

// MemoryType represents the type of memory file
type MemoryType int

const (
	EnterpriseMemory MemoryType = iota
	UserMemory
	ProjectMemory
	LocalMemory
	NestedMemory // For DIVE.md files in subdirectories
)

// NewMemory creates a new memory manager
func NewMemory() *Memory {
	return &Memory{
		files:   []MemoryFile{},
		content: make(map[string]string),
		imports: make(map[string][]string),
	}
}

// Load loads all memory files from the hierarchy
func (m *Memory) Load() error {
	// Load enterprise memory (highest priority)
	if err := m.loadEnterpriseMemory(); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Load user memory
	if err := m.loadUserMemory(); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Load project memories (recursively from cwd up to root)
	if err := m.loadProjectMemories(); err != nil {
		return err
	}

	// Process imports for all loaded memories
	if err := m.processImports(); err != nil {
		return err
	}

	return nil
}

// loadEnterpriseMemory loads the enterprise-level DIVE.md
func (m *Memory) loadEnterpriseMemory() error {
	paths := getEnterpriseMemoryPaths()
	for _, path := range paths {
		if content, err := m.readMemoryFile(path); err == nil {
			m.files = append(m.files, MemoryFile{
				Path:     path,
				Type:     EnterpriseMemory,
				Content:  content,
				Priority: 100,
			})
			m.content[path] = content
			return nil
		}
	}
	return os.ErrNotExist
}

// loadUserMemory loads the user-level DIVE.md
func (m *Memory) loadUserMemory() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(homeDir, ".dive", "DIVE.md")
	content, err := m.readMemoryFile(path)
	if err != nil {
		return err
	}

	m.files = append(m.files, MemoryFile{
		Path:     path,
		Type:     UserMemory,
		Content:  content,
		Priority: 90,
	})
	m.content[path] = content

	return nil
}

// loadProjectMemories loads project memories recursively from cwd
func (m *Memory) loadProjectMemories() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Walk up from cwd to root, loading memories
	priority := 80
	for dir := cwd; dir != "/" && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		// Check for DIVE.md in this directory
		memoryPath := filepath.Join(dir, "DIVE.md")
		if content, err := m.readMemoryFile(memoryPath); err == nil {
			m.files = append(m.files, MemoryFile{
				Path:     memoryPath,
				Type:     ProjectMemory,
				Content:  content,
				Priority: priority,
			})
			m.content[memoryPath] = content
			priority--
		}

		// Check for .dive/DIVE.md
		diveMemoryPath := filepath.Join(dir, ".dive", "DIVE.md")
		if content, err := m.readMemoryFile(diveMemoryPath); err == nil {
			m.files = append(m.files, MemoryFile{
				Path:     diveMemoryPath,
				Type:     ProjectMemory,
				Content:  content,
				Priority: priority,
			})
			m.content[diveMemoryPath] = content
			priority--
		}

		// Check for DIVE.local.md (deprecated but supported)
		localMemoryPath := filepath.Join(dir, "DIVE.local.md")
		if content, err := m.readMemoryFile(localMemoryPath); err == nil {
			m.files = append(m.files, MemoryFile{
				Path:     localMemoryPath,
				Type:     LocalMemory,
				Content:  content,
				Priority: priority - 10, // Local has lower priority
			})
			m.content[localMemoryPath] = content
		}
	}

	// Also discover nested DIVE.md files in subdirectories for lazy loading
	if err := m.discoverNestedMemories(cwd); err != nil {
		return err
	}

	return nil
}

// discoverNestedMemories finds DIVE.md files in subdirectories
func (m *Memory) discoverNestedMemories(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and node_modules
		if info.IsDir() && (strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules") {
			return filepath.SkipDir
		}

		// Check if this is a DIVE.md file
		if !info.IsDir() && info.Name() == "DIVE.md" {
			// Skip if it's in the root (already loaded)
			if filepath.Dir(path) == root {
				return nil
			}

			// Store for lazy loading
			m.files = append(m.files, MemoryFile{
				Path:     path,
				Type:     NestedMemory,
				Content:  "", // Will be loaded when needed
				Priority: 0,  // Lowest priority
			})
		}

		return nil
	})
}

// processImports processes @import directives in memory files
func (m *Memory) processImports() error {
	importRegex := regexp.MustCompile(`(?m)(?:^|[^` + "`" + `])@([^\s]+)`)
	maxDepth := 5

	for path, content := range m.content {
		if err := m.processFileImports(path, content, importRegex, 0, maxDepth); err != nil {
			return err
		}
	}

	return nil
}

// processFileImports recursively processes imports for a single file
func (m *Memory) processFileImports(basePath, content string, importRegex *regexp.Regexp, depth, maxDepth int) error {
	if depth >= maxDepth {
		return fmt.Errorf("maximum import depth exceeded for %s", basePath)
	}

	matches := importRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		importPath := match[1]

		// Resolve the import path
		resolvedPath := m.resolveImportPath(basePath, importPath)

		// Track the import
		if m.imports[basePath] == nil {
			m.imports[basePath] = []string{}
		}
		m.imports[basePath] = append(m.imports[basePath], resolvedPath)

		// Load the imported file if not already loaded
		if _, exists := m.content[resolvedPath]; !exists {
			importContent, err := m.readMemoryFile(resolvedPath)
			if err != nil {
				continue // Skip if can't read
			}

			m.content[resolvedPath] = importContent

			// Recursively process imports in the imported file
			if err := m.processFileImports(resolvedPath, importContent, importRegex, depth+1, maxDepth); err != nil {
				return err
			}
		}
	}

	return nil
}

// resolveImportPath resolves an import path relative to a base path
func (m *Memory) resolveImportPath(basePath, importPath string) string {
	// Handle home directory expansion
	if strings.HasPrefix(importPath, "~/") {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, importPath[2:])
	}

	// Handle absolute paths
	if filepath.IsAbs(importPath) {
		return importPath
	}

	// Handle relative paths
	baseDir := filepath.Dir(basePath)
	return filepath.Clean(filepath.Join(baseDir, importPath))
}

// readMemoryFile reads a memory file from disk
func (m *Memory) readMemoryFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var content strings.Builder
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return content.String(), nil
}

// GetCombinedMemory returns all memory content combined
func (m *Memory) GetCombinedMemory() string {
	var combined strings.Builder

	// Sort files by priority (highest first)
	for i := len(m.files) - 1; i >= 0; i-- {
		for _, file := range m.files {
			if file.Priority == 100-i*10 && file.Content != "" {
				combined.WriteString(fmt.Sprintf("# Memory from: %s\n\n", file.Path))
				combined.WriteString(file.Content)
				combined.WriteString("\n\n")

				// Include imports for this file
				if imports, ok := m.imports[file.Path]; ok && len(imports) > 0 {
					for _, imp := range imports {
						if importContent, ok := m.content[imp]; ok {
							combined.WriteString(fmt.Sprintf("# Imported from: %s\n\n", imp))
							combined.WriteString(importContent)
							combined.WriteString("\n\n")
						}
					}
				}
			}
		}
	}

	return combined.String()
}

// AddMemory adds a new memory entry
func (m *Memory) AddMemory(content string, memoryType MemoryType) error {
	var targetPath string

	switch memoryType {
	case UserMemory:
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		targetPath = filepath.Join(homeDir, ".dive", "DIVE.md")
	case ProjectMemory:
		targetPath = filepath.Join(".dive", "DIVE.md")
	default:
		return fmt.Errorf("unsupported memory type for adding: %v", memoryType)
	}

	// Ensure directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Append to file
	file, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Add timestamp and content
	_, err = file.WriteString(fmt.Sprintf("\n\n%s\n", content))
	return err
}

// GetMemoryFiles returns list of loaded memory files
func (m *Memory) GetMemoryFiles() []MemoryFile {
	return m.files
}

// Helper function to get enterprise memory paths
func getEnterpriseMemoryPaths() []string {
	switch {
	case strings.Contains(strings.ToLower(os.Getenv("OS")), "windows"):
		return []string{`C:\ProgramData\Dive\DIVE.md`}
	case strings.Contains(strings.ToLower(os.Getenv("HOME")), "/users"):
		// macOS
		return []string{"/Library/Application Support/Dive/DIVE.md"}
	default:
		// Linux/Unix
		return []string{"/etc/dive/DIVE.md"}
	}
}

// ReadMemoryFile reads a memory file from disk (public method for external use)
func (m *Memory) ReadMemoryFile(path string) (string, error) {
	return m.readMemoryFile(path)
}

// AddCustomMemory adds a custom memory entry from an external source
func (m *Memory) AddCustomMemory(path, content string) {
	m.files = append(m.files, MemoryFile{
		Path:     path,
		Type:     LocalMemory,
		Content:  content,
		Priority: 10, // Lower priority than discovered files
	})
	m.content[path] = content
}

// InitProjectMemory creates a new DIVE.md file for the project
func InitProjectMemory() error {
	template := `# Dive Project Memory

This file stores important project information, conventions, and frequently used commands.
It is automatically loaded by Dive when you run commands in this directory or its subdirectories.

## Project Overview

<!-- Describe your project here -->

## Key Commands

<!-- List frequently used commands -->
- Build: ` + "`go build ./...`" + `
- Test: ` + "`go test ./...`" + `
- Lint: ` + "`golangci-lint run`" + `

## Code Style

<!-- Document your code style preferences -->
- Use descriptive variable names
- Add comments for complex logic
- Follow Go best practices

## Architecture Patterns

<!-- Describe important patterns used in this project -->

## Team Conventions

<!-- Document team-specific conventions -->

## Imports

You can import other files using @path/to/file syntax:
<!-- @README.md -->
<!-- @docs/architecture.md -->
`

	// Check if DIVE.md already exists
	paths := []string{"DIVE.md", ".dive/DIVE.md"}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("memory file already exists at %s", path)
		}
	}

	// Create .dive directory
	if err := os.MkdirAll(".dive", 0755); err != nil {
		return err
	}

	// Write the template
	return os.WriteFile(".dive/DIVE.md", []byte(template), 0644)
}