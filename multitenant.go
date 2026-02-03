package dopplerconfig

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// MultiTenantLoader provides configuration loading for multi-tenant systems.
// It supports a two-level config structure:
//   - E: Environment/shared config (loaded once)
//   - P: Project/tenant-specific config (loaded per-tenant)
//
// This matches the Solstice pattern with EnvConfig + ProjectConfig.
type MultiTenantLoader[E any, P any] interface {
	// LoadEnv loads the environment-level configuration.
	LoadEnv(ctx context.Context) (*E, error)

	// LoadProject loads configuration for a specific project/tenant.
	LoadProject(ctx context.Context, code string) (*P, error)

	// LoadAllProjects loads all project configurations.
	// The projectCodes parameter lists which projects to load.
	LoadAllProjects(ctx context.Context, projectCodes []string) (map[string]*P, error)

	// ReloadProjects reloads all project configurations and returns what changed.
	ReloadProjects(ctx context.Context) (*ReloadDiff, error)

	// Project returns a specific project config (from cache).
	Project(code string) (*P, bool)

	// Projects returns all cached project configs.
	Projects() map[string]*P

	// ProjectCodes returns a sorted list of loaded project codes.
	ProjectCodes() []string

	// Env returns the current environment config.
	Env() *E

	// OnEnvChange registers a callback for environment config changes.
	OnEnvChange(fn func(old, new *E))

	// OnProjectChange registers a callback for project config changes.
	OnProjectChange(fn func(diff *ReloadDiff))

	// Close releases resources.
	Close() error
}

// ReloadDiff describes what changed during a config reload.
type ReloadDiff struct {
	Added     []string // Project codes that were added
	Removed   []string // Project codes that were removed
	Unchanged []string // Project codes that remained (may have updated)
}

// multiTenantLoader implements MultiTenantLoader.
type multiTenantLoader[E any, P any] struct {
	provider  Provider
	fallback  Provider
	bootstrap BootstrapConfig

	mu          sync.RWMutex
	envConfig   *E
	projects    map[string]*P
	projectKeys []string // Sorted list of project codes

	envCallbacks     []func(old, new *E)
	projectCallbacks []func(diff *ReloadDiff)
}

// MultiTenantBootstrap extends BootstrapConfig for multi-tenant scenarios.
type MultiTenantBootstrap struct {
	BootstrapConfig
}

// NewMultiTenantLoader creates a new multi-tenant loader.
func NewMultiTenantLoader[E any, P any](bootstrap MultiTenantBootstrap) (MultiTenantLoader[E, P], error) {
	l := &multiTenantLoader[E, P]{
		bootstrap: bootstrap.BootstrapConfig,
		projects:  make(map[string]*P),
	}

	// Initialize primary provider (Doppler)
	if bootstrap.IsEnabled() {
		provider, err := NewDopplerProvider(bootstrap.Token, bootstrap.Project, bootstrap.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create doppler provider: %w", err)
		}
		l.provider = provider
	}

	// Initialize fallback provider (file)
	if bootstrap.HasFallback() {
		l.fallback = NewFileProvider(bootstrap.FallbackPath)
	}

	if l.provider == nil && l.fallback == nil {
		return nil, fmt.Errorf("no configuration source available")
	}

	return l, nil
}

// NewMultiTenantLoaderWithProvider creates a loader with custom providers.
func NewMultiTenantLoaderWithProvider[E any, P any](provider, fallback Provider) MultiTenantLoader[E, P] {
	return &multiTenantLoader[E, P]{
		provider: provider,
		fallback: fallback,
		projects: make(map[string]*P),
	}
}

// LoadEnv implements MultiTenantLoader.LoadEnv.
func (l *multiTenantLoader[E, P]) LoadEnv(ctx context.Context) (*E, error) {
	values, err := l.fetchWithFallback(ctx, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch env config: %w", err)
	}

	cfg := new(E)
	if _, err := unmarshalConfig(values, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse env config: %w", err)
	}

	l.mu.Lock()
	old := l.envConfig
	l.envConfig = cfg
	callbacks := l.envCallbacks
	l.mu.Unlock()

	// Notify callbacks
	if old != nil {
		for _, cb := range callbacks {
			cb(old, cfg)
		}
	}

	return cfg, nil
}

// LoadProject implements MultiTenantLoader.LoadProject.
func (l *multiTenantLoader[E, P]) LoadProject(ctx context.Context, code string) (*P, error) {
	values, err := l.fetchWithFallback(ctx, "", code)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch project config for %s: %w", code, err)
	}

	cfg := new(P)
	if _, err := unmarshalConfig(values, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse project config for %s: %w", code, err)
	}

	l.mu.Lock()
	l.projects[code] = cfg
	l.updateProjectKeys()
	l.mu.Unlock()

	return cfg, nil
}

// LoadAllProjects implements MultiTenantLoader.LoadAllProjects.
func (l *multiTenantLoader[E, P]) LoadAllProjects(ctx context.Context, projectCodes []string) (map[string]*P, error) {
	result := make(map[string]*P, len(projectCodes))

	for _, code := range projectCodes {
		cfg, err := l.LoadProject(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to load project %s: %w", code, err)
		}
		result[code] = cfg
	}

	return result, nil
}

