package main

import (
	"context"
	"encoding/json"
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

type contextDemoDelivery string

const (
	contextDemoModelOnly contextDemoDelivery = "model-only"
)

type contextDemoNotice struct {
	Reminder dive.Reminder
	Delivery contextDemoDelivery
	Action   string
}

type contextDemoReporter func(contextDemoNotice)

type contextDemoRuntime struct {
	report contextDemoReporter
}

func (r contextDemoRuntime) appendChangedModelOnly(hctx *dive.HookContext, reminder dive.Reminder) error {
	action, changed := "queued", true
	if state := contextDemoState(hctx); state != nil {
		action, changed = state.recordModelOnlyReminder(reminder)
	}
	if !changed {
		return nil
	}
	if err := hctx.AppendReminder(reminder, dive.ModelOnly); err != nil {
		return err
	}
	if r.report != nil {
		r.report(contextDemoNotice{Reminder: reminder, Delivery: contextDemoModelOnly, Action: action})
	}
	return nil
}

func (r contextDemoRuntime) appendModelOnly(hctx *dive.HookContext, reminder dive.Reminder) error {
	if err := hctx.AppendReminder(reminder, dive.ModelOnly); err != nil {
		return err
	}
	if r.report != nil {
		r.report(contextDemoNotice{Reminder: reminder, Delivery: contextDemoModelOnly, Action: "queued"})
	}
	return nil
}

func applyContextDemoAgentOptions(agentOpts *dive.AgentOptions, workspaceDir string, selection contextDemoSelection, reporters ...contextDemoReporter) {
	if selection.empty() {
		return
	}
	var runtime contextDemoRuntime
	if len(reporters) > 0 {
		runtime.report = reporters[0]
	}

	// Install turn-local state before the first iteration. Besides tracking
	// verification and security observations, it prevents unchanged snapshots
	// from being appended again during a long tool loop. Tool hooks can run in
	// parallel, so the state object protects its own collections.
	agentOpts.Hooks.PreGeneration = append(agentOpts.Hooks.PreGeneration, func(_ context.Context, hctx *dive.HookContext) error {
		hctx.Values[contextDemoStateKey] = &contextDemoTurnState{}
		return nil
	})

	if selection.enabled(contextDemoWorkspace) {
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, workspaceContextDemoHook(workspaceDir, runtime))
	}
	if selection.enabled(contextDemoPipeline) {
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, pipelineContextDemoHook(workspaceDir, runtime))
	}
	if selection.enabled(contextDemoVerification) {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, verificationCollectorHook())
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, qualityGateCollectorHook(qualityGatePassed))
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, qualityGateCollectorFailureHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, verificationReminderHook(runtime))
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, verificationGateReminderHook(runtime))
	}
	if selection.enabled(contextDemoRecovery) {
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, recoveryContextDemoHook(runtime))
	}
	if selection.enabled(contextDemoSecurity) {
		agentOpts.Hooks.PostToolUse = append(agentOpts.Hooks.PostToolUse, securityAwarenessSuccessHook())
		agentOpts.Hooks.PostToolUseFailure = append(agentOpts.Hooks.PostToolUseFailure, securityAwarenessFailureHook())
		agentOpts.Hooks.PreIteration = append(agentOpts.Hooks.PreIteration, securityAwarenessReminderHook(runtime))
	}
}

// contextDemoTurnState is allocated for each CreateResponse call. It is shared
// by tool hooks within that call and discarded before the next user turn.
type contextDemoTurnState struct {
	mu sync.Mutex

	unverified          []string
	omittedUnverified   int
	batchChanges        []string
	omittedBatchChanges int
	batchCheck          string

	qualityGates      map[qualityGateKind]qualityGateObservation
	batchSecurityRisk map[securityRiskCategory]securityRiskObservation
	reportedModelOnly map[string]string
}

func contextDemoState(hctx *dive.HookContext) *contextDemoTurnState {
	if hctx == nil || hctx.Values == nil {
		return nil
	}
	state, _ := hctx.Values[contextDemoStateKey].(*contextDemoTurnState)
	return state
}

func (s *contextDemoTurnState) recordModelOnlyReminder(reminder dive.Reminder) (action string, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportedModelOnly == nil {
		s.reportedModelOnly = make(map[string]string)
	}
	previous, exists := s.reportedModelOnly[reminder.Name]
	if exists && previous == reminder.Content {
		return "", false
	}
	s.reportedModelOnly[reminder.Name] = reminder.Content
	if exists {
		return "refreshed", true
	}
	return "queued", true
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
