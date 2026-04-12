package a2alib

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/deepnoodle-ai/dive"
)

// WellKnownAgentCardPath is the canonical well-known URL path for the agent card.
const WellKnownAgentCardPath = "/.well-known/agent-card.json"

// ServerOptions configures a Server.
type ServerOptions struct {
	// Agent is the Dive agent to expose via A2A. Required.
	Agent *dive.Agent

	// Card provides static agent card fields. The server fills in defaults
	// for any missing required fields.
	Card a2a.AgentCard

	// BaseURL is the public URL for this server. Used to populate the card's
	// SupportedInterfaces. Optional.
	BaseURL string

	// SessionProvider supplies Dive sessions for context IDs. Optional.
	SessionProvider SessionProvider

	// Transport selects the A2A transport protocol. Defaults to "jsonrpc".
	// Supported: "jsonrpc", "rest".
	Transport string
}

// Server exposes a Dive agent as an A2A endpoint using the a2a-go SDK.
type Server struct {
	handler     a2asrv.RequestHandler
	httpHandler http.Handler
	card        *a2a.AgentCard
}

// NewServer constructs a Server from the given options.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Agent == nil {
		return nil, fmt.Errorf("a2alib: ServerOptions.Agent is required")
	}
	if opts.Transport == "" {
		opts.Transport = "jsonrpc"
	}

	// Build the agent card with defaults.
	card := opts.Card
	if card.Name == "" {
		card.Name = opts.Agent.Name()
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

	// Set up the supported interface based on transport and base URL.
	if len(card.SupportedInterfaces) == 0 && opts.BaseURL != "" {
		url := strings.TrimRight(opts.BaseURL, "/")
		proto := a2a.TransportProtocolJSONRPC
		if opts.Transport == "rest" {
			proto = a2a.TransportProtocolHTTPJSON
		}
		card.SupportedInterfaces = []*a2a.AgentInterface{{
			URL:             url,
			ProtocolBinding: proto,
			ProtocolVersion: a2a.Version,
		}}
	}

	// Capabilities.
	card.Capabilities.Streaming = true

	// Build the executor.
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

	return &Server{
		handler:     handler,
		httpHandler: transportHandler,
		card:        &card,
	}, nil
}

// Handler returns an http.Handler that serves both the agent card and the
// A2A protocol endpoint. Mount this at the root of your HTTP server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(s.card))
	mux.Handle("/", s.httpHandler)
	return mux
}

// Card returns a copy of the server's agent card.
func (s *Server) Card() *a2a.AgentCard {
	return s.card
}
