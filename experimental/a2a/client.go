package a2a

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
)

// Client is a low-level client for an A2A agent. It speaks JSON-RPC 2.0
// over HTTP and knows how to fetch an agent card from the well-known
// path (/.well-known/agent-card.json, falling back to the legacy
// /.well-known/agent.json for older servers).
//
// The Client wraps an *http.Client and a base endpoint URL. All public
// methods are safe for concurrent use.
type Client struct {
	endpoint        string
	cardURL         string
	legacyCardURL   string
	cardURLOverride bool
	http            *http.Client
	headers         http.Header
	reqID           atomic.Int64
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// Endpoint is the base URL of the A2A agent's JSON-RPC endpoint.
	// Required.
	Endpoint string

	// CardURL overrides the URL used when fetching the agent card. If
	// empty, the well-known path is derived from Endpoint's origin.
	CardURL string

	// HTTPClient is used for all outbound requests. Defaults to
	// http.DefaultClient.
	HTTPClient *http.Client

	// Headers are added to every outbound request. Useful for passing
	// bearer tokens or custom auth headers.
	Headers http.Header
}

// NewClient returns a new Client configured for the given endpoint.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.Endpoint == "" {
		return nil, fmt.Errorf("a2a: ClientOptions.Endpoint is required")
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	override := opts.CardURL != ""
	cardURL := opts.CardURL
	if cardURL == "" {
		cardURL = deriveCardURL(opts.Endpoint, DefaultAgentCardPath)
	}
	return &Client{
		endpoint:        opts.Endpoint,
		cardURL:         cardURL,
		legacyCardURL:   deriveCardURL(opts.Endpoint, LegacyAgentCardPath),
		cardURLOverride: override,
		http:            httpClient,
		headers:         opts.Headers,
	}, nil
}

// deriveCardURL returns the well-known agent card URL for a given
// endpoint at the supplied well-known path, preserving scheme and
// authority.
func deriveCardURL(endpoint, path string) string {
	schemeIdx := strings.Index(endpoint, "://")
	if schemeIdx < 0 {
		return strings.TrimRight(endpoint, "/") + path
	}
	hostStart := schemeIdx + 3
	pathIdx := strings.Index(endpoint[hostStart:], "/")
	if pathIdx < 0 {
		return endpoint + path
	}
	return endpoint[:hostStart+pathIdx] + path
}

// FetchCard retrieves the remote agent's card from the well-known URL.
// It tries the canonical /.well-known/agent-card.json path first; if that
// returns 404 (and the caller did not explicitly override CardURL) it
// falls back to the legacy /.well-known/agent.json path so older servers
// that have not migrated still work.
func (c *Client) FetchCard(ctx context.Context) (*AgentCard, error) {
	card, status, err := c.fetchCardAt(ctx, c.cardURL)
	if err == nil {
		return card, nil
	}
	if status == http.StatusNotFound && !c.cardURLOverride && c.legacyCardURL != c.cardURL {
		card, _, legacyErr := c.fetchCardAt(ctx, c.legacyCardURL)
		if legacyErr == nil {
			return card, nil
		}
	}
	return nil, err
}

func (c *Client) fetchCardAt(ctx context.Context, url string) (*AgentCard, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("a2a: fetch card: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("a2a: fetch card: %s: %s",
			resp.Status, strings.TrimSpace(string(body)))
	}
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("a2a: decode card: %w", err)
	}
	return &card, resp.StatusCode, nil
}

// SendMessage invokes message/send and returns the resulting Task.
//
// The A2A spec lets a server return either a Task or a bare Message on
// message/send. When a server replies with a Message, SendMessage
// synthesizes a minimal completed Task wrapping it so callers always
// receive a *Task. Inspect task.History and task.Artifacts to read the
// reply; task.Metadata["a2a.syntheticFromMessage"] is set to true so
// callers that care can tell the difference.
func (c *Client) SendMessage(ctx context.Context, msg *Message, cfg *MessageConfiguration) (*Task, error) {
	if msg == nil {
		return nil, fmt.Errorf("a2a: SendMessage: nil message")
	}
	if msg.MessageID == "" {
		msg.MessageID = uuid.NewString()
	}
	params := SendMessageParams{Message: msg, Configuration: cfg}
	var raw json.RawMessage
	if err := c.call(ctx, MethodMessageSend, params, &raw); err != nil {
		return nil, err
	}
	return decodeSendMessageResult(raw)
}

