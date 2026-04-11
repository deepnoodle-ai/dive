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
	// /.well-known/agent.json. The server fills in the URL from
	// ServerOptions.BaseURL if BaseURL is set and Card.URL is empty.
	Card AgentCard

	// BaseURL is the public URL that clients should use to reach this
	// server. Optional — used to fill AgentCard.URL and nothing else.
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
	if card.URL == "" && opts.BaseURL != "" {
		card.URL = strings.TrimRight(opts.BaseURL, "/") + opts.Path
	}
	if len(card.DefaultInputModes) == 0 {
		card.DefaultInputModes = []string{"text/plain"}
	}
	if len(card.DefaultOutputModes) == 0 {
		card.DefaultOutputModes = []string{"text/plain"}
	}
	if len(card.Skills) == 0 {
		// Publish one default skill so the card validates against strict
		// A2A clients without forcing every caller to enumerate skills.
		card.Skills = []AgentSkill{{
			ID:          "chat",
			Name:        "chat",
			Description: "General purpose conversational interface.",
			Tags:        []string{"chat"},
		}}
	}
	if card.PreferredTransport == "" {
		card.PreferredTransport = "JSONRPC"
	}
	// Dive agents support streaming by default because CreateResponse
	// exposes an event callback. Expose that in the card unless the
	// caller explicitly opted out.
	card.Capabilities.Streaming = true

	s := &Server{
		agent:    opts.Agent,
		card:     card,
		path:     opts.Path,
		store:    opts.TaskStore,
		provider: opts.SessionProvider,
		logger:   opts.Logger,
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
	emit := func(event any) {
		streamMu.Lock()
		defer streamMu.Unlock()
		writeSSEEvent(w, flusher, req.ID, event)
	}

	cb := func(ctx context.Context, item *dive.ResponseItem) error {
		switch item.Type {
		case dive.ResponseItemTypeToolCall:
			if item.ToolCall == nil {
				return nil
			}
			emit(TaskStatusUpdateEvent{
				Kind: "status-update",
				Status: TaskStatus{
					State: TaskStateWorking,
					Message: &Message{
						MessageID: uuid.NewString(),
						Role:      RoleAgent,
						Parts: []Part{NewTextPart(fmt.Sprintf(
							"Calling tool: %s", item.ToolCall.Name))},
					},
					Timestamp: time.Now().UTC(),
				},
			})
		case dive.ResponseItemTypeMessage:
			if item.Message == nil || item.Message.Role != llm.Assistant {
				return nil
			}
			text := item.Message.LastText()
			if text == "" {
				return nil
			}
			emit(TaskStatusUpdateEvent{
				Kind: "status-update",
				Status: TaskStatus{
					State: TaskStateWorking,
					Message: &Message{
						MessageID: uuid.NewString(),
						Role:      RoleAgent,
						Parts:     []Part{NewTextPart(text)},
					},
					Timestamp: time.Now().UTC(),
				},
			})
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

	// Patch contextID/taskID on the final status event so the client
	// can correlate it with the task that was just created.
	finalEvent := TaskStatusUpdateEvent{
		Kind:      "status-update",
		TaskID:    task.ID,
		ContextID: task.ContextID,
		Status:    task.Status,
		Final:     true,
	}
	if len(task.Artifacts) > 0 {
		streamMu.Lock()
		writeSSEEvent(w, flusher, req.ID, TaskArtifactUpdateEvent{
			Kind:      "artifact-update",
			TaskID:    task.ID,
			ContextID: task.ContextID,
			Artifact:  task.Artifacts[0],
			LastChunk: true,
		})
		streamMu.Unlock()
	}
	streamMu.Lock()
	writeSSEEvent(w, flusher, req.ID, finalEvent)
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
	rec.Task.Status = TaskStatus{
		State:     TaskStateCanceled,
		Timestamp: time.Now().UTC(),
	}
	rec.Suspension = nil
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

	inputText, err := inputPromptFromMessage(params.Message)
	if err != nil {
		return nil, newRPCError(ErrorCodeInvalidParams, err.Error(), nil)
	}

	opts := []dive.CreateResponseOption{dive.WithInput(inputText)}
	if sess != nil {
		opts = append(opts, dive.WithSession(sess))
	}
	if cb != nil {
		opts = append(opts, dive.WithEventCallback(cb))
	}

	resp, runErr := s.agent.CreateResponse(ctx, opts...)
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

	inputText, err := inputPromptFromMessage(params.Message)
	if err != nil {
		return nil, newRPCError(ErrorCodeInvalidParams, err.Error(), nil)
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
		results, rpcErr := resumeToolResults(rec.Suspension, inputText)
		if rpcErr != nil {
			return nil, rpcErr
		}
		opts = append(opts, dive.WithResume(rec.Suspension, results))
	} else {
		// No suspension — treat the inbound message as a new turn on
		// the same session.
		opts = append(opts, dive.WithInput(inputText))
	}

	resp, runErr := s.agent.CreateResponse(ctx, opts...)
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
// for the pending tool calls on a suspended Dive turn. The prototype
// supports the common case of a single pending call: the full message
// text becomes that call's text result. Multi-pending is rejected with
// a clear error; a future revision can accept a structured payload.
func resumeToolResults(state *dive.SuspensionState, inputText string) (map[string]*dive.ToolResult, *RPCError) {
	if len(state.PendingToolCalls) == 0 {
		return nil, nil
	}
	if len(state.PendingToolCalls) > 1 {
		return nil, newRPCError(ErrorCodeInvalidRequest,
			"a2a adapter cannot resume a turn with multiple pending tool calls yet", nil)
	}
	call := state.PendingToolCalls[0]
	results := map[string]*dive.ToolResult{
		call.ID: dive.NewToolResultText(inputText),
	}
	return results, nil
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
// into an RPC error. Known sentinel errors get specific codes; anything
// else becomes InternalError.
func rpcErrorFromTurn(err error) *RPCError {
	switch {
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

// buildTaskFromResponse projects a completed (or suspended) Dive Response
// onto the A2A Task shape. It is the core of the server's translation
// layer.
func buildTaskFromResponse(taskID, contextID string, userMsg *Message, resp *dive.Response) *Task {
	now := time.Now().UTC()
	task := &Task{
		ID:        taskID,
		ContextID: contextID,
		Kind:      "task",
	}
	if userMsg != nil {
		task.History = append(task.History, userMsg)
	}

	// Append each assistant message (text content only for the
	// prototype) to task history so the remote caller can render the
	// conversation if they care.
	for _, out := range resp.OutputMessages {
		if out.Role != llm.Assistant {
			continue
		}
		text := out.LastText()
		if text == "" {
			continue
		}
		task.History = append(task.History, &Message{
			MessageID: uuid.NewString(),
			Role:      RoleAgent,
			TaskID:    taskID,
			ContextID: contextID,
			Parts:     []Part{NewTextPart(text)},
		})
	}

	// Status mapping.
	switch resp.Status {
	case dive.ResponseStatusSuspended:
		task.Status = TaskStatus{
			State:     TaskStateInputRequired,
			Timestamp: now,
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
			Timestamp: now,
		}
		finalText := resp.OutputText()
		if finalText != "" {
			task.Artifacts = []*Artifact{{
				ArtifactID: uuid.NewString(),
				Name:       "response",
				Parts:      []Part{NewTextPart(finalText)},
				LastChunk:  true,
			}}
		}
	default:
		task.Status = TaskStatus{
			State:     TaskStateFailed,
			Timestamp: now,
			Message: &Message{
				MessageID: uuid.NewString(),
				Role:      RoleAgent,
				Parts:     []Part{NewTextPart("unknown response status: " + string(resp.Status))},
			},
		}
	}
	return task
}

// inputPromptFromMessage flattens an A2A Message into a single prompt
// string suitable for dive.WithInput. Text parts pass through verbatim.
// DataParts are rendered as a JSON code block (labeled with the part's
// "kind" when present in metadata) so the agent can reason over
// structured inputs. FileParts are rendered as a short reference line
// carrying name/MIME/URI when provided; inline base64 file bytes are
// intentionally summarized rather than inlined to keep prompt budgets
// predictable. The function returns an error only if the message is
// empty or contains no renderable content.
func inputPromptFromMessage(msg *Message) (string, error) {
	if msg == nil || len(msg.Parts) == 0 {
		return "", fmt.Errorf("a2a: message has no parts")
	}
	var b strings.Builder
	appendSeparator := func() {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
	}
	for _, p := range msg.Parts {
		switch p.Kind {
		case PartKindText:
			if p.Text == "" {
				continue
			}
			appendSeparator()
			b.WriteString(p.Text)
		case PartKindData:
			if len(p.Data) == 0 {
				continue
			}
			encoded, err := json.Marshal(p.Data)
			if err != nil {
				return "", fmt.Errorf("a2a: marshal data part: %w", err)
			}
			appendSeparator()
			b.WriteString("```json\n")
			b.Write(encoded)
			b.WriteString("\n```")
		case PartKindFile:
			if p.File == nil {
				continue
			}
			appendSeparator()
			b.WriteString("[file")
			if p.File.Name != "" {
				b.WriteString(" name=")
				b.WriteString(p.File.Name)
			}
			if p.File.MimeType != "" {
				b.WriteString(" mime=")
				b.WriteString(p.File.MimeType)
			}
			switch {
			case p.File.URI != "":
				b.WriteString(" uri=")
				b.WriteString(p.File.URI)
			case p.File.Bytes != "":
				fmt.Fprintf(&b, " bytes=%d (base64)", len(p.File.Bytes))
			}
			b.WriteString("]")
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("a2a: message has no renderable content")
	}
	return b.String(), nil
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

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, id json.RawMessage, result any) {
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
