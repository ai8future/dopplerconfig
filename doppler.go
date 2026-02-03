// Package dopplerconfig provides a unified configuration management system
// using Doppler as the primary source, with fallback support for local files.
//
// # chassis-go Integration
//
// This package integrates with [github.com/ai8future/chassis-go] for resilient
// HTTP calls, structured logging, and test utilities.
//
// The DopplerProvider uses chassis-go's call.Client by default, providing
// automatic retries with exponential backoff and circuit breaking for
// Doppler API calls. Use [WithProviderLogger] to inject a chassis-go
// logz.New() logger (or any *slog.Logger) for structured logging.
//
// Config structs support both the "doppler" and "env" struct tags, allowing
// a single struct to work with both dopplerconfig and chassis-go's
// config.MustLoad:
//
//	type AppConfig struct {
//	    Port     int    `env:"PORT" doppler:"PORT" default:"8080" validate:"port"`
//	    LogLevel string `env:"LOG_LEVEL" doppler:"LOG_LEVEL" default:"info"`
//	}
//
// Use [LoadBootstrapWithChassis] to load bootstrap config via config.MustLoad,
// and [ValidateConfig] to validate structs loaded by either mechanism.
//
// Combined usage example:
//
//	bootstrap := dopplerconfig.LoadBootstrapWithChassis()
//	loader, _ := dopplerconfig.NewLoader[AppConfig](bootstrap)
//	cfg, _ := loader.Load(ctx)
//
//	if err := dopplerconfig.ValidateConfig(cfg); err != nil {
//	    panic(err)
//	}
//
//	logger := logz.New(cfg.LogLevel)
//
// # Secret Notes
//
// When adding secrets to Doppler, use the Secret Notes feature to document
// non-obvious information. Only add notes that provide value beyond what the
// secret name already conveys. Good notes include:
//
//   - Format requirements (e.g., "Format: user@realm!tokenname")
//   - Required pairings (e.g., "Must be paired with API_KEY")
//   - Permission requirements (e.g., "Needs Zone:Edit permission")
//   - Special handling (e.g., "Hashed with htpasswd for basic auth")
//
// Avoid notes that just restate the obvious (e.g., "API key for service X"
// when the secret is named SERVICE_X_API_KEY).
//
// Set notes via CLI:
//
//	doppler secrets notes set SECRET_NAME "Your note here" --project myproject
package dopplerconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"log/slog"
	"sync"
	"time"

	"github.com/ai8future/chassis-go/call"
)

const (
	// DefaultDopplerAPIURL is the default Doppler API endpoint.
	DefaultDopplerAPIURL = "https://api.doppler.com/v3"

	// DefaultTimeout is the default HTTP timeout for Doppler API calls.
	DefaultTimeout = 30 * time.Second

	// DefaultRetryAttempts is the default number of retry attempts for Doppler API calls.
	DefaultRetryAttempts = 3

	// DefaultRetryDelay is the default base delay between retries.
	DefaultRetryDelay = 1 * time.Second

	// DefaultBreakerThreshold is the number of consecutive failures before the circuit opens.
	DefaultBreakerThreshold = 5

	// DefaultBreakerReset is how long the circuit stays open before allowing a probe.
	DefaultBreakerReset = 30 * time.Second
)

// httpDoer is the interface satisfied by both call.Client and http.Client.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

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
// It uses chassis-go's call.Client for automatic retries with exponential
// backoff and circuit breaking to handle transient Doppler API failures.
type DopplerProvider struct {
	token   string
	project string
	config  string
	apiURL  string
	client  httpDoer
	breaker *call.CircuitBreaker
	logger  *slog.Logger
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

// WithHTTPClient sets a custom HTTP client, bypassing the default
// resilient call.Client. Use this for testing or when you need full
// control over HTTP behavior.
func WithHTTPClient(client *http.Client) DopplerProviderOption {
	return func(p *DopplerProvider) {
		p.client = client
		p.breaker = nil
	}
}

// WithProviderLogger sets the logger for the DopplerProvider.
// Compatible with any *slog.Logger, including chassis-go's logz.New().
func WithProviderLogger(logger *slog.Logger) DopplerProviderOption {
	return func(p *DopplerProvider) {
		p.logger = logger
	}
}

// WithCallOptions configures the underlying chassis-go call.Client
// with custom options (timeout, retry, circuit breaker settings).
// This replaces the default call.Client configuration.
func WithCallOptions(opts ...call.Option) DopplerProviderOption {
	return func(p *DopplerProvider) {
		p.client = call.New(opts...)
		p.breaker = nil // Custom options manage their own breaker
	}
}

// NewDopplerProvider creates a new Doppler API provider.
// The token can be either a service token (includes project/config) or a
// personal token (requires project and config parameters).
//
// By default, the provider uses chassis-go's call.Client with:
//   - 30s timeout per request
//   - 3 retry attempts with 1s base delay and exponential backoff
//   - Circuit breaker that opens after 5 consecutive failures
func NewDopplerProvider(token, project, config string, opts ...DopplerProviderOption) (*DopplerProvider, error) {
	if token == "" {
		return nil, fmt.Errorf("doppler token is required")
	}

	breaker := call.GetBreaker("doppler-api", DefaultBreakerThreshold, DefaultBreakerReset)

	p := &DopplerProvider{
		token:   token,
		project: project,
		config:  config,
		apiURL:  DefaultDopplerAPIURL,
		breaker: breaker,
		logger:  slog.Default(),
		client: call.New(
			call.WithTimeout(DefaultTimeout),
			call.WithRetry(DefaultRetryAttempts, DefaultRetryDelay),
			call.WithBreaker(breaker),
		),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

// CircuitState returns the current state of the Doppler API circuit breaker.
// Returns call.StateClosed if no circuit breaker is configured (e.g., when
// using WithHTTPClient).
//
// This is useful for health check integration:
//
//	if provider.CircuitState() == call.StateOpen {
//	    // Doppler is currently unavailable, using fallback
//	}
func (p *DopplerProvider) CircuitState() call.State {
	if p.breaker == nil {
		return call.StateClosed
	}
	return p.breaker.State()
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
		p.logger.Warn("doppler API request failed",
			"error", err,
			"project", project,
			"config", config,
		)
		return nil, fmt.Errorf("doppler API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle not modified (cache hit)
	if resp.StatusCode == http.StatusNotModified {
		p.logger.Debug("doppler cache hit (ETag match)",
			"project", project,
			"config", config,
		)
		p.mu.RLock()
		cached := make(map[string]string, len(p.cache))
		for k, v := range p.cache {
			cached[k] = v
		}
		p.mu.RUnlock()
		return cached, nil
	}

	if resp.StatusCode != http.StatusOK {
		// Limit error body read to 1KB to prevent memory issues and limit exposure
		const maxErrorBodySize = 1024
		limitedReader := io.LimitReader(resp.Body, maxErrorBodySize)
		body, _ := io.ReadAll(limitedReader)
		rawBody := string(body)
		if len(rawBody) >= maxErrorBodySize {
			rawBody = rawBody[:maxErrorBodySize-3] + "..."
		}
		return nil, &DopplerError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("API returned status %d", resp.StatusCode),
			Raw:        rawBody,
		}
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
// Uses errors.As for proper error chain unwrapping.
func IsDopplerError(err error) (*DopplerError, bool) {
	var de *DopplerError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}
