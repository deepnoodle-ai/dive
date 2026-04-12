package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/google/uuid"
)

// SessionProvider returns the Dive session to use for an A2A task. The
// contextID corresponds to the A2A Message.contextId the client sent (or
// to an empty string on the very first message of a new context).
//
// The provider is called from the server's JSON-RPC dispatcher before the
// Dive agent runs. Implementations are responsible for any persistence:
// the server does not cache sessions across calls.
//
// If the provider returns a nil Session the agent runs stateless and the
// adapter synthesizes a per-task context ID.
type SessionProvider func(ctx context.Context, contextID string) (dive.Session, error)

// ServerOptions configures a Server.
type ServerOptions struct {
	// Agent is the Dive agent that will be exposed via A2A. Required.
	Agent *dive.Agent

	// Card is the static portion of the agent card served at
	// /.well-known/agent-card.json. The server fills in defaults for
	// any missing required fields.
	Card AgentCard

	// BaseURL is the public URL that clients should use to reach this
	// server. Optional — used to fill AgentCard.SupportedInterfaces.
	BaseURL string

	// Path is the mount path of the JSON-RPC endpoint. Defaults to "/".
	Path string

	// SessionProvider supplies Dive sessions for contextIds. Optional —
	// when nil, the server runs the agent without a session per call.
	SessionProvider SessionProvider

	// TaskStore persists task records. Optional — defaults to an
	// in-memory store. Prototype callers can leave this nil.
	TaskStore TaskStore

	// Logger is an optional logger. Defaults to llm.NullLogger.
	Logger llm.Logger
}

// Server exposes a Dive agent as an A2A endpoint. Wire it into an HTTP
// mux via Handler; the returned handler serves both the well-known agent
// card and the JSON-RPC endpoint at the configured Path.
type Server struct {
	agent    *dive.Agent
	card     AgentCard
	path     string
	store    TaskStore
	provider SessionProvider
	logger   llm.Logger

	// cardBytes is the pre-marshaled agent card response.
	cardBytes []byte
	cardMu    sync.RWMutex

	// inflight tracks cancel functions for in-progress turns so that
	// tasks/cancel can interrupt a running CreateResponse.
	inflight   map[string]context.CancelFunc
	inflightMu sync.Mutex
}

// NewServer constructs a Server from the given options. It returns an
// error if required fields are missing.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Agent == nil {
		return nil, errors.New("a2a: ServerOptions.Agent is required")
	}
	if opts.Path == "" {
		opts.Path = "/"
	}
	if opts.TaskStore == nil {
		opts.TaskStore = NewMemoryTaskStore()
	}
	if opts.Logger == nil {
		opts.Logger = &llm.NullLogger{}
	}
	card := opts.Card
	if card.Name == "" {
		card.Name = opts.Agent.Name()
	}
	if card.Name == "" {
		card.Name = "dive-agent"
	}
	if card.Version == "" {
		card.Version = "0.1.0"
	}
	if card.Description == "" {
		card.Description = card.Name + " (Dive A2A agent)"
	}
	if len(card.DefaultInputModes) == 0 {
		card.DefaultInputModes = []string{"text/plain"}
	}
	if len(card.DefaultOutputModes) == 0 {
		card.DefaultOutputModes = []string{"text/plain"}
	}
	if len(card.Skills) == 0 {
		card.Skills = []AgentSkill{{
			ID:          "chat",
			Name:        "chat",
			Description: "General purpose conversational interface.",
			Tags:        []string{"chat"},
		}}
	}
	if len(card.SupportedInterfaces) == 0 && opts.BaseURL != "" {
		url := strings.TrimRight(opts.BaseURL, "/") + opts.Path
		card.SupportedInterfaces = []AgentInterface{{
			URL:             url,
			ProtocolBinding: "JSONRPC",
			ProtocolVersion: "1.0",
		}}
	}
	// Dive agents support streaming by default because CreateResponse
	// exposes an event callback.
	card.Capabilities.Streaming = true

	s := &Server{
		agent:    opts.Agent,
		card:     card,
		path:     opts.Path,
		store:    opts.TaskStore,
		provider: opts.SessionProvider,
		logger:   opts.Logger,
		inflight: make(map[string]context.CancelFunc),
	}
	if err := s.refreshCardBytes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) refreshCardBytes() error {
	b, err := json.Marshal(s.card)
	if err != nil {
		return fmt.Errorf("a2a: marshal agent card: %w", err)
	}
	s.cardMu.Lock()
	s.cardBytes = b
	s.cardMu.Unlock()
	return nil
}

