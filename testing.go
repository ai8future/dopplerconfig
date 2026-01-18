package dopplerconfig

import (
	"context"
	"fmt"
	"sync"
)

// MockProvider is a test provider that returns configured values.
// It implements the Provider interface for use in tests.
type MockProvider struct {
	mu       sync.RWMutex
	values   map[string]string
	projects map[string]map[string]string // project -> config values
	fetchErr error
	name     string
}

// NewMockProvider creates a new mock provider with the given values.
func NewMockProvider(values map[string]string) *MockProvider {
	return &MockProvider{
		values:   values,
		projects: make(map[string]map[string]string),
		name:     "mock",
	}
}

// NewMockProviderWithError creates a mock provider that returns an error.
func NewMockProviderWithError(err error) *MockProvider {
	return &MockProvider{
		values:   nil,
		projects: make(map[string]map[string]string),
		fetchErr: err,
		name:     "mock",
	}
}

// Fetch returns the configured values.
func (p *MockProvider) Fetch(ctx context.Context) (map[string]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.fetchErr != nil {
		return nil, p.fetchErr
	}

	// Return a copy to prevent mutation
	result := make(map[string]string, len(p.values))
	for k, v := range p.values {
		result[k] = v
	}
	return result, nil
}

// FetchProject returns values for a specific project/config.
func (p *MockProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.fetchErr != nil {
		return nil, p.fetchErr
	}

	key := project + "/" + config
	if values, ok := p.projects[key]; ok {
		result := make(map[string]string, len(values))
		for k, v := range values {
			result[k] = v
		}
		return result, nil
	}

	// Fall back to default values
	return p.Fetch(ctx)
}

// Name returns the provider name.
func (p *MockProvider) Name() string {
	return p.name
}

// Close is a no-op for mock providers.
func (p *MockProvider) Close() error {
	return nil
}

// SetValue sets a single value in the mock.
func (p *MockProvider) SetValue(key, value string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.values == nil {
		p.values = make(map[string]string)
	}
	p.values[key] = value
}

// SetValues replaces all values in the mock.
func (p *MockProvider) SetValues(values map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values = values
}

// SetProjectValues sets values for a specific project/config.
func (p *MockProvider) SetProjectValues(project, config string, values map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.projects == nil {
		p.projects = make(map[string]map[string]string)
	}
	p.projects[project+"/"+config] = values
}

// SetError configures the mock to return an error on fetch.
func (p *MockProvider) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fetchErr = err
}

// Clear removes all values and errors.
func (p *MockProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values = make(map[string]string)
	p.projects = make(map[string]map[string]string)
	p.fetchErr = nil
}

// TestBootstrap creates a BootstrapConfig for testing with sensible defaults.
func TestBootstrap() BootstrapConfig {
	return BootstrapConfig{
		Token:         "test-token",
		Project:       "test-project",
		Config:        "test",
		FailurePolicy: FailurePolicyFail,
	}
}

// TestLoader creates a Loader for testing with a MockProvider.
func TestLoader[T any](values map[string]string) (Loader[T], *MockProvider) {
	mock := NewMockProvider(values)
	loader := NewLoaderWithProvider[T](mock, nil)
	return loader, mock
}

// TestLoaderWithConfig creates a Loader with initial config loading.
func TestLoaderWithConfig[T any](values map[string]string) (Loader[T], *MockProvider, *T, error) {
	loader, mock := TestLoader[T](values)
	cfg, err := loader.Load(context.Background())
	return loader, mock, cfg, err
}

// AssertConfigEqual is a helper to compare two configs in tests.
// Returns an error if they are not equal.
func AssertConfigEqual[T comparable](expected, actual T) error {
	if expected != actual {
		return fmt.Errorf("config mismatch: expected %+v, got %+v", expected, actual)
	}
	return nil
}

// RecordingProvider wraps another provider and records all fetch calls.
// Useful for testing that the loader calls the provider correctly.
type RecordingProvider struct {
	provider Provider
	mu       sync.Mutex
	calls    []FetchCall
}

// FetchCall records a single fetch invocation.
type FetchCall struct {
	Project string
	Config  string
	Values  map[string]string
	Error   error
}

// NewRecordingProvider wraps a provider to record calls.
func NewRecordingProvider(provider Provider) *RecordingProvider {
	return &RecordingProvider{
		provider: provider,
	}
}

// Fetch delegates to the wrapped provider and records the call.
func (p *RecordingProvider) Fetch(ctx context.Context) (map[string]string, error) {
	values, err := p.provider.Fetch(ctx)
	p.mu.Lock()
	p.calls = append(p.calls, FetchCall{
		Values: values,
		Error:  err,
	})
	p.mu.Unlock()
	return values, err
}

// FetchProject delegates to the wrapped provider and records the call.
func (p *RecordingProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
	values, err := p.provider.FetchProject(ctx, project, config)
	p.mu.Lock()
	p.calls = append(p.calls, FetchCall{
		Project: project,
		Config:  config,
		Values:  values,
		Error:   err,
	})
	p.mu.Unlock()
	return values, err
}

// Name returns the wrapped provider's name.
func (p *RecordingProvider) Name() string {
	return "recording:" + p.provider.Name()
}

// Close delegates to the wrapped provider.
func (p *RecordingProvider) Close() error {
	return p.provider.Close()
}

// Calls returns all recorded fetch calls.
func (p *RecordingProvider) Calls() []FetchCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	calls := make([]FetchCall, len(p.calls))
	copy(calls, p.calls)
	return calls
}

// CallCount returns the number of fetch calls made.
func (p *RecordingProvider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// Reset clears all recorded calls.
func (p *RecordingProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = nil
}
