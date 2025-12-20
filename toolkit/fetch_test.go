package toolkit

import (
	"net"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Loopback addresses
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 loopback other", "127.0.0.2", true},
		{"IPv6 loopback", "::1", true},

		// Private ranges (RFC 1918)
		{"10.0.0.0/8 start", "10.0.0.1", true},
		{"10.0.0.0/8 middle", "10.100.50.25", true},
		{"10.0.0.0/8 end", "10.255.255.254", true},
		{"172.16.0.0/12 start", "172.16.0.1", true},
		{"172.16.0.0/12 middle", "172.20.10.5", true},
		{"172.16.0.0/12 end", "172.31.255.254", true},
		{"192.168.0.0/16 start", "192.168.0.1", true},
		{"192.168.0.0/16 middle", "192.168.100.50", true},
		{"192.168.0.0/16 end", "192.168.255.254", true},

		// Link-local addresses
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv4 link-local AWS metadata", "169.254.169.254", true},
		{"IPv6 link-local", "fe80::1", true},

		// Unspecified addresses
		{"IPv4 unspecified", "0.0.0.0", true},
		{"IPv6 unspecified", "::", true},

		// 0.0.0.0/8 range
		{"0.x.x.x address", "0.1.2.3", true},

		// Public addresses - should NOT be private
		{"Google DNS", "8.8.8.8", false},
		{"Cloudflare DNS", "1.1.1.1", false},
		{"Public IP 1", "203.0.113.1", false},
		{"Public IP 2", "198.51.100.1", false},
		{"IPv6 public", "2001:4860:4860::8888", false},

		// Edge cases around private ranges
		{"Just before 10.0.0.0", "9.255.255.255", false},
		{"Just after 10.255.255.255", "11.0.0.0", false},
		{"Just before 172.16.0.0", "172.15.255.255", false},
		{"Just after 172.31.255.255", "172.32.0.0", false},
		{"Just before 192.168.0.0", "192.167.255.255", false},
		{"Just after 192.168.255.255", "192.169.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			assert.NotNil(t, ip, "Failed to parse IP: %s", tt.ip)
			result := isPrivateIP(ip)
			assert.Equal(t, tt.expected, result, "isPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
		})
	}
}

func TestIsPrivateIP_NilIP(t *testing.T) {
	result := isPrivateIP(nil)
	assert.False(t, result, "isPrivateIP(nil) should return false")
}

func TestValidateFetchURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		// Valid public URLs
		{"Valid HTTPS URL", "https://example.com/page", false, ""},
		{"Valid HTTP URL", "http://example.com/page", false, ""},
		{"Valid URL with port", "https://example.com:8080/api", false, ""},
		{"Valid URL with path", "https://example.com/path/to/resource", false, ""},
		{"Valid URL with query", "https://example.com/search?q=test", false, ""},

		// Invalid schemes
		{"File scheme", "file:///etc/passwd", true, "only http and https"},
		{"FTP scheme", "ftp://ftp.example.com", true, "only http and https"},
		{"JavaScript scheme", "javascript:alert(1)", true, "only http and https"},
		{"Data scheme", "data:text/html,<script>alert(1)</script>", true, "only http and https"},
		{"No scheme", "example.com", true, "invalid URL scheme"},

		// Localhost variations
		{"localhost", "http://localhost/admin", true, "localhost is not allowed"},
		{"localhost with port", "http://localhost:8080/admin", true, "localhost is not allowed"},
		{"subdomain of localhost", "http://foo.localhost/admin", true, "localhost is not allowed"},

		// Empty/invalid URLs
		{"Empty URL", "", true, "invalid URL"},
		{"No hostname", "http:///path", true, "must include a hostname"},

		// Note: Private IP tests may fail if DNS doesn't resolve,
		// so we test the validateFetchURL function's scheme/localhost checks here.
		// The isPrivateIP function is tested separately above.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFetchURL(tt.url)
			if tt.expectError {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tt.url)
			}
		})
	}
}

func TestValidateFetchURL_PrivateIPs(t *testing.T) {
	// These tests validate that URLs with IP addresses that resolve to private IPs are blocked
	// Note: These use literal IP addresses, so DNS resolution isn't needed

	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		// Loopback IPs
		{"IPv4 loopback", "http://127.0.0.1/", true, "private/internal IP"},
		{"IPv4 loopback with port", "http://127.0.0.1:8080/admin", true, "private/internal IP"},

		// Private range IPs
		{"10.x.x.x IP", "http://10.0.0.1/internal", true, "private/internal IP"},
		{"172.16.x.x IP", "http://172.16.0.1/internal", true, "private/internal IP"},
		{"192.168.x.x IP", "http://192.168.1.1/router", true, "private/internal IP"},

		// AWS metadata endpoint
		{"AWS metadata", "http://169.254.169.254/latest/meta-data/", true, "private/internal IP"},
		{"AWS metadata with path", "http://169.254.169.254/latest/meta-data/iam/security-credentials/", true, "private/internal IP"},

		// Public IPs should work
		{"Public IP", "http://8.8.8.8/", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFetchURL(tt.url)
			if tt.expectError {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tt.url)
			}
		})
	}
}

func TestValidateFetchURL_IPv6(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorMsg    string
	}{
		// IPv6 loopback
		{"IPv6 loopback", "http://[::1]/", true, "private/internal IP"},
		{"IPv6 loopback with port", "http://[::1]:8080/", true, "private/internal IP"},

		// IPv6 link-local
		{"IPv6 link-local", "http://[fe80::1]/", true, "private/internal IP"},

		// IPv6 public (Google DNS)
		{"IPv6 public", "http://[2001:4860:4860::8888]/", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFetchURL(tt.url)
			if tt.expectError {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tt.url)
			}
		})
	}
}
