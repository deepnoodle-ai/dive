package a2alib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
)

// WellKnownAgentCardPath is the canonical well-known URL path for the agent card.
const WellKnownAgentCardPath = "/.well-known/agent-card.json"

// AgentCardProvider resolves an agent card on demand. It is called on every
// request to /.well-known/agent-card.json, so the card can reflect dynamic
// state (e.g. available tools, authentication requirements, version metadata)
// without restarting the server. Callers wanting caching should wrap their
// implementation themselves.
type AgentCardProvider func(ctx context.Context) (*a2a.AgentCard, error)

// ServerOptions configures a Server.
type ServerOptions struct {
	// Agent is the Dive agent to expose via A2A. Required.
	Agent *dive.Agent

	// Card provides static agent card fields. The server fills in defaults
	// for any missing required fields. Ignored when CardProvider is set.
	Card a2a.AgentCard

	// CardProvider resolves the agent card dynamically on each request to
	// /.well-known/agent-card.json. When set, Card is ignored. The provider
	// is called with the incoming request context.
	CardProvider AgentCardProvider

	// BaseURL is the public URL for this server. Used to populate the card's
	// SupportedInterfaces when the card does not already have them set.
	// Optional.
	BaseURL string

	// SessionProvider supplies Dive sessions for context IDs. Optional.
	SessionProvider SessionProvider

	// Transport selects the A2A transport protocol. Defaults to "jsonrpc".
	// Supported: "jsonrpc", "rest".
	Transport string
}

// Server exposes a Dive agent as an A2A endpoint using the a2a-go SDK.
type Server struct {
	handler      a2asrv.RequestHandler
	httpHandler  http.Handler
	card         *a2a.AgentCard
	cardProvider AgentCardProvider
}

// NewServer constructs a Server from the given options.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Agent == nil {
		return nil, fmt.Errorf("a2alib: ServerOptions.Agent is required")
	}
	if opts.Transport == "" {
		opts.Transport = "jsonrpc"
	}

	// Build executor.
	var execOpts []ExecutorOption
	if opts.SessionProvider != nil {
		execOpts = append(execOpts, WithSessionProvider(opts.SessionProvider))
	}
	executor := NewExecutor(opts.Agent, execOpts...)

	// Build the a2a-go request handler.
	handlerOpts := []a2asrv.RequestHandlerOption{
		a2asrv.WithCapabilityChecks(&a2a.AgentCapabilities{
			Streaming: true,
		}),
	}
	handler := a2asrv.NewHandler(executor, handlerOpts...)

	// Build the HTTP handler with the selected transport.
	var transportHandler http.Handler
	switch opts.Transport {
	case "jsonrpc":
		transportHandler = a2asrv.NewJSONRPCHandler(handler)
	case "rest":
		transportHandler = a2asrv.NewRESTHandler(handler)
	default:
		return nil, fmt.Errorf("a2alib: unsupported transport: %q", opts.Transport)
	}

	srv := &Server{
		handler:      handler,
		httpHandler:  transportHandler,
		cardProvider: opts.CardProvider,
	}

	if opts.CardProvider == nil {
		srv.card = buildStaticCard(opts.Agent, opts.BaseURL, opts.Transport, &opts.Card)
	}

	return srv, nil
}

// buildStaticCard applies defaults to a caller-supplied card and returns the
// finalized copy. It does not modify the caller's value.
func buildStaticCard(agent *dive.Agent, baseURL, transport string, src *a2a.AgentCard) *a2a.AgentCard {
	card := deepCopyCard(src)

	if card.Name == "" {
		card.Name = agent.Name()
		if card.Name == "" {
			card.Name = "dive-agent"
		}
	}
	if card.Description == "" {
		card.Description = card.Name + " (Dive A2A agent)"
	}
	if card.Version == "" {
		card.Version = "0.1.0"
	}
	if len(card.DefaultInputModes) == 0 {
		card.DefaultInputModes = []string{"text/plain"}
	}
	if len(card.DefaultOutputModes) == 0 {
		card.DefaultOutputModes = []string{"text/plain"}
	}
	if len(card.Skills) == 0 {
		card.Skills = []a2a.AgentSkill{{
			ID:          "chat",
			Name:        "chat",
			Description: "General purpose conversational interface.",
			Tags:        []string{"chat"},
		}}
	}
	if len(card.SupportedInterfaces) == 0 && baseURL != "" {
		url := strings.TrimRight(baseURL, "/")
		proto := a2a.TransportProtocolJSONRPC
		if transport == "rest" {
			proto = a2a.TransportProtocolHTTPJSON
		}
		card.SupportedInterfaces = []*a2a.AgentInterface{{
			URL:             url,
			ProtocolBinding: proto,
			ProtocolVersion: a2a.Version,
		}}
	}
	card.Capabilities.Streaming = true
	return card
}

// Handler returns an http.Handler that serves both the agent card endpoint and
// the A2A protocol endpoint. Mount this at the root of your HTTP server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.cardProvider != nil {
		mux.Handle(WellKnownAgentCardPath, http.HandlerFunc(s.serveCard))
	} else {
		mux.Handle(WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(s.card))
	}
	mux.Handle("/", s.httpHandler)
	return mux
}

// serveCard handles /.well-known/agent-card.json using the dynamic provider.
func (s *Server) serveCard(w http.ResponseWriter, r *http.Request) {
	card, err := s.cardProvider(r.Context())
	if err != nil {
		http.Error(w, "failed to resolve agent card: "+err.Error(), http.StatusInternalServerError)
		return
	}
	b, err := json.Marshal(card)
	if err != nil {
		http.Error(w, "failed to marshal agent card", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b) //nolint:errcheck
}

// Card returns a deep copy of the server's static agent card. Returns nil when
// the server was created with a CardProvider — callers should invoke the
// provider directly in that case.
func (s *Server) Card() *a2a.AgentCard {
	if s.card == nil {
		return nil
	}
	b, err := json.Marshal(s.card)
	if err != nil {
		// Should never happen — we marshaled successfully in buildStaticCard.
		cp := *s.card
		return &cp
	}
	var cp a2a.AgentCard
	if err := json.Unmarshal(b, &cp); err != nil {
		cp2 := *s.card
		return &cp2
	}
	return &cp
}

// deepCopyCard returns a deep copy of an AgentCard via JSON round-trip.
// Falls back to a shallow copy if marshaling fails.
func deepCopyCard(src *a2a.AgentCard) *a2a.AgentCard {
	if src == nil {
		return &a2a.AgentCard{}
	}
	b, err := json.Marshal(src)
	if err != nil {
		cp := *src
		return &cp
	}
	var cp a2a.AgentCard
	if err := json.Unmarshal(b, &cp); err != nil {
		cp2 := *src
		return &cp2
	}
	return &cp
}
