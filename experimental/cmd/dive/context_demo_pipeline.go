package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

const (
	pipelineFileReadLimit = 64 * 1024
	pipelineDirEntryLimit = 256
)

func pipelineContextDemoHook(workspaceDir string) dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		reminder, err := dive.NewContextReminder("delivery-pipeline", pipelineSnapshot(workspaceDir))
		if err != nil {
			return err
		}
		return hctx.PinReminder(reminder)
	}
}

func pipelineSnapshot(workspaceDir string) string {
	var surfaces []string
	if regularFileExists(filepath.Join(workspaceDir, "go.mod")) || regularFileExists(filepath.Join(workspaceDir, "go.work")) {
		surfaces = append(surfaces, "Go module/workspace: build, test, vet")
	}
	if regularFileExists(filepath.Join(workspaceDir, "Cargo.toml")) {
		surfaces = append(surfaces, "Rust package: build, test, clippy")
	}
	if regularFileExists(filepath.Join(workspaceDir, "pyproject.toml")) || regularFileExists(filepath.Join(workspaceDir, "requirements.txt")) {
		surfaces = append(surfaces, "Python project: test and static-analysis tooling may apply")
	}
	if regularFileExists(filepath.Join(workspaceDir, "package.json")) {
		surface := "JavaScript package manifest"
		if scripts := recognizedPackageScriptKinds(filepath.Join(workspaceDir, "package.json")); len(scripts) > 0 {
			surface += ": " + strings.Join(scripts, ", ") + " scripts"
		}
		surfaces = append(surfaces, surface)
	}
	if targets := recognizedMakeTargets(filepath.Join(workspaceDir, "Makefile")); len(targets) > 0 {
		surfaces = append(surfaces, "Make targets: "+strings.Join(targets, ", "))
	}
	if regularFileExists(filepath.Join(workspaceDir, "Taskfile.yml")) || regularFileExists(filepath.Join(workspaceDir, "Taskfile.yaml")) || regularFileExists(filepath.Join(workspaceDir, "Justfile")) {
		surfaces = append(surfaces, "Task runner configuration detected")
	}
	if count, truncated := countWorkflowFiles(filepath.Join(workspaceDir, ".github", "workflows")); count > 0 || truncated {
		label := fmt.Sprintf("GitHub Actions: %d workflow file%s", count, pluralSuffix(count))
		if truncated {
			label += " in bounded sample"
		}
		surfaces = append(surfaces, label)
	}
	if regularFileExists(filepath.Join(workspaceDir, ".gitlab-ci.yml")) || regularFileExists(filepath.Join(workspaceDir, "Jenkinsfile")) {
		surfaces = append(surfaces, "Additional CI configuration detected")
	}
	if regularFileExists(filepath.Join(workspaceDir, "Dockerfile")) || regularFileExists(filepath.Join(workspaceDir, "docker-compose.yml")) || regularFileExists(filepath.Join(workspaceDir, "compose.yml")) {
		surfaces = append(surfaces, "Container build/development configuration detected")
	}
	if regularFileExists(filepath.Join(workspaceDir, ".github", "dependabot.yml")) || regularFileExists(filepath.Join(workspaceDir, ".github", "dependabot.yaml")) ||
		regularFileExists(filepath.Join(workspaceDir, "renovate.json")) || regularFileExists(filepath.Join(workspaceDir, ".snyk")) ||
		regularFileExists(filepath.Join(workspaceDir, ".semgrep.yml")) {
		surfaces = append(surfaces, "Dependency/security automation detected")
	}

	var content strings.Builder
	content.WriteString("Detected repository delivery surfaces (read-only heuristic; verify repository docs and CI before acting):")
	if len(surfaces) == 0 {
		content.WriteString("\n- No recognized build or CI surface at the workspace root")
	} else {
		for _, surface := range surfaces {
			content.WriteString("\n- ")
			content.WriteString(surface)
		}
	}
	content.WriteString("\nThis map reports configuration presence only. It does not establish that any gate ran or passed.")
	return content.String()
}

func recognizedMakeTargets(path string) []string {
	data, ok := readRegularFilePrefix(path)
	if !ok {
		return nil
	}
	allow := []string{"build", "test", "test-race", "vet", "lint", "check", "fmt-check", "security", "audit", "scan"}
	found := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || line[0] == '\t' || line[0] == ' ' || strings.HasPrefix(line, "#") {
			continue
		}
		targets, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		for _, target := range strings.Fields(targets) {
			for _, allowed := range allow {
				if target == allowed {
					found[allowed] = true
				}
			}
		}
	}
	var targets []string
	for _, allowed := range allow {
		if found[allowed] {
			targets = append(targets, allowed)
		}
	}
	return targets
}

func recognizedPackageScriptKinds(path string) []string {
	data, ok := readRegularFilePrefix(path)
	if !ok {
		return nil
	}
	var manifest struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	kinds := make(map[string]bool)
	for script := range manifest.Scripts {
		for _, kind := range []string{"build", "test", "lint", "check", "typecheck", "security", "audit"} {
			if targetMatches(strings.ToLower(script), kind) {
				kinds[kind] = true
			}
		}
	}
	var result []string
	for kind := range kinds {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

func countWorkflowFiles(path string) (count int, truncated bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, false
	}
	dir, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer dir.Close()
	entries, err := dir.ReadDir(pipelineDirEntryLimit + 1)
	if err != nil && err != io.EOF {
		return 0, false
	}
	if len(entries) > pipelineDirEntryLimit {
		entries = entries[:pipelineDirEntryLimit]
		truncated = true
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".yml" || ext == ".yaml" {
				count++
			}
		}
	}
	return count, truncated
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func readRegularFilePrefix(path string) ([]byte, bool) {
	if !regularFileExists(path) {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, pipelineFileReadLimit))
	return data, err == nil
}
