package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// qualityGateKind classifies concrete build and test evidence tracked by the
// verification preset; it is not a separately selectable context demo.
type qualityGateKind string

const (
	qualityGateBuild    qualityGateKind = "build"
	qualityGateTest     qualityGateKind = "test"
	qualityGateAnalysis qualityGateKind = "static analysis"
	qualityGateSecurity qualityGateKind = "security"
)

var qualityGateOrder = []qualityGateKind{
	qualityGateBuild,
	qualityGateTest,
	qualityGateAnalysis,
	qualityGateSecurity,
}

type qualityGateOutcome string

const (
	qualityGatePassed          qualityGateOutcome = "passed"
	qualityGateFailedOrBlocked qualityGateOutcome = "failed or blocked"
)

type qualityGateObservation struct {
	Kind    qualityGateKind
	Label   string
	Outcome qualityGateOutcome
}

func qualityGateCollectorHook(outcome qualityGateOutcome) dive.PostToolUseHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		recordQualityGate(hctx, outcome)
		return nil
	}
}

func qualityGateCollectorFailureHook() dive.PostToolUseFailureHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		recordQualityGate(hctx, qualityGateFailedOrBlocked)
		return nil
	}
}

func recordQualityGate(hctx *dive.HookContext, outcome qualityGateOutcome) {
	state := contextDemoState(hctx)
	if state == nil || hctx.Call == nil || hctx.Call.Name != "Bash" {
		return
	}
	command := firstString(toolInput(hctx.Call), "command")
	observation, ok := classifyQualityGateCommand(command)
	if !ok {
		return
	}
	observation.Outcome = outcome
	state.observeQualityGate(observation)
}

func (s *contextDemoTurnState) observeQualityGate(observation qualityGateObservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.qualityGates == nil {
		s.qualityGates = make(map[qualityGateKind]qualityGateObservation)
	}
	current, ok := s.qualityGates[observation.Kind]
	if !ok ||
		(current.Outcome == qualityGatePassed && observation.Outcome == qualityGateFailedOrBlocked) ||
		(current.Outcome == observation.Outcome && observation.Label < current.Label) {
		s.qualityGates[observation.Kind] = observation
	}
}

func (s *contextDemoTurnState) qualityGateSnapshot() []qualityGateObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	observations := make([]qualityGateObservation, 0, len(s.qualityGates))
	for _, kind := range qualityGateOrder {
		if observation, ok := s.qualityGates[kind]; ok {
			observations = append(observations, observation)
		}
	}
	return observations
}

func verificationGateReminderHook(runtime contextDemoRuntime) dive.PreIterationHook {
	return func(_ context.Context, hctx *dive.HookContext) error {
		state := contextDemoState(hctx)
		if state == nil {
			return nil
		}
		observations := state.qualityGateSnapshot()
		if len(observations) == 0 {
			return nil
		}
		var content strings.Builder
		content.WriteString("Observed quality gates this response (Bash tool outcomes, not proof of coverage):")
		for _, observation := range observations {
			fmt.Fprintf(&content, "\n- %s: %s (%s)", observation.Kind, observation.Outcome, observation.Label)
		}
		content.WriteString("\nUse failed or blocked gates as unresolved evidence; passing gates establish only the scope their command actually covered.")
		reminder, err := dive.NewContextReminder("verification-gates", content.String())
		if err != nil {
			return err
		}
		return runtime.appendChangedModelOnly(hctx, reminder)
	}
}

func classifyQualityGateCommand(command string) (qualityGateObservation, bool) {
	fields, ok := finalShellInvocation(command)
	if !ok {
		return qualityGateObservation{}, false
	}
	kind, label, ok := classifyQualityGateInvocation(fields)
	return qualityGateObservation{Kind: kind, Label: label}, ok
}