// decodeSendMessageResult distinguishes a Task from a bare Message result.
func decodeSendMessageResult(raw json.RawMessage) (*Task, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("a2a: message/send returned empty result")
	}
	// Probe for task-specific fields. A Task has "id" and "status";
	// a Message has "messageId" and "role".
	var probe struct {
		ID        string `json:"id"`
		MessageID string `json:"messageId"`
		Status    *struct {
			State string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("a2a: probe message/send result: %w", err)
	}
	if probe.MessageID != "" && probe.Status == nil {
		// Bare Message result.
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("a2a: decode message/send message result: %w", err)
		}
		return syntheticTaskFromMessage(&msg), nil
	}
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, fmt.Errorf("a2a: decode message/send task result: %w", err)
	}
	return &task, nil
}

// syntheticTaskFromMessage wraps a bare Message reply as a completed Task.
func syntheticTaskFromMessage(msg *Message) *Task {
	task := &Task{
		ID:        msg.TaskID,
		ContextID: msg.ContextID,
		Status: TaskStatus{
			State: TaskStateCompleted,
		},
		History: []*Message{msg},
		Metadata: map[string]any{
			"a2a.syntheticFromMessage": true,
		},
	}
	if task.ID == "" {
		task.ID = msg.MessageID
	}
	if text := msg.TextContent(); text != "" {
		task.Artifacts = []*Artifact{{
			ArtifactID: msg.MessageID,
			Name:       "response",
			Parts:      []Part{NewTextPart(text)},
		}}
	}
	return task
}

// GetTask invokes tasks/get.
func (c *Client) GetTask(ctx context.Context, id string) (*Task, error) {
	var task Task
	if err := c.call(ctx, MethodTasksGet, TaskIDParams{ID: id}, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CancelTask invokes tasks/cancel.
func (c *Client) CancelTask(ctx context.Context, id string) (*Task, error) {
	var task Task
	if err := c.call(ctx, MethodTasksCancel, TaskIDParams{ID: id}, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// StreamEvent is one element received from an A2A message/stream call.
// Exactly one of Task, StatusUpdate, or ArtifactUpdate is populated; the
// others are nil.
type StreamEvent struct {
	Task           *Task
	StatusUpdate   *TaskStatusUpdateEvent
	ArtifactUpdate *TaskArtifactUpdateEvent
}

// StreamMessage invokes message/stream and delivers server-sent events
// to the given callback. The call blocks until the stream ends, the
// context is canceled, or an error occurs.
func (c *Client) StreamMessage(ctx context.Context, msg *Message, cfg *MessageConfiguration, onEvent func(*StreamEvent) error) error {
	if msg == nil {
		return fmt.Errorf("a2a: StreamMessage: nil message")
	}
	if msg.MessageID == "" {
		msg.MessageID = uuid.NewString()
	}
	params := SendMessageParams{Message: msg, Configuration: cfg}
	body, err := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  MethodMessageStream,
		Params:  mustMarshal(params),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("a2a: stream: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return parseSSEStream(resp.Body, onEvent)
}

// call is the shared JSON-RPC round-trip helper.
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(RPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID(),
		Method:  method,
		Params:  mustMarshal(params),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("a2a: %s: %s: %s", method, resp.Status, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *RPCError       `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("a2a: decode %s response: %w", method, err)
	}
	if envelope.Error != nil {
		return envelope.Error
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

func (c *Client) applyHeaders(req *http.Request) {
	for k, vs := range c.headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
}

func (c *Client) nextID() json.RawMessage {
	n := c.reqID.Add(1)
	return json.RawMessage(fmt.Sprintf("%d", n))
}

// parseSSEStream consumes a server-sent events stream and invokes the
// callback for every decoded A2A stream event.
func parseSSEStream(r io.Reader, onEvent func(*StreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		var env struct {
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			return fmt.Errorf("a2a: decode stream event: %w", err)
		}
		if env.Error != nil {
			return env.Error
		}
		event, err := decodeStreamResult(env.Result)
		if err != nil {
			return err
		}
		if event == nil {
			continue
		}
		if err := onEvent(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// decodeStreamResult parses a StreamResponse envelope from the result
// field of a streamed JSON-RPC response. The A2A v1.0 format uses
// field-name discriminators: statusUpdate, artifactUpdate, task, message.
func decodeStreamResult(raw json.RawMessage) (*StreamEvent, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var sr StreamResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return nil, fmt.Errorf("a2a: decode stream result: %w", err)
	}
	switch {
	case sr.StatusUpdate != nil:
		return &StreamEvent{StatusUpdate: sr.StatusUpdate}, nil
	case sr.ArtifactUpdate != nil:
		return &StreamEvent{ArtifactUpdate: sr.ArtifactUpdate}, nil
	case sr.Task != nil:
		return &StreamEvent{Task: sr.Task}, nil
	case sr.Message != nil:
		// Wrap bare message as a synthetic task.
		return &StreamEvent{Task: syntheticTaskFromMessage(sr.Message)}, nil
	}
	return nil, nil
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