// ReloadProjects implements MultiTenantLoader.ReloadProjects.
func (l *multiTenantLoader[E, P]) ReloadProjects(ctx context.Context) (*ReloadDiff, error) {
	l.mu.RLock()
	oldCodes := make(map[string]bool, len(l.projects))
	for code := range l.projects {
		oldCodes[code] = true
	}
	l.mu.RUnlock()

	// Reload each project
	newProjects := make(map[string]*P, len(oldCodes))
	var reloadErrors []string
	for code := range oldCodes {
		cfg, err := l.fetchAndParse(ctx, code)
		if err != nil {
			// Log warning but continue with other projects
			slog.Warn("failed to reload project config",
				"project", code,
				"error", err,
			)
			reloadErrors = append(reloadErrors, code)
			continue
		}
		newProjects[code] = cfg
	}

	// Log summary if any projects failed to reload
	if len(reloadErrors) > 0 {
		slog.Error("some projects failed to reload",
			"failed_count", len(reloadErrors),
			"failed_projects", reloadErrors,
		)
	}

	// Calculate diff
	diff := &ReloadDiff{
		Added:     make([]string, 0),
		Removed:   make([]string, 0),
		Unchanged: make([]string, 0),
	}

	for code := range newProjects {
		if oldCodes[code] {
			diff.Unchanged = append(diff.Unchanged, code)
			delete(oldCodes, code)
		} else {
			diff.Added = append(diff.Added, code)
		}
	}

	for code := range oldCodes {
		diff.Removed = append(diff.Removed, code)
	}

	// Sort for consistent output
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Strings(diff.Unchanged)

	// Apply changes
	l.mu.Lock()
	l.projects = newProjects
	l.updateProjectKeys()
	callbacks := l.projectCallbacks
	l.mu.Unlock()

	// Notify callbacks
	for _, cb := range callbacks {
		cb(diff)
	}

	return diff, nil
}

// Project implements MultiTenantLoader.Project.
func (l *multiTenantLoader[E, P]) Project(code string) (*P, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cfg, ok := l.projects[code]
	return cfg, ok
}

// Projects implements MultiTenantLoader.Projects.
func (l *multiTenantLoader[E, P]) Projects() map[string]*P {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make(map[string]*P, len(l.projects))
	for k, v := range l.projects {
		result[k] = v
	}
	return result
}

// ProjectCodes implements MultiTenantLoader.ProjectCodes.
func (l *multiTenantLoader[E, P]) ProjectCodes() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	codes := make([]string, len(l.projectKeys))
	copy(codes, l.projectKeys)
	return codes
}

// Env implements MultiTenantLoader.Env.
func (l *multiTenantLoader[E, P]) Env() *E {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.envConfig
}

// OnEnvChange implements MultiTenantLoader.OnEnvChange.
func (l *multiTenantLoader[E, P]) OnEnvChange(fn func(old, new *E)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.envCallbacks = append(l.envCallbacks, fn)
}

// OnProjectChange implements MultiTenantLoader.OnProjectChange.
func (l *multiTenantLoader[E, P]) OnProjectChange(fn func(diff *ReloadDiff)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.projectCallbacks = append(l.projectCallbacks, fn)
}

// Close implements MultiTenantLoader.Close.
func (l *multiTenantLoader[E, P]) Close() error {
	var errs []error
	if l.provider != nil {
		if err := l.provider.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if l.fallback != nil {
		if err := l.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

func (l *multiTenantLoader[E, P]) fetchWithFallback(ctx context.Context, project, config string) (map[string]string, error) {
	var values map[string]string
	var err error

	// Try primary provider first
	if l.provider != nil {
		values, err = l.provider.FetchProject(ctx, project, config)
		if err == nil {
			return values, nil
		}
	}

	// Fall back if primary failed
	if l.fallback != nil {
		values, err = l.fallback.FetchProject(ctx, project, config)
		if err == nil {
			return values, nil
		}
	}

	return nil, err
}

func (l *multiTenantLoader[E, P]) fetchAndParse(ctx context.Context, code string) (*P, error) {
	values, err := l.fetchWithFallback(ctx, "", code)
	if err != nil {
		return nil, err
	}

	cfg := new(P)
	if _, err := unmarshalConfig(values, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (l *multiTenantLoader[E, P]) updateProjectKeys() {
	keys := make([]string, 0, len(l.projects))
	for k := range l.projects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	l.projectKeys = keys
}

// MultiTenantWatcher watches both env and project configs.
type MultiTenantWatcher[E any, P any] struct {
	loader   MultiTenantLoader[E, P]
	interval time.Duration
	logger   *slog.Logger

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewMultiTenantWatcher creates a watcher for multi-tenant configs.
func NewMultiTenantWatcher[E any, P any](loader MultiTenantLoader[E, P], interval time.Duration) *MultiTenantWatcher[E, P] {
	return &MultiTenantWatcher[E, P]{
		loader:   loader,
		interval: interval,
		logger:   slog.Default(),
	}
}

// WithLogger sets the logger for the watcher.
func (w *MultiTenantWatcher[E, P]) WithLogger(logger *slog.Logger) *MultiTenantWatcher[E, P] {
	w.logger = logger
	return w
}

// Start begins watching for changes.
func (w *MultiTenantWatcher[E, P]) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.mu.Unlock()

	go w.run(ctx)
	return nil
}

// Stop stops watching.
func (w *MultiTenantWatcher[E, P]) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	close(w.stopCh)
	w.mu.Unlock()
	<-w.doneCh
}

func (w *MultiTenantWatcher[E, P]) run(ctx context.Context) {
	defer func() {
		w.mu.Lock()
		w.running = false
		close(w.doneCh)
		w.mu.Unlock()
	}()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("multi-tenant watcher stopping: context cancelled")
			return
		case <-w.stopCh:
			w.logger.Info("multi-tenant watcher stopping: stop requested")
			return
		case <-ticker.C:
			// Reload env config
			if _, err := w.loader.LoadEnv(ctx); err != nil {
				w.logger.Warn("failed to reload env config", "error", err)
			}
			// Reload project configs
			if _, err := w.loader.ReloadProjects(ctx); err != nil {
				w.logger.Warn("failed to reload project configs", "error", err)
			}
		}
	}
}
