package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	goModuleEntryLimit = 2048
	goModuleCountLimit = 32
	goModuleDepthLimit = 4
)

func goDevelopmentSnapshot(workspaceDir string) (string, bool) {
	goMod := filepath.Join(workspaceDir, "go.mod")
	goWork := filepath.Join(workspaceDir, "go.work")
	hasMod := regularFileExists(goMod)
	hasWork := regularFileExists(goWork)
	nestedModules, truncated := countNestedGoModules(workspaceDir)
	if !hasMod && !hasWork && nestedModules == 0 {
		return "", false
	}

	var content strings.Builder
	content.WriteString("Go development context (read-only heuristic; advisory):")
	switch {
	case hasMod && hasWork:
		content.WriteString("\n- workspace root: Go module with go.work")
	case hasMod:
		content.WriteString("\n- workspace root: Go module (go.mod)")
	case hasWork:
		content.WriteString("\n- workspace root: Go workspace (go.work)")
	default:
		content.WriteString("\n- workspace root: contains Go modules below the root")
	}
	if version := goDirective(goWork, "go"); version == "" {
		if version = goDirective(goMod, "go"); version != "" {
			fmt.Fprintf(&content, "\n- declared Go version: %s", version)
		}
	} else {
		fmt.Fprintf(&content, "\n- declared Go version: %s", version)
	}
	if nestedModules > 0 {
		fmt.Fprintf(&content, "\n- nested Go module manifests observed: %d", nestedModules)
		content.WriteString("; root-module checks do not cover them automatically")
	}
	if truncated {
		content.WriteString("\n- module topology scan: incomplete after reaching a fixed safety bound")
	}
	if _, limited := detectWorkspaceBoundary(workspaceDir); limited {
		content.WriteString("\n- scope: this CLI workspace is below the Git root; parent modules are outside tool access")
	}
	content.WriteString("\nRecommended completion loop for affected Go modules:")
	content.WriteString("\n1. Format changed Go files with gofmt.")
	content.WriteString("\n2. Run go test ./... and go vet ./... in each affected module.")
	content.WriteString("\n3. Run go test -race ./... when concurrency behavior changed.")
	content.WriteString("\n4. Update generators or source definitions before editing generated Go output.")
	if hasMod || hasWork {
		content.WriteString("\nWhen dependency or workspace membership changes, run the appropriate go mod tidy or go work sync command and review manifest diffs.")
	}
	content.WriteString("\nThese are workflow suggestions, not evidence that any check ran or passed.")
	return content.String(), true
}

func goDirective(path, name string) string {
	data, ok := readRegularFilePrefix(path)
	if !ok {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name && safeGoVersion(fields[1]) {
			return fields[1]
		}
	}
	return ""
}

func safeGoVersion(value string) bool {
	if value == "" || len(value) > 16 {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func countNestedGoModules(root string) (count int, truncated bool) {
	root = filepath.Clean(root)
	entries := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		entries++
		if entries > goModuleEntryLimit {
			truncated = true
			return fs.SkipAll
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		depth := len(strings.Split(relative, string(filepath.Separator)))
		if entry.IsDir() {
			if depth > goModuleDepthLimit || ignoredGoScanDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && entry.Name() == "go.mod" && filepath.Dir(path) != root {
			count++
			if count == goModuleCountLimit {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})
	return count, truncated
}

func ignoredGoScanDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", ".idea", ".vscode":
		return true
	default:
		return false
	}
}
