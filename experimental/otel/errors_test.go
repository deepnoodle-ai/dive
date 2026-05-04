package otel

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/deepnoodle-ai/dive/providers"
)

func TestClassifyChatError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"429", providers.NewError(429, "slow"), "rate_limit"},
		{"401", providers.NewError(401, "no key"), "auth"},
		{"403", providers.NewError(403, "forbidden"), "auth"},
		{"413", providers.NewError(413, "too long"), "context_length"},
		{"504", providers.NewError(504, "gateway"), "timeout"},
		{"408", providers.NewError(408, "req timeout"), "timeout"},
		{"ctx deadline", context.DeadlineExceeded, "timeout"},
		{"net timeout", &timeoutNetErr{}, "timeout"},
		{"net other", &nonTimeoutNetErr{}, "network"},
		{"context length text", errors.New("model context length exceeded"), "context_length"},
		{"rate text", errors.New("rate limit exceeded"), "rate_limit"},
		{"timeout text", errors.New("request timed out"), "timeout"},
		{"auth text", errors.New("invalid api key"), "auth"},
		{"unknown", errors.New("kerblam"), "_OTHER"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyChatError(c.err); got != c.want {
				t.Errorf("classifyChatError(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

func TestServerAttrs(t *testing.T) {
	cases := []struct {
		endpoint string
		address  string
		port     int
	}{
		{"https://api.anthropic.com/v1/messages", "api.anthropic.com", 0},
		{"https://api.example.com:8443/v1", "api.example.com", 8443},
		{"http://localhost:11434/api/chat", "localhost", 11434},
		{"", "", 0},
		{"://invalid", "", 0},
	}
	for _, c := range cases {
		attrs := serverAttrs(c.endpoint)
		var address string
		var port int
		for _, kv := range attrs {
			switch kv.Key {
			case "server.address":
				address = kv.Value.AsString()
			case "server.port":
				port = int(kv.Value.AsInt64())
			}
		}
		if address != c.address {
			t.Errorf("serverAttrs(%q) address = %q, want %q", c.endpoint, address, c.address)
		}
		if port != c.port {
			t.Errorf("serverAttrs(%q) port = %d, want %d", c.endpoint, port, c.port)
		}
	}
}

// timeoutNetErr is a minimal net.Error with Timeout() == true so the
// classifier's net-error branch can be exercised without standing up a
// real connection.
type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return false }

// nonTimeoutNetErr exercises the network bucket: a connection-level
// failure that isn't a timeout (refused, reset).
type nonTimeoutNetErr struct{}

func (nonTimeoutNetErr) Error() string   { return "connection refused" }
func (nonTimeoutNetErr) Timeout() bool   { return false }
func (nonTimeoutNetErr) Temporary() bool { return false }

// keep the net import used so the type interface check compiles.
var _ net.Error = timeoutNetErr{}
var _ net.Error = nonTimeoutNetErr{}