func classifyQualityGateInvocation(fields []string) (qualityGateKind, string, bool) {
	if len(fields) == 0 {
		return "", "", false
	}
	executable := filepath.Base(fields[0])
	arg := func(index int) string { return shellArgument(fields, index) }
	switch executable {
	case "go":
		switch arg(1) {
		case "build":
			return qualityGateBuild, "go build", true
		case "test":
			return qualityGateTest, "go test", true
		case "vet":
			return qualityGateAnalysis, "go vet", true
		}
	case "pytest":
		return qualityGateTest, "pytest", true
	case "python", "python3":
		if arg(1) == "-m" && arg(2) == "pytest" {
			return qualityGateTest, "python pytest", true
		}
	case "cargo":
		switch arg(1) {
		case "build", "check":
			return qualityGateBuild, "cargo build/check", true
		case "test":
			return qualityGateTest, "cargo test", true
		case "clippy":
			return qualityGateAnalysis, "cargo clippy", true
		case "audit":
			return qualityGateSecurity, "cargo audit", true
		}
	case "swift":
		switch arg(1) {
		case "build":
			return qualityGateBuild, "swift build", true
		case "test":
			return qualityGateTest, "swift test", true
		}
	case "dotnet":
		switch arg(1) {
		case "build":
			return qualityGateBuild, "dotnet build", true
		case "test":
			return qualityGateTest, "dotnet test", true
		}
	case "mvn":
		switch arg(1) {
		case "package", "verify":
			return qualityGateBuild, "maven build", true
		case "test":
			return qualityGateTest, "maven test", true
		}
	case "gradle", "gradlew":
		switch arg(1) {
		case "build", "assemble":
			return qualityGateBuild, "gradle build", true
		case "test":
			return qualityGateTest, "gradle test", true
		}
	case "make":
		return classifyQualityTarget(arg(1), "make")
	case "npm", "pnpm", "yarn":
		if executable == "yarn" && arg(1) == "npm" && arg(2) == "audit" {
			return qualityGateSecurity, "yarn audit", true
		}
		target := arg(1)
		if target == "run" {
			target = arg(2)
		}
		return classifyQualityTarget(target, executable)
	case "xcodebuild":
		switch arg(1) {
		case "build", "build-for-testing":
			return qualityGateBuild, "xcodebuild build", true
		case "test", "test-without-building":
			return qualityGateTest, "xcodebuild test", true
		}
	case "docker":
		if arg(1) == "build" || arg(1) == "buildx" {
			return qualityGateBuild, "docker build", true
		}
		if arg(1) == "scout" {
			return qualityGateSecurity, "docker scout", true
		}
	case "ruff":
		if arg(1) == "check" {
			return qualityGateAnalysis, "ruff check", true
		}
	case "mypy", "tsc", "golangci-lint":
		return qualityGateAnalysis, executable, true
	case "govulncheck", "gosec", "trivy", "semgrep", "bandit", "pip-audit", "osv-scanner", "snyk":
		return qualityGateSecurity, executable, true
	}
	return "", "", false
}

func classifyQualityTarget(target, runner string) (qualityGateKind, string, bool) {
	for _, candidate := range []struct {
		kind qualityGateKind
		name string
	}{
		{qualityGateSecurity, "security"},
		{qualityGateSecurity, "audit"},
		{qualityGateSecurity, "scan"},
		{qualityGateTest, "test"},
		{qualityGateAnalysis, "lint"},
		{qualityGateAnalysis, "check"},
		{qualityGateAnalysis, "vet"},
		{qualityGateAnalysis, "typecheck"},
		{qualityGateBuild, "build"},
		{qualityGateBuild, "package"},
		{qualityGateBuild, "release"},
	} {
		if targetMatches(target, candidate.name) {
			return candidate.kind, runner + " " + candidate.name, true
		}
	}
	return "", "", false
}

func targetMatches(target, name string) bool {
	return target == name || strings.HasPrefix(target, name+":") || strings.HasPrefix(target, name+"-") ||
		strings.HasSuffix(target, ":"+name) || strings.HasSuffix(target, "-"+name)
}
