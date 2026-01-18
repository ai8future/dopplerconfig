package dopplerconfig

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Watcher provides hot-reload functionality for configuration changes.
// It periodically polls the provider and triggers callbacks when changes are detected.
type Watcher[T any] struct {
	loader   Loader[T]
	interval time.Duration
	logger   *slog.Logger

	mu           sync.Mutex
	running      bool
	stopCh       chan struct{}
	doneCh       chan struct{}
	failureCount int
	maxFailures  int
}

// WatcherOption configures a Watcher.
type WatcherOption[T any] func(*Watcher[T])

// WithWatchInterval sets the polling interval.
func WithWatchInterval[T any](interval time.Duration) WatcherOption[T] {
	return func(w *Watcher[T]) {
		w.interval = interval
	}
}

// WithWatchLogger sets the logger for watch events.
func WithWatchLogger[T any](logger *slog.Logger) WatcherOption[T] {
	return func(w *Watcher[T]) {
		w.logger = logger
	}
}

// WithMaxFailures sets the maximum consecutive failures before stopping.
// Set to 0 for unlimited retries (default).
func WithMaxFailures[T any](max int) WatcherOption[T] {
	return func(w *Watcher[T]) {
		w.maxFailures = max
	}
}

// NewWatcher creates a new configuration watcher.
func NewWatcher[T any](loader Loader[T], opts ...WatcherOption[T]) *Watcher[T] {
	w := &Watcher[T]{
		loader:      loader,
		interval:    30 * time.Second,
		logger:      slog.Default(),
		maxFailures: 0, // Unlimited by default
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start begins watching for configuration changes.
// It runs in the background until Stop is called.
func (w *Watcher[T]) Start(ctx context.Context) error {
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

// Stop stops watching for configuration changes.
func (w *Watcher[T]) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	close(w.stopCh)
	w.mu.Unlock()

	// Wait for goroutine to finish
	<-w.doneCh
}

// IsRunning returns whether the watcher is currently running.
func (w *Watcher[T]) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *Watcher[T]) run(ctx context.Context) {
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
			w.logger.Info("watcher stopping: context cancelled")
			return
		case <-w.stopCh:
			w.logger.Info("watcher stopping: stop requested")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

func (w *Watcher[T]) poll(ctx context.Context) {
	_, err := w.loader.Reload(ctx)
	if err != nil {
		w.mu.Lock()
		w.failureCount++
		failures := w.failureCount
		maxFail := w.maxFailures
		w.mu.Unlock()

		w.logger.Warn("config reload failed",
			"error", err,
			"consecutive_failures", failures,
		)

		if maxFail > 0 && failures >= maxFail {
			w.logger.Error("max failures reached, stopping watcher",
				"max_failures", maxFail,
			)
			go w.Stop()
		}
		return
	}

	// Reset failure count on success
	w.mu.Lock()
	w.failureCount = 0
	w.mu.Unlock()

	meta := w.loader.Metadata()
	w.logger.Debug("config reloaded",
		"source", meta.Source,
		"key_count", meta.KeyCount,
	)
}

// Watch is a convenience function that creates and starts a watcher.
// It returns a stop function that should be called when done.
func Watch[T any](ctx context.Context, loader Loader[T], opts ...WatcherOption[T]) (stop func()) {
	w := NewWatcher(loader, opts...)
	if err := w.Start(ctx); err != nil {
		return func() {}
	}
	return w.Stop
}

// WatchWithCallback is a convenience function that watches and calls a callback on each change.
func WatchWithCallback[T any](ctx context.Context, loader Loader[T], callback func(old, new *T), opts ...WatcherOption[T]) (stop func()) {
	loader.OnChange(callback)
	return Watch(ctx, loader, opts...)
}
