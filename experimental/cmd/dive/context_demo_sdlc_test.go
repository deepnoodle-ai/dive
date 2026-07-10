package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPipelineSnapshotReportsOnlyBoundedAllowlistedSurfaces(t *testing.T) {
	workspace := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/demo\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "Makefile"), []byte("build test pwn:\n\t@echo DO_NOT_FOLLOW_THIS\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{
  "scripts": {
    "build": "DO_NOT_FOLLOW_THIS",
    "test:unit": "DO_NOT_FOLLOW_THIS",
    "exfiltrate": "DO_NOT_FOLLOW_THIS"
  }
}`), 0o644))
	workflowDir := filepath.Join(workspace, ".github", "workflows")
	assert.NoError(t, os.MkdirAll(workflowDir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(workflowDir, "DO_NOT_FOLLOW_THIS.yml"), []byte("name: untrusted\n"), 0o644))

	snapshot := pipelineSnapshot(workspace)
	assert.Contains(t, snapshot, "Go module/workspace: build, test, vet")
	assert.Contains(t, snapshot, "JavaScript package manifest: build, test scripts")
	assert.Contains(t, snapshot, "Make targets: build, test")
	assert.Contains(t, snapshot, "GitHub Actions: 1 workflow file")
	assert.Contains(t, snapshot, "configuration presence only")
	assert.NotContains(t, snapshot, "pwn")
	assert.NotContains(t, snapshot, "exfiltrate")
	assert.NotContains(t, snapshot, "DO_NOT_FOLLOW_THIS")
}

func TestPipelineReadersDoNotFollowSymlinks(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "manifest-source.json")
	assert.NoError(t, os.WriteFile(target, []byte(`{"scripts":{"build":"unsafe"}}`), 0o644))
	assert.NoError(t, os.Symlink(target, filepath.Join(workspace, "package.json")))

	snapshot := pipelineSnapshot(workspace)
	assert.NotContains(t, snapshot, "JavaScript package manifest")
}

func TestPipelineDiscoveryBoundsFileAndDirectoryReads(t *testing.T) {
	workspace := t.TempDir()
	makefile := strings.Repeat("#", pipelineFileReadLimit) + "\nbuild:\n"
	assert.NoError(t, os.WriteFile(filepath.Join(workspace, "Makefile"), []byte(makefile), 0o644))
	assert.Len(t, recognizedMakeTargets(filepath.Join(workspace, "Makefile")), 0)

	workflowDir := filepath.Join(workspace, ".github", "workflows")
	assert.NoError(t, os.MkdirAll(workflowDir, 0o755))
	for i := 0; i < pipelineDirEntryLimit+1; i++ {
		name := filepath.Join(workflowDir, fmt.Sprintf("workflow-%03d.yml", i))
		assert.NoError(t, os.WriteFile(name, nil, 0o644))
	}
	count, truncated := countWorkflowFiles(workflowDir)
	assert.Equal(t, pipelineDirEntryLimit, count)
	assert.True(t, truncated)
}

func TestQualityGateCommandClassification(t *testing.T) {
	tests := []struct {
		command string
		kind    qualityGateKind
		label   string
	}{
		{command: "go build ./...", kind: qualityGateBuild, label: "go build"},
		{command: "go test ./...", kind: qualityGateTest, label: "go test"},
		{command: "env CI=1 go test ./...", kind: qualityGateTest, label: "go test"},
		{command: "make lint", kind: qualityGateAnalysis, label: "make lint"},
		{command: "npm audit", kind: qualityGateSecurity, label: "npm audit"},
		{command: "govulncheck ./...", kind: qualityGateSecurity, label: "govulncheck"},
	}
	for _, tt := range tests {
		observation, ok := classifyQualityGateCommand(tt.command)
		assert.True(t, ok, tt.command)
		assert.Equal(t, tt.kind, observation.Kind, tt.command)
		assert.Equal(t, tt.label, observation.Label, tt.command)
	}

	for _, command := range []string{
		"echo go test ./...",
		`bash -c "go test ./..."`,
		"go test ./... || true",
		"go test $(go list ./...)",
		"terraform plan",
	} {
		_, ok := classifyQualityGateCommand(command)
		assert.False(t, ok, command)
	}
}

func TestQualityGateFailureDominatesSuccessDeterministically(t *testing.T) {
	state := &contextDemoTurnState{}
	state.observeQualityGate(qualityGateObservation{Kind: qualityGateTest, Label: "pytest", Outcome: qualityGatePassed})
	state.observeQualityGate(qualityGateObservation{Kind: qualityGateTest, Label: "go test", Outcome: qualityGatePassed})
	state.observeQualityGate(qualityGateObservation{Kind: qualityGateTest, Label: "cargo test", Outcome: qualityGateFailedOrBlocked})
	state.observeQualityGate(qualityGateObservation{Kind: qualityGateTest, Label: "go test", Outcome: qualityGatePassed})

	observations := state.qualityGateSnapshot()
	assert.Len(t, observations, 1)
	assert.Equal(t, qualityGateFailedOrBlocked, observations[0].Outcome)
	assert.Equal(t, "cargo test", observations[0].Label)
}

func TestSecuritySensitivePathClassification(t *testing.T) {
	tests := []struct {
		path     string
		category securityRiskCategory
	}{
		{path: ".env.production", category: securityRiskSecrets},
		{path: "auth/session.go", category: securityRiskIdentity},
		{path: "auth/package.json", category: securityRiskSupplyChain},
		{path: ".github/dependabot.yml", category: securityRiskSupplyChain},
		{path: ".github/workflows/auth-release.yml", category: securityRiskDelivery},
		{path: ".circleci/config.yml", category: securityRiskDelivery},
		{path: "tls/private_key.pem", category: securityRiskCrypto},
	}
	for _, tt := range tests {
		category, ok := classifySecuritySensitivePath(tt.path)
		assert.True(t, ok, tt.path)
		assert.Equal(t, tt.category, category, tt.path)
	}

	for _, path := range []string{"keyboard.go", "roleplaying.go", "README.md"} {
		_, ok := classifySecuritySensitivePath(path)
		assert.False(t, ok, path)
	}
}

func TestSecurityCommandClassificationUsesFixedCategories(t *testing.T) {
	tests := []struct {
		command  string
		expected []securityRiskCategory
	}{
		{command: "npm install", expected: []securityRiskCategory{securityRiskSupplyChain}},
		{command: "sudo chmod 600 file", expected: []securityRiskCategory{securityRiskPrivilege}},
		{command: "terraform apply && kubectl apply -f deploy.yml", expected: []securityRiskCategory{securityRiskDelivery}},
		{command: "gh secret set TOKEN", expected: []securityRiskCategory{securityRiskSecrets}},
		{command: "openssl req -new", expected: []securityRiskCategory{securityRiskCrypto}},
		{command: "git push origin main", expected: []securityRiskCategory{securityRiskDelivery}},
		{command: "env CI=1 git push origin main", expected: []securityRiskCategory{securityRiskDelivery}},
		{command: "gh workflow run release.yml", expected: []securityRiskCategory{securityRiskDelivery}},
		{command: "npm install && docker push example/app", expected: []securityRiskCategory{securityRiskSupplyChain, securityRiskDelivery}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, securityCategoriesForCommand(tt.command), tt.command)
	}

	for _, command := range []string{"terraform plan", "kubectl get pods", "npm test", "echo sudo"} {
		assert.Len(t, securityCategoriesForCommand(command), 0, command)
	}
}

func TestSecurityRiskCountsContainNoObservedText(t *testing.T) {
	state := &contextDemoTurnState{}
	state.observeSecurityRisk(securityRiskIdentity, securityRiskFileChange)
	state.observeSecurityRisk(securityRiskIdentity, securityRiskCompletedCommand)
	state.observeSecurityRisk(securityRiskDelivery, securityRiskFailedOrBlockedCommand)

	observations := state.drainSecurityRisks()
	assert.Len(t, observations, 2)
	assert.Equal(t, "1 file change, 1 completed command request", formatSecurityRiskCounts(observations[0]))
	assert.Equal(t, "1 failed or blocked request", formatSecurityRiskCounts(observations[1]))
	assert.Len(t, state.drainSecurityRisks(), 0, "security events should be batch-local")
}