// Card returns a copy of the server's agent card.
func (s *Server) Card() AgentCard {
	return s.card
}

// Handler returns an http.Handler that serves the well-known agent card
// and the JSON-RPC endpoint. Mount it at the root of your public origin
// (the server picks out the specific paths it handles internally). The
// card is served at both the canonical /.well-known/agent-card.json path
// and the legacy /.well-known/agent.json path so older clients keep
// working.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(DefaultAgentCardPath, s.handleCard)
	if LegacyAgentCardPath != DefaultAgentCardPath {
		mux.HandleFunc(LegacyAgentCardPath, s.handleCard)
	}
	mux.HandleFunc(s.path, s.handleRPC)
	return mux
}

// ServeHTTP implements http.Handler by delegating to Handler().
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler().ServeHTTP(w, r)
}

func (s *Server) handleCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.cardMu.RLock()
	body := s.cardBytes
	s.cardMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write(body)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, newRPCError(ErrorCodeParseError, "parse error: "+err.Error(), nil))
		return
	}
	if req.JSONRPC != "2.0" {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidRequest, `jsonrpc must be "2.0"`, nil))
		return
	}

	switch req.Method {
	case MethodMessageSend:
		s.handleMessageSend(w, r, &req)
	case MethodMessageStream:
		s.handleMessageStream(w, r, &req)
	case MethodTasksGet:
		s.handleTasksGet(w, r, &req)
	case MethodTasksCancel:
		s.handleTasksCancel(w, r, &req)
	case MethodTasksResubscribe,
		MethodTasksPushNotifConfigSet,
		MethodTasksPushNotifConfigGet,
		MethodTasksPushNotifConfigList,
		MethodTasksPushNotifConfigDelete,
		MethodAgentExtendedCard:
		writeRPCError(w, req.ID, newRPCError(ErrorCodeUnsupportedOperation,
			"a2a server does not implement "+req.Method, nil))
	default:
		writeRPCError(w, req.ID, newRPCError(ErrorCodeMethodNotFound, "method not found: "+req.Method, nil))
	}
}

// ---------------------------------------------------------------------------
// message/send
// ---------------------------------------------------------------------------

func (s *Server) handleMessageSend(w http.ResponseWriter, r *http.Request, req *RPCRequest) {
	var params SendMessageParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "invalid params: "+err.Error(), nil))
		return
	}
	if err := params.Validate(); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, err.Error(), nil))
		return
	}
	task, rpcErr := s.runTurn(r.Context(), &params, nil)
	if rpcErr != nil {
		writeRPCError(w, req.ID, rpcErr)
		return
	}
	writeRPCResult(w, req.ID, task)
}

// ---------------------------------------------------------------------------
// message/stream
// ---------------------------------------------------------------------------

