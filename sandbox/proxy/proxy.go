// Package proxy provides a simple HTTP/HTTPS proxy that enforces a domain allowlist.
// It is intended to be used within the sandbox to restrict network access for
// sandboxed processes.
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is a simple HTTP/HTTPS proxy that enforces a domain allowlist.
// It supports both standard HTTP proxy requests and HTTPS CONNECT tunnels.
type Server struct {
	// AllowedDomains is the list of domains allowed to be accessed via the proxy.
	// Matching is case-insensitive and supports subdomains (e.g. "example.com" allows "api.example.com").
	AllowedDomains []string

	// AuditLog controls whether allowed/blocked requests are logged to standard output.
	AuditLog bool

	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
	running  bool
}

// New creates a new proxy server with the given allowlist and audit log setting.
func New(allowedDomains []string, auditLog bool) *Server {
	return &Server{
		AllowedDomains: allowedDomains,
		AuditLog:       auditLog,
	}
}

// Start starts the proxy server on a random local port.
// It returns the address (e.g., "127.0.0.1:12345") or an error.
// The server runs in a background goroutine until Stop is called.
func (s *Server) Start() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return s.listener.Addr().String(), nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to bind to local port: %w", err)
	}
	s.listener = ln

	s.server = &http.Server{
		Handler: s,
	}

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			if s.AuditLog {
				log.Printf("proxy server error: %v", err)
			}
		}
	}()

	s.running = true
	if s.AuditLog {
		log.Printf("proxy: started on %s with allowed domains: %v", ln.Addr(), s.AllowedDomains)
	}
	return ln.Addr().String(), nil
}

// Stop stops the proxy server and closes the listener.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown proxy server: %w", err)
	}

	s.running = false
	if s.AuditLog {
		log.Printf("proxy: stopped")
	}
	return nil
}

// ServeHTTP handles HTTP and CONNECT requests, enforcing the domain allowlist.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
	} else {
		s.handleHTTP(w, r)
	}
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Remove port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if !s.isAllowed(host) {
		if s.AuditLog {
			log.Printf("proxy: blocked CONNECT to %s", host)
		}
		http.Error(w, "Forbidden by sandbox policy", http.StatusForbidden)
		return
	}

	if s.AuditLog {
		log.Printf("proxy: allowed CONNECT to %s", host)
	}

	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
}

func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if host == "" {
		host = r.Host
	}
	// Handle host:port case for r.Host if r.URL.Hostname() was empty
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if !s.isAllowed(host) {
		if s.AuditLog {
			log.Printf("proxy: blocked HTTP request to %s", host)
		}
		http.Error(w, "Forbidden by sandbox policy", http.StatusForbidden)
		return
	}

	if s.AuditLog {
		log.Printf("proxy: allowed HTTP request to %s", host)
	}

	// Prepare the request to be sent to the target
	r.RequestURI = "" // RequestURI must be empty for client requests

	// We need to use a new transport or the default one
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *Server) isAllowed(host string) bool {
	host = strings.ToLower(host)
	// Remove trailing dot if present (FQDN)
	host = strings.TrimSuffix(host, ".")

	for _, domain := range s.AllowedDomains {
		domain = strings.ToLower(domain)
		domain = strings.TrimSuffix(domain, ".")

		if host == domain {
			return true
		}
		if strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// transfer copies data between two connections.
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}
