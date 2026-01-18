// Package dopplerconfig provides a unified configuration management system
// using Doppler as the primary source, with fallback support for local files.
package dopplerconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultDopplerAPIURL is the default Doppler API endpoint.
	DefaultDopplerAPIURL = "https://api.doppler.com/v3"

	// DefaultTimeout is the default HTTP timeout for Doppler API calls.
	DefaultTimeout = 30 * time.Second
)

// Provider is the interface for config sources.
// Implementations include DopplerProvider, FileProvider, and MockProvider.
type Provider interface {
	// Fetch retrieves all config values from the source.
	// Returns a map of key -> value.
	Fetch(ctx context.Context) (map[string]string, error)

	// FetchProject retrieves config for a specific project/config combination.
	// Used for multi-tenant scenarios where each tenant has its own config.
	FetchProject(ctx context.Context, project, config string) (map[string]string, error)

	// Name returns a human-readable name for this provider.
	Name() string

	// Close releases any resources held by the provider.
	Close() error
}

// DopplerProvider fetches configuration directly from the Doppler API.
type DopplerProvider struct {
	token   string
	project string
	config  string
	apiURL  string
	client  *http.Client
	mu      sync.RWMutex
	cache   map[string]string
	etag    string
}

// DopplerProviderOption configures a DopplerProvider.
type DopplerProviderOption func(*DopplerProvider)

// WithAPIURL sets a custom Doppler API URL.
func WithAPIURL(url string) DopplerProviderOption {
	return func(p *DopplerProvider) {
		p.apiURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) DopplerProviderOption {
	return func(p *DopplerProvider) {
		p.client = client
	}
}

// NewDopplerProvider creates a new Doppler API provider.
// The token can be either a service token (includes project/config) or a
// personal token (requires project and config parameters).
func NewDopplerProvider(token, project, config string, opts ...DopplerProviderOption) (*DopplerProvider, error) {
	if token == "" {
		return nil, fmt.Errorf("doppler token is required")
	}

	p := &DopplerProvider{
		token:   token,
		project: project,
		config:  config,
		apiURL:  DefaultDopplerAPIURL,
		client: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

// dopplerSecretsResponse is the response from Doppler's /secrets endpoint.
type dopplerSecretsResponse struct {
	Secrets map[string]struct {
		Raw string `json:"raw"`
	} `json:"secrets"`
}

// Fetch retrieves all secrets from the configured Doppler project/config.
func (p *DopplerProvider) Fetch(ctx context.Context) (map[string]string, error) {
	return p.FetchProject(ctx, p.project, p.config)
}

// FetchProject retrieves secrets for a specific project/config.
func (p *DopplerProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
	url := fmt.Sprintf("%s/configs/config/secrets", p.apiURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters
	q := req.URL.Query()
	if project != "" {
		q.Add("project", project)
	}
	if config != "" {
		q.Add("config", config)
	}
	req.URL.RawQuery = q.Encode()

	// Set auth header
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")

	// Add ETag for caching if available
	p.mu.RLock()
	if p.etag != "" {
		req.Header.Set("If-None-Match", p.etag)
	}
	p.mu.RUnlock()

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doppler API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle not modified (cache hit)
	if resp.StatusCode == http.StatusNotModified {
		p.mu.RLock()
		cached := make(map[string]string, len(p.cache))
		for k, v := range p.cache {
			cached[k] = v
		}
		p.mu.RUnlock()
		return cached, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("doppler API returned status %d: %s", resp.StatusCode, string(body))
	}

	var dopplerResp dopplerSecretsResponse
	if err := json.NewDecoder(resp.Body).Decode(&dopplerResp); err != nil {
		return nil, fmt.Errorf("failed to decode doppler response: %w", err)
	}

	// Extract raw values
	result := make(map[string]string, len(dopplerResp.Secrets))
	for k, v := range dopplerResp.Secrets {
		result[k] = v.Raw
	}

	// Update cache with new ETag
	p.mu.Lock()
	p.cache = result
	if etag := resp.Header.Get("ETag"); etag != "" {
		p.etag = etag
	}
	p.mu.Unlock()

	return result, nil
}

// Name returns the provider name.
func (p *DopplerProvider) Name() string {
	return "doppler"
}

// Close releases resources.
func (p *DopplerProvider) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// DopplerError represents an error from the Doppler API.
type DopplerError struct {
	StatusCode int
	Message    string
	Raw        string
}

func (e *DopplerError) Error() string {
	return fmt.Sprintf("doppler error %d: %s", e.StatusCode, e.Message)
}

// IsDopplerError checks if an error is a Doppler API error.
func IsDopplerError(err error) (*DopplerError, bool) {
	if de, ok := err.(*DopplerError); ok {
		return de, true
	}
	return nil, false
}