func (s *Server) handleMessageStream(w http.ResponseWriter, r *http.Request, req *RPCRequest) {
	var params SendMessageParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "invalid params: "+err.Error(), nil))
		return
	}
	if err := params.Validate(); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, err.Error(), nil))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInternalError, "streaming not supported by this server", nil))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Streaming callback: emit a status update for every Dive response
	// item that translates to a user-visible progress signal.
	var streamMu sync.Mutex
	emit := func(sr StreamResponse) {
		streamMu.Lock()
		defer streamMu.Unlock()
		writeSSEEvent(w, flusher, req.ID, sr)
	}

	cb := func(ctx context.Context, item *dive.ResponseItem) error {
		switch item.Type {
		case dive.ResponseItemTypeToolCall:
			if item.ToolCall == nil {
				return nil
			}
			emit(StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{
				Status: TaskStatus{
					State: TaskStateWorking,
					Message: &Message{
						MessageID: uuid.NewString(),
						Role:      RoleAgent,
						Parts: []Part{NewTextPart(fmt.Sprintf(
							"Calling tool: %s", item.ToolCall.Name))},
					},
				},
			}})
		case dive.ResponseItemTypeMessage:
			if item.Message == nil || item.Message.Role != llm.Assistant {
				return nil
			}
			text := item.Message.LastText()
			if text == "" {
				return nil
			}
			emit(StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{
				Status: TaskStatus{
					State: TaskStateWorking,
					Message: &Message{
						MessageID: uuid.NewString(),
						Role:      RoleAgent,
						Parts:     []Part{NewTextPart(text)},
					},
				},
			}})
		}
		return nil
	}

	task, rpcErr := s.runTurn(r.Context(), &params, cb)
	if rpcErr != nil {
		streamMu.Lock()
		defer streamMu.Unlock()
		writeSSEError(w, flusher, req.ID, rpcErr)
		return
	}

	// Emit all artifacts, then the final status event so the client
	// can correlate everything with the task.
	streamMu.Lock()
	for _, art := range task.Artifacts {
		writeSSEEvent(w, flusher, req.ID, StreamResponse{
			ArtifactUpdate: &TaskArtifactUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Artifact:  art,
				LastChunk: true,
			},
		})
	}
	writeSSEEvent(w, flusher, req.ID, StreamResponse{
		StatusUpdate: &TaskStatusUpdateEvent{
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Status:    task.Status,
		},
	})
	streamMu.Unlock()
}

// ---------------------------------------------------------------------------
// tasks/get
// ---------------------------------------------------------------------------

func (s *Server) handleTasksGet(w http.ResponseWriter, r *http.Request, req *RPCRequest) {
	var params TaskIDParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "invalid params: "+err.Error(), nil))
		return
	}
	if params.ID == "" {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "missing task id", nil))
		return
	}
	rec, ok, err := s.store.Get(r.Context(), params.ID)
	if err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInternalError, err.Error(), nil))
		return
	}
	if !ok {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeTaskNotFound, "task not found: "+params.ID, nil))
		return
	}
	writeRPCResult(w, req.ID, rec.Task)
}

// ---------------------------------------------------------------------------
// tasks/cancel
// ---------------------------------------------------------------------------

func (s *Server) handleTasksCancel(w http.ResponseWriter, r *http.Request, req *RPCRequest) {
	var params TaskIDParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "invalid params: "+err.Error(), nil))
		return
	}
	if params.ID == "" {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInvalidParams, "missing task id", nil))
		return
	}
	rec, ok, err := s.store.Get(r.Context(), params.ID)
	if err != nil {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeInternalError, err.Error(), nil))
		return
	}
	if !ok {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeTaskNotFound, "task not found: "+params.ID, nil))
		return
	}
	if rec.Task.Status.State.IsTerminal() {
		writeRPCError(w, req.ID, newRPCError(ErrorCodeTaskNotCancelable,
			"task is already in terminal state: "+string(rec.Task.Status.State), nil))
		return
	}
	s.cancelInflight(params.ID)
	now := time.Now().UTC()
	rec.Task.Status = TaskStatus{
		State:     TaskStateCanceled,
		Timestamp: &now,
	}
	rec.Suspension = nil

	// If the session supports cancellation, clear its suspension state
	// so the underlying session doesn't retain stale pending tool calls.
	if rec.SessionID != "" && s.provider != nil {
		if sess, sessErr := s.provider(r.Context(), rec.SessionID); sessErr == nil && sess != nil {
			if suspendable, ok := sess.(dive.SuspendableSession); ok {
				_ = suspendable.CancelSuspension(r.Context())
			}
		}
	}

	_ = s.store.Put(r.Context(), rec)
	writeRPCResult(w, req.ID, rec.Task)
}

