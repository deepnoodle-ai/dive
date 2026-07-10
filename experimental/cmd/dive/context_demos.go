package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

const (
	contextDemoStateKey  = "dive-cli-context-demo-state"
	contextDemoItemLimit = 12
)

var contextDemoNames = []string{
	"workspace", "sources", "verification", "recovery",
	"pipeline", "quality", "security",
}

type contextDemoSelection struct {
	workspace    bool
	sources      bool
	verification bool
	recovery     bool
	pipeline     bool
	quality      bool
	security     bool
}

func (s contextDemoSelection) empty() bool {
	return !s.workspace && !s.sources && !s.verification && !s.recovery &&
		!s.pipeline && !s.quality && !s.security
}

// parseContextDemoNames accepts repeatable values and comma-separated groups so
// both --context-demo workspace --context-demo sources and
// --context-demo workspace,sources are convenient at the shell.
func parseContextDemoNames(specs []string) (contextDemoSelection, error) {
	var selection contextDemoSelection
	for _, spec := range specs {
		for _, rawName := range strings.Split(spec, ",") {
			name := strings.ToLower(strings.TrimSpace(rawName))
			switch name {
			case "all":
				selection.workspace = true
				selection.sources = true
				selection.verification = true
				selection.recovery = true
				selection.pipeline = true
				selection.quality = true
				selection.security = true
			case "workspace":
				selection.workspace = true
			case "sources":
				selection.sources = true
			case "verification":
				selection.verification = true
			case "recovery":
				selection.recovery = true
			case "pipeline":
				selection.pipeline = true
			case "quality":
				selection.quality = true
			case "security":
				selection.security = true
			case "":
				return contextDemoSelection{}, fmt.Errorf("context demo name cannot be empty")
			default:
				return contextDemoSelection{}, fmt.Errorf(
					"unknown context demo %q: expected one of all, %s",
					name,
					strings.Join(contextDemoNames, ", "),
				)
			}
		}
	}
	return selection, nil
}

func applyContextDemoAgentOptions(agentOpts *dive.AgentOptions, workspaceDir string, selection contextDemoSelection) {
	if selection.empty() {
		return
	}

	if selection.sources || selection.verification || selection.quality || selection.security {
		// Install turn-local state before the first iteration. Tool hooks can run
		// in parallel, so the state object protects its own collections.
		agentOpts.Hooks.PreGeneration = append(agentOpts.Hooks.PreGeneration, func(_ context.Context, hctx *dive.HookContext) error {
			hctx.Values[contextDemoStateKey] = &contextDemoTurnState{}
			return nil
		})
	}

	if selection.workspace {
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, workspaceContextDemoHook(workspaceDir))
	}
	if selection.sources {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, sourceLedgerCollectorHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, sourceLedgerReminderHook())
	}
	if selection.verification {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, verificationCollectorHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, verificationReminderHook())
	}
	if selection.recovery {
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, recoveryContextDemoHook())
	}
	if selection.pipeline {
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, pipelineContextDemoHook(workspaceDir))
	}
	if selection.quality {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, qualityGateCollectorHook(qualityGatePassed))
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, qualityGateCollectorFailureHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, qualityGateReminderHook())
	}
	if selection.security {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, securityAwarenessSuccessHook())
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, securityAwarenessFailureHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, securityAwarenessReminderHook())
	}
}

// contextDemoTurnState is allocated for each CreateResponse call. It is shared
// by tool hooks within that call and discarded before the next user turn.
type contextDemoTurnState struct {
	mu sync.Mutex

	sources                   []string
	omittedSourceObservations int

	unverified          []string
	omittedUnverified   int
	batchChanges        []string
	omittedBatchChanges int
	batchCheck          string

	qualityGates      map[qualityGateKind]qualityGateObservation
	batchSecurityRisk map[securityRiskCategory]securityRiskObservation
}

func contextDemoState(hctx *dive.HookContext) *contextDemoTurnState {
	if hctx == nil || hctx.Values == nil {
		return nil
	}
	state, _ := hctx.Values[contextDemoStateKey].(*contextDemoTurnState)
	return state
}

func (s *contextDemoTurnState) addSource(source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addBoundedContextDemoItem(&s.sources, &s.omittedSourceObservations, source)
}

type sourceLedgerSnapshot struct {
	sources []string
	omitted int
}

func (s *contextDemoTurnState) sourceSnapshot() sourceLedgerSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sourceLedgerSnapshot{
		sources: append([]string(nil), s.sources...),
		omitted: s.omittedSourceObservations,
	}
}

func (s *contextDemoTurnState) addBatchChange(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	addBoundedContextDemoItem(&s.batchChanges, &s.omittedBatchChanges, path)
}

func (s *contextDemoTurnState) addBatchCheck(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batchCheck == "" || command < s.batchCheck {
		s.batchCheck = command
	}
}

type verificationUpdate struct {
	checkedPaths      []string
	checkedOmitted    int
	checkCommand      string
	unverified        []string
	unverifiedOmitted int
	emitDebt          bool
}

// applyVerificationBatch treats a check as evidence only for debt that existed
// before its tool batch. An edit and test launched in parallel do not prove that
// the test covered the edit, regardless of which tool happens to finish first.
func (s *contextDemoTurnState) applyVerificationBatch() verificationUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()

	var update verificationUpdate
	if s.batchCheck != "" && (len(s.unverified) > 0 || s.omittedUnverified > 0) {
		update.checkedPaths = append([]string(nil), s.unverified...)
		update.checkedOmitted = s.omittedUnverified
		update.checkCommand = s.batchCheck
		s.unverified = nil
		s.omittedUnverified = 0
	}
	for _, path := range s.batchChanges {
		addBoundedContextDemoItem(&s.unverified, &s.omittedUnverified, path)
	}
	s.omittedUnverified += s.omittedBatchChanges
	update.emitDebt = len(s.batchChanges) > 0 || s.omittedBatchChanges > 0
	update.unverified = append([]string(nil), s.unverified...)
	update.unverifiedOmitted = s.omittedUnverified
	s.batchChanges = nil
	s.omittedBatchChanges = 0
	s.batchCheck = ""
	return update
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// addBoundedContextDemoItem keeps the model-facing set deterministic under
// parallel hooks while bounding both stored values and prompt growth.
func addBoundedContextDemoItem(items *[]string, omitted *int, value string) {
	if stringSliceContains(*items, value) {
		return
	}
	if len(*items) == contextDemoItemLimit {
		if value < (*items)[len(*items)-1] {
			(*items)[len(*items)-1] = value
			sort.Strings(*items)
		}
		(*omitted)++
		return
	}
	*items = append(*items, value)
	sort.Strings(*items)
}

func toolInput(call *llm.ToolUseContent) map[string]any {
	if call == nil || len(call.Input) == 0 {
		return nil
	}
	var input map[string]any
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return nil
	}
	return input
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func truncateText(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-1]) + "…"
}
