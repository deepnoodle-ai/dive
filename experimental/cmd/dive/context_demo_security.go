package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/deepnoodle-ai/dive"
)

type securityRiskCategory string

const (
	securityRiskSecrets     securityRiskCategory = "secrets and credentials"
	securityRiskIdentity    securityRiskCategory = "identity and access"
	securityRiskSupplyChain securityRiskCategory = "software supply chain"
	securityRiskDelivery    securityRiskCategory = "deployment and CI"
	securityRiskCrypto      securityRiskCategory = "cryptography and trust"
	securityRiskPrivilege   securityRiskCategory = "privilege changes"
)

var securityRiskOrder = []securityRiskCategory{
	securityRiskSecrets,
	securityRiskIdentity,
	securityRiskSupplyChain,
	securityRiskDelivery,
	securityRiskCrypto,
	securityRiskPrivilege,
}

type securityRiskObservation struct {
	Category                 securityRiskCategory
	FileChanges              int
	CompletedCommandRequests int
	FailedOrBlockedRequests  int
}

type securityRiskEvent uint8

const (
	securityRiskFileChange securityRiskEvent = iota
	securityRiskCompletedCommand
	securityRiskFailedOrBlockedCommand
)

func securityAwarenessSuccessHook() dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil || hctx.Call == nil {
			return nil
		}
		input := toolInput(hctx.Call)
		switch hctx.Call.Name {
		case "Write", "Edit":
			if category, ok := classifySecuritySensitivePath(firstString(input, "file_path", "path")); ok {
				state.observeSecurityRisk(category, securityRiskFileChange)
			}
		case "Bash":
			for _, category := range securityCategoriesForCommand(firstString(input, "command")) {
				state.observeSecurityRisk(category, securityRiskCompletedCommand)
			}
		}
		return nil
	}
}

func securityAwarenessFailureHook() dive.PostToolUseFailureHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil || hctx.Call == nil || hctx.Call.Name != "Bash" {
			return nil
		}
		for _, category := range securityCategoriesForCommand(firstString(toolInput(hctx.Call), "command")) {
			state.observeSecurityRisk(category, securityRiskFailedOrBlockedCommand)
		}
		return nil
	}
}

func (s *contextDemoTurnState) observeSecurityRisk(category securityRiskCategory, event securityRiskEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batchSecurityRisk == nil {
		s.batchSecurityRisk = make(map[securityRiskCategory]securityRiskObservation)
	}
	observation := s.batchSecurityRisk[category]
	observation.Category = category
	switch event {
	case securityRiskFileChange:
		observation.FileChanges++
	case securityRiskCompletedCommand:
		observation.CompletedCommandRequests++
	case securityRiskFailedOrBlockedCommand:
		observation.FailedOrBlockedRequests++
	}
	s.batchSecurityRisk[category] = observation
}

func (s *contextDemoTurnState) drainSecurityRisks() []securityRiskObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	observations := make([]securityRiskObservation, 0, len(s.batchSecurityRisk))
	for _, category := range securityRiskOrder {
		if observation, ok := s.batchSecurityRisk[category]; ok {
			observations = append(observations, observation)
		}
	}
	s.batchSecurityRisk = nil
	return observations
}

func securityAwarenessReminderHook() dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		observations := state.drainSecurityRisks()
		if len(observations) == 0 {
			return nil
		}
		var content strings.Builder
		content.WriteString("Security review trigger (advisory; not a vulnerability finding):")
		for _, observation := range observations {
			fmt.Fprintf(&content, "\n- %s: %s", observation.Category, formatSecurityRiskCounts(observation))
		}
		content.WriteString("\nBefore completion, inspect the relevant diff and command effects for secret exposure, least privilege, dependency provenance, and deployment scope. Run applicable security checks and require user approval for privileged or deployment actions. This reminder is not authorization or enforcement.")
		reminder, err := dive.NewOperatorReminder("security-review", content.String())
		if err != nil {
			return err
		}
		return hctx.AppendReminder(reminder, dive.ModelOnly)
	}
}

func formatSecurityRiskCounts(observation securityRiskObservation) string {
	var counts []string
	if observation.FileChanges > 0 {
		counts = append(counts, fmt.Sprintf("%d file change%s", observation.FileChanges, pluralSuffix(observation.FileChanges)))
	}
	if observation.CompletedCommandRequests > 0 {
		counts = append(counts, fmt.Sprintf("%d completed command request%s", observation.CompletedCommandRequests, pluralSuffix(observation.CompletedCommandRequests)))
	}
	if observation.FailedOrBlockedRequests > 0 {
		counts = append(counts, fmt.Sprintf("%d failed or blocked request%s", observation.FailedOrBlockedRequests, pluralSuffix(observation.FailedOrBlockedRequests)))
	}
	return strings.Join(counts, ", ")
}