// ---------------------------------------------------------------------------
// Turn runner — shared by message/send and message/stream
// ---------------------------------------------------------------------------

// runTurn runs a single Dive turn for the given params and returns the
// resulting A2A Task. The callback, if non-nil, is wired into the agent's
// event callback so streaming consumers can observe progress before the
// turn completes.
func (s *Server) runTurn(ctx context.Context, params *SendMessageParams, cb dive.EventCallback) (*Task, *RPCError) {
	contextID := params.Message.ContextID
	taskID := params.Message.TaskID

	// If the client is targeting an existing task, attempt resume.
	if taskID != "" {
		return s.resumeTurn(ctx, params, cb, taskID)
	}

	// Fresh turn: pick a task ID and context ID.
	if contextID == "" {
		contextID = uuid.NewString()
	}
	newTaskID := uuid.NewString()

	sess, err := s.resolveSession(ctx, contextID)
	if err != nil {
		return nil, newRPCError(ErrorCodeInternalError, "session: "+err.Error(), nil)
	}

	inputMsg, err := inputMessageFromParts(params.Message)
	if err != nil {
		return nil, newRPCError(ErrorCodeInvalidParams, err.Error(), nil)
	}

	opts := []dive.CreateResponseOption{dive.WithMessages(inputMsg)}
	if sess != nil {
		opts = append(opts, dive.WithSession(sess))
	}
	if cb != nil {
		opts = append(opts, dive.WithEventCallback(cb))
	}

	turnCtx, turnCancel := context.WithCancel(ctx)
	defer turnCancel()
	s.trackInflight(newTaskID, turnCancel)
	defer s.untrackInflight(newTaskID)

	resp, runErr := s.agent.CreateResponse(turnCtx, opts...)
	if runErr != nil {
		return nil, rpcErrorFromTurn(runErr)
	}

	task := buildTaskFromResponse(newTaskID, contextID, params.Message, resp)

	rec := &TaskRecord{
		Task:       task,
		Suspension: resp.Suspension,
		SessionID:  contextID,
	}
	if err := s.store.Put(ctx, rec); err != nil {
		return nil, newRPCError(ErrorCodeInternalError, "task store: "+err.Error(), nil)
	}
	return task, nil
}

// resumeTurn handles a message/send or message/stream that targets an
// existing task, typically an input-required task that is waiting for the
// client to supply the next piece of input.
func (s *Server) resumeTurn(ctx context.Context, params *SendMessageParams, cb dive.EventCallback, taskID string) (*Task, *RPCError) {
	rec, ok, err := s.store.Get(ctx, taskID)
	if err != nil {
		return nil, newRPCError(ErrorCodeInternalError, err.Error(), nil)
	}
	if !ok {
		return nil, newRPCError(ErrorCodeTaskNotFound, "task not found: "+taskID, nil)
	}
	if rec.Task.Status.State.IsTerminal() {
		return nil, newRPCError(ErrorCodeInvalidRequest,
			"task is already in terminal state: "+string(rec.Task.Status.State), nil)
	}

	sess, err := s.resolveSession(ctx, rec.SessionID)
	if err != nil {
		return nil, newRPCError(ErrorCodeInternalError, "session: "+err.Error(), nil)
	}

	// Snapshot the existing task history so we can prepend it to the
	// freshly built turn below. The new user message is *not* appended
	// here — buildTaskFromResponse seeds its own history with userMsg,
	// and double-appending it here would duplicate the entry.
	priorHistory := append([]*Message(nil), rec.Task.History...)

	opts := []dive.CreateResponseOption{}
	if sess != nil {
		opts = append(opts, dive.WithSession(sess))
	}
	if cb != nil {
		opts = append(opts, dive.WithEventCallback(cb))
	}

	if rec.Suspension != nil && len(rec.Suspension.PendingToolCalls) > 0 {
		// Resume path. Map the inbound message to tool results.
		results, rpcErr := resumeToolResults(rec.Suspension, params.Message)
		if rpcErr != nil {
			return nil, rpcErr
		}
		opts = append(opts, dive.WithResume(rec.Suspension, results))
	} else {
		// No suspension — treat the inbound message as a new turn on
		// the same session.
		inputMsg, msgErr := inputMessageFromParts(params.Message)
		if msgErr != nil {
			return nil, newRPCError(ErrorCodeInvalidParams, msgErr.Error(), nil)
		}
		opts = append(opts, dive.WithMessages(inputMsg))
	}

	turnCtx, turnCancel := context.WithCancel(ctx)
	defer turnCancel()
	s.trackInflight(taskID, turnCancel)
	defer s.untrackInflight(taskID)

	resp, runErr := s.agent.CreateResponse(turnCtx, opts...)
	if runErr != nil {
		return nil, rpcErrorFromTurn(runErr)
	}

	// Merge the fresh response into the existing task record.
	updated := buildTaskFromResponse(rec.Task.ID, rec.Task.ContextID, params.Message, resp)
	updated.History = append(priorHistory, updated.History...)
	rec.Task = updated
	rec.Suspension = resp.Suspension
	if err := s.store.Put(ctx, rec); err != nil {
		return nil, newRPCError(ErrorCodeInternalError, "task store: "+err.Error(), nil)
	}
	return updated, nil
}

