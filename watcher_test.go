package dopplerconfig

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type WatchTestConfig struct {
	Value string `doppler:"VALUE" default:"initial"`
}

func TestNewWatcher_Defaults(t *testing.T) {
	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	w := NewWatcher[WatchTestConfig](loader)

	if w.interval != 30*time.Second {
		t.Errorf("default interval = %v, want 30s", w.interval)
	}
	if w.maxFailures != 0 {
		t.Errorf("default maxFailures = %d, want 0 (unlimited)", w.maxFailures)
	}
	if w.IsRunning() {
		t.Error("watcher should not be running before Start")
	}
}

func TestNewWatcher_WithOptions(t *testing.T) {
	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	w := NewWatcher[WatchTestConfig](loader,
		WithWatchInterval[WatchTestConfig](5*time.Second),
		WithMaxFailures[WatchTestConfig](3),
	)

	if w.interval != 5*time.Second {
		t.Errorf("interval = %v, want 5s", w.interval)
	}
	if w.maxFailures != 3 {
		t.Errorf("maxFailures = %d, want 3", w.maxFailures)
	}
}

func TestWatcher_StartStop(t *testing.T) {
	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	loader.Load(context.Background())

	w := NewWatcher[WatchTestConfig](loader,
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
	)

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !w.IsRunning() {
		t.Error("watcher should be running after Start")
	}

	// Double start should be a no-op
	if err := w.Start(ctx); err != nil {
		t.Fatalf("double Start failed: %v", err)
	}

	w.Stop()

	if w.IsRunning() {
		t.Error("watcher should not be running after Stop")
	}

	// Double stop should be safe
	w.Stop()
}

func TestWatcher_ContextCancellation(t *testing.T) {
	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	loader.Load(context.Background())

	w := NewWatcher[WatchTestConfig](loader,
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cancel()

	// Wait for watcher to stop (should happen quickly after context cancel)
	deadline := time.After(2 * time.Second)
	for {
		if !w.IsRunning() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("watcher did not stop after context cancellation")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestWatcher_PollReloadsConfig(t *testing.T) {
	values := map[string]string{"VALUE": "original"}
	loader, mock := TestLoader[WatchTestConfig](values)
	loader.Load(context.Background())

	w := NewWatcher[WatchTestConfig](loader,
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
	)

	ctx := context.Background()
	w.Start(ctx)

	// Update mock values
	mock.SetValue("VALUE", "updated")

	// Wait for at least a few poll cycles
	time.Sleep(100 * time.Millisecond)

	w.Stop()

	cfg := loader.Current()
	if cfg.Value != "updated" {
		t.Errorf("config value = %q, want %q", cfg.Value, "updated")
	}
}

func TestWatcher_MaxFailures(t *testing.T) {
	loader, mock := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	loader.Load(context.Background())

	w := NewWatcher[WatchTestConfig](loader,
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
		WithMaxFailures[WatchTestConfig](2),
	)

	// Make provider fail
	mock.SetError(fmt.Errorf("provider failure"))

	ctx := context.Background()
	w.Start(ctx)

	// Wait for failures to accumulate and watcher to self-stop
	deadline := time.After(2 * time.Second)
	for {
		if !w.IsRunning() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("watcher did not stop after max failures")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestWatch_Convenience(t *testing.T) {
	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
	loader.Load(context.Background())

	ctx := context.Background()
	stop := Watch[WatchTestConfig](ctx, loader,
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
	)

	time.Sleep(30 * time.Millisecond)
	stop()
}

func TestWatchWithCallback_Convenience(t *testing.T) {
	values := map[string]string{"VALUE": "first"}
	loader, mock := TestLoader[WatchTestConfig](values)
	loader.Load(context.Background())

	var callbackFired atomic.Bool
	stop := WatchWithCallback[WatchTestConfig](context.Background(), loader,
		func(old, new *WatchTestConfig) {
			callbackFired.Store(true)
		},
		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
	)

	mock.SetValue("VALUE", "second")
	time.Sleep(100 * time.Millisecond)
	stop()

	if !callbackFired.Load() {
		t.Error("expected callback to fire on config change")
	}
}
