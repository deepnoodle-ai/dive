package toolkit

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/fetch"
	"github.com/deepnoodle-ai/wonton/retry"
	"github.com/deepnoodle-ai/wonton/schema"
)

const (
	DefaultFetchMaxSize    = 1024 * 500 // 500k runes
	DefaultFetchMaxRetries = 1
	DefaultFetchTimeout    = 15 * time.Second
)

var DefaultFetchExcludeTags = []string{
	"script",
	"style",
	"hr",
	"noscript",
	"iframe",
	"select",
	"input",
	"button",
	"svg",
	"form",
	"header",
	"nav",
	"footer",
}

var _ dive.TypedTool[*fetch.Request] = &FetchTool{}
var _ dive.TypedToolPreviewer[*fetch.Request] = &FetchTool{}

type FetchTool struct {
	fetcher         fetch.Fetcher
	maxSize         int
	maxRetries      int
	timeout         time.Duration
	onlyMainContent bool
}

type FetchToolOptions struct {
	MaxSize         int           `json:"max_size,omitempty"`
	MaxRetries      int           `json:"max_retries,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	OnlyMainContent bool          `json:"only_main_content,omitempty"`
	Fetcher         fetch.Fetcher `json:"-"`
}

func NewFetchTool(options FetchToolOptions) *dive.TypedToolAdapter[*fetch.Request] {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFetchMaxSize
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultFetchTimeout
	}
	return dive.ToolAdapter(&FetchTool{
		fetcher:         options.Fetcher,
		maxSize:         options.MaxSize,
		maxRetries:      options.MaxRetries,
		timeout:         options.Timeout,
		onlyMainContent: options.OnlyMainContent,
	})
}

func (t *FetchTool) Name() string {
	return "WebFetch"
}

func (t *FetchTool) Description() string {
	return "Fetches the contents of the webpage at the given URL."
}

func (t *FetchTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"url"},
		Properties: map[string]*schema.Property{
			"url": {
				Type:        "string",
				Description: "The URL of the webpage to fetch, e.g. https://www.example.com",
			},
		},
	}
}

func (t *FetchTool) PreviewCall(ctx context.Context, req *fetch.Request) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Fetch %s", req.URL),
	}
}

func (t *FetchTool) Call(ctx context.Context, req *fetch.Request) (*dive.ToolResult, error) {
	// Validate URL to prevent SSRF attacks
	if err := validateFetchURL(req.URL); err != nil {
		return NewToolResultError(fmt.Sprintf("URL validation failed: %s", err)), nil
	}

	req.Formats = []string{"markdown"}

	if req.ExcludeTags == nil {
		req.ExcludeTags = DefaultFetchExcludeTags
	}
	if t.onlyMainContent {
		req.OnlyMainContent = true
	}

	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	var response *fetch.Response
	err := retry.DoSimple(ctx, func() error {
		var err error
		response, err = t.fetcher.Fetch(ctx, req)
		if err != nil {
			return err
		}
		return nil
	}, retry.WithMaxAttempts(t.maxRetries+1))

	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to fetch url after %d attempts: %s", t.maxRetries, err)), nil
	}

	var sb strings.Builder
	if value := response.Metadata.Title; value != "" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", value))
	}
	if value := response.Metadata.Description; value != "" {
		sb.WriteString(fmt.Sprintf("## %s\n\n", value))
	}
	sb.WriteString(response.Markdown)

	content := truncateText(sb.String(), t.maxSize)
	contentLen := len([]rune(content))

	// Build display summary
	display := fmt.Sprintf("Fetched %s", req.URL)
	if title := response.Metadata.Title; title != "" {
		display = fmt.Sprintf("Fetched %s (%s)", req.URL, title)
	}
	display = fmt.Sprintf("%s - %d chars", display, contentLen)

	return NewToolResultText(content).WithDisplay(display), nil
}

func (t *FetchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "WebFetch",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   true,
	}
}

func truncateText(text string, maxSize int) string {
	runes := []rune(text)
	if len(runes) <= maxSize {
		return text
	}
	return string(runes[:maxSize]) + "..."
}

// isPrivateIP checks if an IP address is in a private or reserved range
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// Check for loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private ranges
	if ip.IsPrivate() {
		return true
	}

	// Check for unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	// Additional check for IPv4-mapped IPv6 addresses
	if ip4 := ip.To4(); ip4 != nil {
		// Check if it starts with 0 (0.0.0.0/8)
		if ip4[0] == 0 {
			return true
		}
	}

	return false
}

// validateFetchURL validates a URL for safe fetching, preventing SSRF attacks
func validateFetchURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("invalid URL scheme %q: only http and https are allowed", parsedURL.Scheme)
	}

	// Get the hostname
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL must include a hostname")
	}

	// Block localhost variations
	lowerHost := strings.ToLower(hostname)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
		return fmt.Errorf("access to localhost is not allowed")
	}

	// Resolve hostname to IP addresses
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Fail closed: DNS resolution failure blocks the request
		return fmt.Errorf("DNS resolution failed for %q: %w", hostname, err)
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("access to private/internal IP address %s is not allowed", ip.String())
		}
	}

	return nil
}
