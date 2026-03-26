package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPProviderOption configures an HTTPProvider.
type HTTPProviderOption func(*httpProviderConfig)

type httpProviderConfig struct {
	client  *http.Client
	timeout time.Duration
	logger  Logger
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) HTTPProviderOption {
	return func(cfg *httpProviderConfig) {
		cfg.client = c
	}
}

// WithHTTPTimeout sets the request timeout. Default is 30 seconds.
func WithHTTPTimeout(d time.Duration) HTTPProviderOption {
	return func(cfg *httpProviderConfig) {
		cfg.timeout = d
	}
}

// WithHTTPLogger sets a logger for the HTTP provider.
func WithHTTPLogger(l Logger) HTTPProviderOption {
	return func(cfg *httpProviderConfig) {
		cfg.logger = l
	}
}

// httpProvider implements Provider for HTTP-based skill loading.
type httpProvider struct {
	baseURL string
	config  httpProviderConfig
	mu      sync.Mutex
	etags   map[string]string // URL -> ETag cache
	cache   map[string][]byte // URL -> last successful body cache
}

// NewHTTPProvider creates a provider that fetches skills from an HTTP endpoint.
//
// Manifest mode: GET baseURL returns JSON {"skills": [{"name": "x", "url": "..."}, ...]}.
// Direct mode: baseURL points to a single SKILL.md file.
//
// Skills loaded from HTTP always have shell expansion disabled
// (SourceURI starts with "http").
func NewHTTPProvider(baseURL string, opts ...HTTPProviderOption) Provider {
	cfg := httpProviderConfig{
		timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.client == nil {
		cfg.client = &http.Client{Timeout: cfg.timeout}
	}
	return &httpProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		config:  cfg,
		etags:   make(map[string]string),
		cache:   make(map[string][]byte),
	}
}

func (p *httpProvider) Name() string {
	return "http"
}

func (p *httpProvider) Load(ctx context.Context) ([]*Skill, error) {
	// Try manifest mode first
	body, contentType, err := p.fetch(ctx, p.baseURL)
	if err != nil {
		p.logWarn("failed to fetch %s: %v", p.baseURL, err)
		return nil, nil
	}

	// If JSON, treat as manifest
	if strings.Contains(contentType, "json") {
		return p.loadManifest(ctx, body)
	}

	// Otherwise treat as a single skill file (direct mode)
	return p.loadDirect(body)
}

// skillManifest is the JSON response from a manifest endpoint.
type skillManifest struct {
	Skills []skillManifestEntry `json:"skills"`
}

type skillManifestEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (p *httpProvider) loadManifest(ctx context.Context, body []byte) ([]*Skill, error) {
	var manifest skillManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		p.logWarn("manifest JSON parse failed, trying direct mode: %v", err)
		return p.loadDirect(body)
	}

	baseURL, err := url.Parse(p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	var skills []*Skill
	for _, entry := range manifest.Skills {
		entryURL := entry.URL
		if !strings.HasPrefix(entryURL, "http") {
			// Resolve relative URL against base
			ref, err := url.Parse(entryURL)
			if err != nil {
				p.logWarn("invalid URL for skill %s: %v", entry.Name, err)
				continue
			}
			entryURL = baseURL.ResolveReference(ref).String()
		}

		skillBody, _, err := p.fetch(ctx, entryURL)
		if err != nil {
			p.logWarn("failed to fetch skill %s from %s: %v", entry.Name, entryURL, err)
			continue
		}

		s, err := ParseContent(skillBody, entry.Name+".md")
		if err != nil {
			p.logWarn("failed to parse skill %s: %v", entry.Name, err)
			continue
		}

		s.SourceURI = entryURL
		s.FilePath = ""
		skills = append(skills, s)
	}

	return skills, nil
}

func (p *httpProvider) loadDirect(body []byte) ([]*Skill, error) {
	s, err := ParseContent(body, "remote-skill.md")
	if err != nil {
		return nil, fmt.Errorf("parsing direct skill: %w", err)
	}
	s.SourceURI = p.baseURL
	s.FilePath = ""
	return []*Skill{s}, nil
}

func (p *httpProvider) fetch(ctx context.Context, fetchURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, "", err
	}

	// Add ETag for cache revalidation
	p.mu.Lock()
	if etag, ok := p.etags[fetchURL]; ok {
		req.Header.Set("If-None-Match", etag)
	}
	p.mu.Unlock()

	resp, err := p.config.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		// Return cached body if available
		p.mu.Lock()
		cached := p.cache[fetchURL]
		p.mu.Unlock()
		if cached != nil {
			return cached, "text/markdown", nil
		}
		return nil, "", fmt.Errorf("304 but no cached body for %s", fetchURL)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Store ETag and cache body
	p.mu.Lock()
	if etag := resp.Header.Get("ETag"); etag != "" {
		p.etags[fetchURL] = etag
	}
	p.cache[fetchURL] = body
	p.mu.Unlock()

	return body, resp.Header.Get("Content-Type"), nil
}

func (p *httpProvider) logWarn(format string, args ...any) {
	if p.config.logger != nil {
		p.config.logger.Warn(fmt.Sprintf(format, args...))
	}
}