// resumeToolResults translates an inbound user message into ToolResults
// for the pending tool calls on a suspended Dive turn.
func resumeToolResults(state *dive.SuspensionState, msg *Message) (map[string]*dive.ToolResult, *RPCError) {
	if len(state.PendingToolCalls) == 0 {
		return nil, nil
	}

	// Check for a structured toolResults DataPart.
	if mapped := extractToolResultsMap(msg); mapped != nil {
		results := make(map[string]*dive.ToolResult, len(state.PendingToolCalls))
		for _, call := range state.PendingToolCalls {
			text, ok := mapped[call.ID]
			if !ok {
				return nil, newRPCError(ErrorCodeInvalidParams,
					fmt.Sprintf("toolResults map missing pending call ID %q", call.ID), nil)
			}
			results[call.ID] = dive.NewToolResultText(text)
		}
		return results, nil
	}

	// Fall back to text: use for single call, broadcast for multiple.
	text := msg.TextContent()
	results := make(map[string]*dive.ToolResult, len(state.PendingToolCalls))
	for _, call := range state.PendingToolCalls {
		results[call.ID] = dive.NewToolResultText(text)
	}
	return results, nil
}

// extractToolResultsMap looks for a DataPart with a "toolResults" key
// whose value is a map[string]string. Returns nil if not found.
func extractToolResultsMap(msg *Message) map[string]string {
	if msg == nil {
		return nil
	}
	for _, p := range msg.Parts {
		if !p.IsData() {
			continue
		}
		raw, ok := p.Data["toolResults"]
		if !ok {
			continue
		}
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out := make(map[string]string, len(m))
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return nil
			}
			out[k] = s
		}
		return out
	}
	return nil
}

func (s *Server) trackInflight(taskID string, cancel context.CancelFunc) {
	s.inflightMu.Lock()
	s.inflight[taskID] = cancel
	s.inflightMu.Unlock()
}

func (s *Server) untrackInflight(taskID string) {
	s.inflightMu.Lock()
	delete(s.inflight, taskID)
	s.inflightMu.Unlock()
}