func classifySecuritySensitivePath(path string) (securityRiskCategory, bool) {
	normalized := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(normalized)
	if base == ".env" || strings.HasPrefix(base, ".env.") || hasAnyPathToken(normalized, "secret", "secrets", "credential", "credentials", "apikey") {
		return securityRiskSecrets, true
	}
	if isDependencyManifest(base) || strings.HasPrefix(normalized, ".github/dependabot.") ||
		strings.Contains(normalized, "/.github/dependabot.") || base == "renovate.json" {
		return securityRiskSupplyChain, true
	}
	if strings.Contains(normalized, "/.github/workflows/") || strings.HasPrefix(normalized, ".github/workflows/") ||
		base == ".gitlab-ci.yml" || base == "jenkinsfile" || strings.HasPrefix(normalized, ".circleci/") ||
		strings.Contains(normalized, "/.circleci/") ||
		base == "dockerfile" || filepath.Ext(base) == ".tf" || hasAnyPathToken(normalized, "terraform", "k8s", "kubernetes", "helm") {
		return securityRiskDelivery, true
	}
	if filepath.Ext(base) == ".pem" || filepath.Ext(base) == ".crt" || filepath.Ext(base) == ".key" ||
		hasAnyPathToken(normalized, "crypto", "tls", "cert", "certificate", "privatekey") ||
		strings.Contains(normalized, "private_key") || strings.Contains(normalized, "private-key") {
		return securityRiskCrypto, true
	}
	if hasAnyPathToken(normalized, "auth", "oauth", "oidc", "permission", "permissions", "policy", "policies", "role", "roles", "session") {
		return securityRiskIdentity, true
	}
	return "", false
}

func isDependencyManifest(base string) bool {
	for _, name := range []string{
		"go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"cargo.toml", "cargo.lock", "requirements.txt", "requirements-dev.txt", "pyproject.toml",
		"poetry.lock", "uv.lock", "gemfile", "gemfile.lock", "composer.json", "composer.lock",
	} {
		if base == name {
			return true
		}
	}
	return false
}

func hasAnyPathToken(path string, candidates ...string) bool {
	tokens := strings.FieldsFunc(path, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, token := range tokens {
		for _, candidate := range candidates {
			if token == candidate {
				return true
			}
		}
	}
	return false
}

func securityCategoriesForCommand(command string) []securityRiskCategory {
	invocations, _ := shellInvocations(command)
	seen := make(map[securityRiskCategory]bool)
	for _, fields := range invocations {
		if category, ok := classifySecurityInvocation(fields); ok {
			seen[category] = true
		}
	}
	var categories []securityRiskCategory
	for _, category := range securityRiskOrder {
		if seen[category] {
			categories = append(categories, category)
		}
	}
	return categories
}

func classifySecurityInvocation(fields []string) (securityRiskCategory, bool) {
	if len(fields) == 0 {
		return "", false
	}
	executable := filepath.Base(fields[0])
	arg := func(index int) string { return shellArgument(fields, index) }
	switch executable {
	case "go":
		if arg(1) == "get" || (arg(1) == "mod" && targetIn(arg(2), "tidy", "download", "edit", "vendor")) {
			return securityRiskSupplyChain, true
		}
	case "npm", "pnpm", "yarn":
		if targetIn(arg(1), "ci", "install", "add", "update", "upgrade", "remove", "uninstall") {
			return securityRiskSupplyChain, true
		}
	case "pip", "pip3":
		if targetIn(arg(1), "install", "uninstall") {
			return securityRiskSupplyChain, true
		}
	case "python", "python3":
		if arg(1) == "-m" && arg(2) == "pip" && targetIn(arg(3), "install", "uninstall") {
			return securityRiskSupplyChain, true
		}
	case "cargo":
		if targetIn(arg(1), "add", "update", "install") {
			return securityRiskSupplyChain, true
		}
	case "sudo", "chmod", "chown":
		return securityRiskPrivilege, true
	case "terraform":
		if targetIn(arg(1), "apply", "destroy", "import") {
			return securityRiskDelivery, true
		}
	case "kubectl":
		if arg(1) == "create" && arg(2) == "secret" {
			return securityRiskSecrets, true
		}
		if targetIn(arg(1), "apply", "create", "delete", "patch", "replace") {
			return securityRiskDelivery, true
		}
	case "helm":
		if targetIn(arg(1), "install", "upgrade", "uninstall", "rollback") {
			return securityRiskDelivery, true
		}
	case "docker":
		if arg(1) == "login" {
			return securityRiskSecrets, true
		}
		if arg(1) == "push" {
			return securityRiskDelivery, true
		}
	case "git":
		if arg(1) == "push" {
			return securityRiskDelivery, true
		}
	case "gh":
		if arg(1) == "secret" {
			return securityRiskSecrets, true
		}
		if arg(1) == "release" || (arg(1) == "pr" && arg(2) == "merge") ||
			(arg(1) == "workflow" && arg(2) == "run") {
			return securityRiskDelivery, true
		}
	case "openssl", "ssh-keygen":
		return securityRiskCrypto, true
	}
	return "", false
}

func targetIn(target string, candidates ...string) bool {
	for _, candidate := range candidates {
		if target == candidate {
			return true
		}
	}
	return false
}
