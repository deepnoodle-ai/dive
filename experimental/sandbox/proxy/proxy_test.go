package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIsAllowed(t *testing.T) {
	s := New([]string{"example.com", "google.com"}, false)

	tests := []struct {
		host    string
		allowed bool
	}{
		{"example.com", true},
		{"api.example.com", true},
		{"google.com", true},
		{"www.google.com", true},
		{"notallowed.com", false},
		{"com", false},
		{"example.com.evil.com", false},
		{"EXAMPLE.COM", true},  // Case insensitive
		{"example.com.", true}, // FQDN
	}

	for _, tt := range tests {
		if got := s.isAllowed(tt.host); got != tt.allowed {
			t.Errorf("isAllowed(%q) = %v, want %v", tt.host, got, tt.allowed)
		}
	}
}

func TestProxyHTTP(t *testing.T) {
	// Start a dummy target server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "target server reached")
	}))
	defer ts.Close()

	tsURL, _ := url.Parse(ts.URL)
	tsHost := tsURL.Hostname()

	// Start proxy allowing the target server
	proxy := New([]string{tsHost}, false)
	proxyAddr, err := proxy.Start()
	if err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}
	defer proxy.Stop()

	proxyURL, _ := url.Parse("http://" + proxyAddr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	// Test allowed request
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("failed to make request through proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	// Test disallowed request
	// Let's create a new proxy that allows NOTHING.
	proxyBlocked := New([]string{}, false)
	proxyBlockedAddr, _ := proxyBlocked.Start()
	defer proxyBlocked.Stop()

	proxyBlockedURL, _ := url.Parse("http://" + proxyBlockedAddr)
	clientBlocked := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyBlockedURL),
		},
	}

	respBlocked, err := clientBlocked.Get(ts.URL)
	if err == nil {
		if respBlocked.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", respBlocked.StatusCode)
		}
		respBlocked.Body.Close()
	}
}

func TestProxyConnect(t *testing.T) {
	// Testing CONNECT is harder without a real HTTPS server or manually doing CONNECT.
	// Let's manually do a CONNECT request.

	targetHost := "example.com"
	proxy := New([]string{targetHost}, false)
	proxyAddr, err := proxy.Start()
	if err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}
	defer proxy.Stop()

	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer conn.Close()

	// Write CONNECT request
	fmt.Fprintf(conn, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n\r\n", targetHost, targetHost)

	// Read response
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for allowed CONNECT, got %d", resp.StatusCode)
	}

	// Close and retry with blocked domain
	conn.Close()

	proxyBlocked := New([]string{}, false)
	proxyBlockedAddr, _ := proxyBlocked.Start()
	defer proxyBlocked.Stop()

	conn2, err := net.Dial("tcp", proxyBlockedAddr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	defer conn2.Close()

	fmt.Fprintf(conn2, "CONNECT %s:443 HTTP/1.1\r\nHost: %s:443\r\n\r\n", targetHost, targetHost)

	br2 := bufio.NewReader(conn2)
	resp2, err := http.ReadResponse(br2, nil)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp2.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for blocked CONNECT, got %d", resp2.StatusCode)
	}
}