func (s *Server) cancelInflight(taskID string) bool {
	s.inflightMu.Lock()
	cancel, ok := s.inflight[taskID]
	s.inflightMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// resolveSession calls the configured SessionProvider, falling back to
// nil when no provider is set.
func (s *Server) resolveSession(ctx context.Context, contextID string) (dive.Session, error) {
	if s.provider == nil {
		return nil, nil
	}
	return s.provider(ctx, contextID)
}

// rpcErrorFromTurn converts an error returned from Dive's CreateResponse
// into an RPC error.
func rpcErrorFromTurn(err error) *RPCError {
	switch {
	case errors.Is(err, context.Canceled):
		return newRPCError(ErrorCodeTaskNotCancelable, "task was canceled", nil)
	case errors.Is(err, dive.ErrNoSuspendedTurn):
		return newRPCError(ErrorCodeInvalidRequest, "no suspended turn to resume", nil)
	case errors.Is(err, dive.ErrUnknownPendingToolCall):
		return newRPCError(ErrorCodeInvalidRequest, "unknown pending tool call id", nil)
	case errors.Is(err, dive.ErrResumeRequired):
		return newRPCError(ErrorCodeInvalidRequest, "session is suspended; resume required", nil)
	default:
		return newRPCError(ErrorCodeInternalError, err.Error(), nil)
	}
}

// ---------------------------------------------------------------------------
// Response → Task mapping
// ---------------------------------------------------------------------------

// contentToParts maps Dive LLM content to A2A v1.0 parts. Internal content
// types (tool use, tool result, thinking) are skipped — only user-visible
// content is projected.
func contentToParts(content []llm.Content) []Part {
	var parts []Part
	for _, c := range content {
		switch v := c.(type) {
		case *llm.TextContent:
			if v.Text != "" {
				parts = append(parts, NewTextPart(v.Text))
			}
		case *llm.ImageContent:
			if v.Source != nil {
				p := partFromSource(v.Source, "")
				if p != nil {
					parts = append(parts, *p)
				}
			}
		case *llm.DocumentContent:
			if v.Source != nil {
				p := partFromSource(v.Source, v.Title)
				if p != nil {
					parts = append(parts, *p)
				}
			}
		case *llm.RefusalContent:
			if v.Text != "" {
				parts = append(parts, NewTextPart(v.Text))
			}
		}
	}
	return parts
}

func partFromSource(src *llm.ContentSource, title string) *Part {
	if src == nil {
		return nil
	}
	switch src.Type {
	case llm.ContentSourceTypeBase64:
		p := NewRawPart(src.Data, src.MediaType)
		p.Filename = title
		return &p
	case llm.ContentSourceTypeURL:
		p := NewURLPart(src.URL, src.MediaType)
		p.Filename = title
		return &p
	}
	return nil
}

// buildTaskFromResponse projects a completed (or suspended) Dive Response
// onto the A2A Task shape.
func buildTaskFromResponse(taskID, contextID string, userMsg *Message, resp *dive.Response) *Task {
	now := time.Now().UTC()
	task := &Task{
		ID:        taskID,
		ContextID: contextID,
	}
	if userMsg != nil {
		task.History = append(task.History, userMsg)
	}

	for _, out := range resp.OutputMessages {
		if out.Role != llm.Assistant {
			continue
		}
		parts := contentToParts(out.Content)
		if len(parts) == 0 {
			continue
		}
		task.History = append(task.History, &Message{
			MessageID: uuid.NewString(),
			Role:      RoleAgent,
			TaskID:    taskID,
			ContextID: contextID,
			Parts:     parts,
		})
	}

	// Status mapping.
	switch resp.Status {
	case dive.ResponseStatusSuspended:
		state := TaskStateInputRequired
		if resp.Suspension != nil && len(resp.Suspension.PendingToolCalls) > 0 {
			if resp.Suspension.PendingToolCalls[0].Reason == dive.SuspendReasonAuth {
				state = TaskStateAuthRequired
			}
		}
		task.Status = TaskStatus{
			State:     state,
			Timestamp: &now,
		}
		if resp.Suspension != nil && len(resp.Suspension.PendingToolCalls) > 0 {
			prompt := resp.Suspension.PendingToolCalls[0].Prompt
			if prompt == "" {
				prompt = "Agent is waiting for input."
			}
			task.Status.Message = &Message{
				MessageID: uuid.NewString(),
				Role:      RoleAgent,
				TaskID:    taskID,
				ContextID: contextID,
				Parts:     []Part{NewTextPart(prompt)},
			}
			if md := resp.Suspension.PendingToolCalls[0].Metadata; len(md) > 0 {
				task.Metadata = mergeMetadata(task.Metadata, map[string]any{
					"suspend": md,
				})
			}
		}
	case "", dive.ResponseStatusCompleted:
		task.Status = TaskStatus{
			State:     TaskStateCompleted,
			Timestamp: &now,
		}
		task.Artifacts = artifactsFromResponse(resp)
	default:
		task.Status = TaskStatus{
			State:     TaskStateFailed,
			Timestamp: &now,
			Message: &Message{
				MessageID: uuid.NewString(),
				Role:      RoleAgent,
				Parts:     []Part{NewTextPart("unknown response status: " + string(resp.Status))},
			},
		}
	}
	return task
}

// artifactsFromResponse builds A2A artifacts from the assistant messages.
func artifactsFromResponse(resp *dive.Response) []*Artifact {
	var artifacts []*Artifact
	for _, out := range resp.OutputMessages {
		if out.Role != llm.Assistant {
			continue
		}
		parts := contentToParts(out.Content)
		if len(parts) == 0 {
			continue
		}
		artifacts = append(artifacts, &Artifact{
			ArtifactID: uuid.NewString(),
			Name:       "response",
			Parts:      parts,
		})
	}
	return artifacts
}

// inputMessageFromParts converts A2A message parts into an *llm.Message
// with properly typed content. Text parts become TextContent, raw/url
// parts become ImageContent or DocumentContent based on MIME type, and
// data parts are rendered as a JSON code block in TextContent.
func inputMessageFromParts(msg *Message) (*llm.Message, error) {
	if msg == nil || len(msg.Parts) == 0 {
		return nil, fmt.Errorf("a2a: message has no parts")
	}
	out := &llm.Message{Role: llm.User}
	for _, p := range msg.Parts {
		switch {
		case p.IsText():
			out.Content = append(out.Content, &llm.TextContent{Text: p.Text})
		case p.IsData():
			encoded, err := json.Marshal(p.Data)
			if err != nil {
				return nil, fmt.Errorf("a2a: marshal data part: %w", err)
			}
			out.Content = append(out.Content, &llm.TextContent{
				Text: "```json\n" + string(encoded) + "\n```",
			})
		case p.IsRaw():
			if isImageMIME(p.MediaType) {
				out.Content = append(out.Content, &llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: p.MediaType,
						Data:      p.Raw,
					},
				})
			} else {
				out.Content = append(out.Content, &llm.DocumentContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: p.MediaType,
						Data:      p.Raw,
					},
					Title: p.Filename,
				})
			}
		case p.IsURL():
			if isImageMIME(p.MediaType) {
				out.Content = append(out.Content, &llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeURL,
						MediaType: p.MediaType,
						URL:       p.URL,
					},
				})
			} else {
				out.Content = append(out.Content, &llm.DocumentContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeURL,
						MediaType: p.MediaType,
						URL:       p.URL,
					},
					Title: p.Filename,
				})
			}
		}
	}
	if len(out.Content) == 0 {
		return nil, fmt.Errorf("a2a: message has no renderable content")
	}
	return out, nil
}

func isImageMIME(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

func mergeMetadata(dst, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ---------------------------------------------------------------------------
// Wire helpers
// ---------------------------------------------------------------------------

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(RPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, rpcErr *RPCError) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(RPCResponse{JSONRPC: "2.0", ID: id, Error: rpcErr})
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, id json.RawMessage, result StreamResponse) {
	payload := RPCResponse{JSONRPC: "2.0", ID: id, Result: result}
	var buf bytes.Buffer
	buf.WriteString("data: ")
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return
	}
	buf.WriteString("\n")
	_, _ = w.Write(buf.Bytes())
	flusher.Flush()
}

func writeSSEError(w http.ResponseWriter, flusher http.Flusher, id json.RawMessage, rpcErr *RPCError) {
	payload := RPCResponse{JSONRPC: "2.0", ID: id, Error: rpcErr}
	var buf bytes.Buffer
	buf.WriteString("data: ")
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return
	}
	buf.WriteString("\n")
	_, _ = w.Write(buf.Bytes())
	flusher.Flush()
}
