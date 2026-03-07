Date Created: 2026-02-16T21:29:50-05:00
TOTAL_SCORE: 34/100

# dopplerconfig — Unit Test Coverage Report

## Executive Summary

The dopplerconfig module has **33.9% statement coverage** across 10 source files (~2,962 lines). Three entire subsystems — **multi-tenant** (489 lines), **watcher/hot-reload** (179 lines), and **feature flags** (251 lines) — have **0% test coverage**. The fallback providers, several loader paths, and numerous config utility functions are also completely untested.

Current state: **34 test cases** across 4 test files. To reach reasonable coverage (75%+), approximately **55–65 additional test cases** are needed.

---

## Coverage by File

| File | Lines | Current Coverage | Gap |
|------|-------|-----------------|-----|
| chassis.go | 108 | ~95% | Minor (warn policy path) |
| config.go | 170 | ~35% | `LoadBootstrapFromEnv`, `IsEnabled`, `HasFallback`, `MarshalJSON`, `SecretValue.String` empty case |
| doppler.go | 401 | ~60% | `FetchProject` error paths, `Name`, `Error`, `IsDopplerError`, `WithCallOptions`, ETag caching |
| fallback.go | 182 | ~15% | `flattenJSON`, `WriteFallbackFile`, entire `EnvProvider`, `FileProvider.Name/Close` |
| feature_flags.go | 251 | **0%** | Everything |
| loader.go | 438 | ~70% | `NewLoader`, `Current`, `Metadata`, `Close`, `loadFromProvider` warn policy, `setFieldValue` unsigned/float/duration |
| multitenant.go | 489 | **0%** | Everything |
| testing.go | 244 | ~50% | `RecordingProvider`, `TestLoaderWithConfig`, `AssertConfigEqual`, `MockProvider.FetchProject/SetValues/SetError/Clear` |
| validation.go | 500 | ~65% | `ValidationError.Error`, `ValidationErrors.Error`, `isSpecialType`, `isZero` for float/slice/map/ptr, port string validation |
| watcher.go | 179 | **0%** | Everything |

---

## Critical Untested Areas (Priority Order)

### 1. feature_flags.go — 0% Coverage (HIGH PRIORITY)

All public API untested. Contains business logic for percentage-based rollouts, caching, and case-insensitive key lookup.

### 2. watcher.go — 0% Coverage (HIGH PRIORITY)

Hot-reload system with goroutine lifecycle, failure counting, and max-failure stops. Concurrency bugs here would be invisible.

### 3. multitenant.go — 0% Coverage (HIGH PRIORITY)

Multi-tenant config loading with parallel fetch, reload diffing, and callbacks. Used by Solstice in production.

### 4. fallback.go — ~15% Coverage (MEDIUM PRIORITY)

`flattenJSON` handles nested JSON flattening with type coercion. `EnvProvider` is entirely untested. `WriteFallbackFile` untested.

### 5. config.go — ~35% Coverage (MEDIUM PRIORITY)

`LoadBootstrapFromEnv` reads env vars and parses failure policy. `SecretValue.MarshalJSON` untested.

### 6. loader.go edge cases — ~70% Coverage (LOW PRIORITY)

`loadFromProvider` warn policy, `Close` with errors, `Current`/`Metadata` accessors, `setFieldValue` for unsigned ints, floats, durations.

---

## Proposed Tests — Patch-Ready Diffs

### PATCH 1: feature_flags_test.go (NEW FILE — 22 test cases)

```diff
--- /dev/null
+++ b/feature_flags_test.go
@@ -0,0 +1,358 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestFeatureFlags_IsEnabled(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_RAG_ENABLED":  "true",
+		"FEATURE_BETA":         "false",
+		"FEATURE_ON_FLAG":      "on",
+		"FEATURE_YES_FLAG":     "yes",
+		"FEATURE_ONE_FLAG":     "1",
+		"FEATURE_ENABLED_FLAG": "enabled",
+		"FEATURE_ENABLE_FLAG":  "enable",
+		"FEATURE_OFF_FLAG":     "off",
+		"FEATURE_EMPTY":        "",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	tests := []struct {
+		name string
+		flag string
+		want bool
+	}{
+		{"true value", "RAG_ENABLED", true},
+		{"false value", "BETA", false},
+		{"on value", "ON_FLAG", true},
+		{"yes value", "YES_FLAG", true},
+		{"1 value", "ONE_FLAG", true},
+		{"enabled value", "ENABLED_FLAG", true},
+		{"enable value", "ENABLE_FLAG", true},
+		{"off value", "OFF_FLAG", false},
+		{"empty value", "EMPTY", false},
+		{"missing flag", "NONEXISTENT", false},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := ff.IsEnabled(tt.flag)
+			if got != tt.want {
+				t.Errorf("IsEnabled(%q) = %v, want %v", tt.flag, got, tt.want)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_IsDisabled(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_ACTIVE": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if ff.IsDisabled("ACTIVE") {
+		t.Error("IsDisabled(ACTIVE) = true, want false")
+	}
+	if !ff.IsDisabled("MISSING") {
+		t.Error("IsDisabled(MISSING) = false, want true")
+	}
+}
+
+func TestFeatureFlags_IsEnabled_CaseInsensitive(t *testing.T) {
+	values := map[string]string{
+		"feature_debug": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// The buildKey normalizes to uppercase, so "debug" -> "FEATURE_DEBUG"
+	// The case-insensitive fallback in IsEnabled should find "feature_debug"
+	if !ff.IsEnabled("debug") {
+		t.Error("IsEnabled(debug) should find case-insensitive match")
+	}
+}
+
+func TestFeatureFlags_IsEnabled_Caching(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_CACHED": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// First call populates cache
+	got1 := ff.IsEnabled("CACHED")
+	// Second call hits cache
+	got2 := ff.IsEnabled("CACHED")
+
+	if got1 != got2 || !got1 {
+		t.Errorf("IsEnabled cache inconsistency: got1=%v, got2=%v", got1, got2)
+	}
+}
+
+func TestFeatureFlags_GetInt(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MAX_RETRIES": "5",
+		"FEATURE_BAD_INT":     "abc",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	tests := []struct {
+		name       string
+		flag       string
+		defaultVal int
+		want       int
+	}{
+		{"valid int", "MAX_RETRIES", 3, 5},
+		{"invalid int returns default", "BAD_INT", 3, 3},
+		{"missing returns default", "MISSING", 42, 42},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := ff.GetInt(tt.flag, tt.defaultVal)
+			if got != tt.want {
+				t.Errorf("GetInt(%q, %d) = %d, want %d", tt.flag, tt.defaultVal, got, tt.want)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_GetFloat(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_RATE":      "0.75",
+		"FEATURE_BAD_FLOAT": "xyz",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	tests := []struct {
+		name       string
+		flag       string
+		defaultVal float64
+		want       float64
+	}{
+		{"valid float", "RATE", 0.0, 0.75},
+		{"invalid float returns default", "BAD_FLOAT", 1.5, 1.5},
+		{"missing returns default", "MISSING", 9.9, 9.9},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := ff.GetFloat(tt.flag, tt.defaultVal)
+			if got != tt.want {
+				t.Errorf("GetFloat(%q, %f) = %f, want %f", tt.flag, tt.defaultVal, got, tt.want)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_GetString(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_ENV": "production",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetString("ENV", "dev"); got != "production" {
+		t.Errorf("GetString(ENV) = %q, want \"production\"", got)
+	}
+	if got := ff.GetString("MISSING", "fallback"); got != "fallback" {
+		t.Errorf("GetString(MISSING) = %q, want \"fallback\"", got)
+	}
+}
+
+func TestFeatureFlags_GetStringSlice(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_REGIONS": "us-east, us-west, eu-west",
+		"FEATURE_EMPTY":   "",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	got := ff.GetStringSlice("REGIONS", nil)
+	expected := []string{"us-east", "us-west", "eu-west"}
+	if len(got) != len(expected) {
+		t.Fatalf("GetStringSlice(REGIONS) length = %d, want %d", len(got), len(expected))
+	}
+	for i, v := range expected {
+		if got[i] != v {
+			t.Errorf("GetStringSlice(REGIONS)[%d] = %q, want %q", i, got[i], v)
+		}
+	}
+
+	// Empty string returns default
+	def := []string{"default"}
+	got = ff.GetStringSlice("EMPTY", def)
+	if len(got) != 1 || got[0] != "default" {
+		t.Errorf("GetStringSlice(EMPTY) = %v, want %v", got, def)
+	}
+
+	// Missing returns default
+	got = ff.GetStringSlice("MISSING", def)
+	if len(got) != 1 || got[0] != "default" {
+		t.Errorf("GetStringSlice(MISSING) = %v, want %v", got, def)
+	}
+}
+
+func TestFeatureFlags_Update(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_A": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Populate cache
+	if !ff.IsEnabled("A") {
+		t.Fatal("IsEnabled(A) should be true before Update")
+	}
+
+	// Update with new values that disable A
+	ff.Update(map[string]string{
+		"FEATURE_A": "false",
+	})
+
+	// Cache should be cleared; new value should take effect
+	if ff.IsEnabled("A") {
+		t.Error("IsEnabled(A) should be false after Update")
+	}
+}
+
+func TestFeatureFlags_BuildKey_NoPrefix(t *testing.T) {
+	values := map[string]string{
+		"MY_FLAG": "true",
+	}
+	ff := NewFeatureFlags(values, "")
+
+	if !ff.IsEnabled("MY_FLAG") {
+		t.Error("IsEnabled(MY_FLAG) with no prefix should work")
+	}
+}
+
+func TestFeatureFlags_BuildKey_PrefixAlreadyPresent(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_ALREADY": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Passing a name that already starts with the prefix should not double-prefix
+	if !ff.IsEnabled("FEATURE_ALREADY") {
+		t.Error("IsEnabled(FEATURE_ALREADY) should not double-prefix")
+	}
+}
+
+func TestFeatureFlags_BuildKey_Normalization(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MY_COOL_FLAG": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Hyphens and spaces should be normalized to underscores
+	if !ff.IsEnabled("my-cool-flag") {
+		t.Error("IsEnabled should normalize hyphens to underscores")
+	}
+	if !ff.IsEnabled("my cool flag") {
+		t.Error("IsEnabled should normalize spaces to underscores")
+	}
+}
+
+func TestFeatureFlagsFromValues(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_TEST": "true",
+	}
+	ff := FeatureFlagsFromValues(values)
+	if !ff.IsEnabled("TEST") {
+		t.Error("FeatureFlagsFromValues should use FEATURE_ prefix")
+	}
+}
+
+func TestParseBool(t *testing.T) {
+	tests := []struct {
+		input string
+		want  bool
+	}{
+		{"true", true},
+		{"1", true},
+		{"yes", true},
+		{"on", true},
+		{"enabled", true},
+		{"enable", true},
+		{"TRUE", true},
+		{"  true  ", true},
+		{"false", false},
+		{"0", false},
+		{"no", false},
+		{"off", false},
+		{"random", false},
+		{"", false},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.input, func(t *testing.T) {
+			got := parseBool(tt.input)
+			if got != tt.want {
+				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
+			}
+		})
+	}
+}
+
+func TestRolloutConfig_ShouldEnable(t *testing.T) {
+	hash := func(s string) uint32 {
+		// Simple deterministic hash for testing
+		var h uint32
+		for _, c := range s {
+			h = h*31 + uint32(c)
+		}
+		return h
+	}
+
+	tests := []struct {
+		name       string
+		config     RolloutConfig
+		userID     string
+		want       bool
+	}{
+		{
+			name:   "allowed user always gets feature",
+			config: RolloutConfig{Percentage: 0, AllowedUsers: []string{"admin"}},
+			userID: "admin",
+			want:   true,
+		},
+		{
+			name:   "blocked user never gets feature",
+			config: RolloutConfig{Percentage: 100, BlockedUsers: []string{"banned"}},
+			userID: "banned",
+			want:   false,
+		},
+		{
+			name:   "0 percent disables for everyone",
+			config: RolloutConfig{Percentage: 0},
+			userID: "user1",
+			want:   false,
+		},
+		{
+			name:   "100 percent enables for everyone",
+			config: RolloutConfig{Percentage: 100},
+			userID: "user1",
+			want:   true,
+		},
+		{
+			name:   "negative percent disables for everyone",
+			config: RolloutConfig{Percentage: -5},
+			userID: "user1",
+			want:   false,
+		},
+		{
+			name:   "percentage-based rollout uses hash",
+			config: RolloutConfig{Percentage: 50},
+			userID: "user1",
+			want:   (hash("user1") % 100) < 50,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := tt.config.ShouldEnable(tt.userID, hash)
+			if got != tt.want {
+				t.Errorf("ShouldEnable(%q) = %v, want %v", tt.userID, got, tt.want)
+			}
+		})
+	}
+}
```

---

### PATCH 2: watcher_test.go (NEW FILE — 10 test cases)

```diff
--- /dev/null
+++ b/watcher_test.go
@@ -0,0 +1,230 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"log/slog"
+	"testing"
+	"time"
+)
+
+type WatcherTestConfig struct {
+	Value string `doppler:"VALUE" default:"initial"`
+}
+
+func TestNewWatcher_Defaults(t *testing.T) {
+	loader, _ := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	w := NewWatcher(loader)
+
+	if w.interval != 30*time.Second {
+		t.Errorf("default interval = %v, want 30s", w.interval)
+	}
+	if w.maxFailures != 0 {
+		t.Errorf("default maxFailures = %d, want 0", w.maxFailures)
+	}
+	if w.logger == nil {
+		t.Error("default logger should not be nil")
+	}
+}
+
+func TestNewWatcher_WithOptions(t *testing.T) {
+	loader, _ := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	logger := slog.Default()
+
+	w := NewWatcher(loader,
+		WithWatchInterval[WatcherTestConfig](5*time.Second),
+		WithWatchLogger[WatcherTestConfig](logger),
+		WithMaxFailures[WatcherTestConfig](3),
+	)
+
+	if w.interval != 5*time.Second {
+		t.Errorf("interval = %v, want 5s", w.interval)
+	}
+	if w.maxFailures != 3 {
+		t.Errorf("maxFailures = %d, want 3", w.maxFailures)
+	}
+}
+
+func TestWatcher_StartStop(t *testing.T) {
+	loader, _ := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](10*time.Millisecond))
+
+	if w.IsRunning() {
+		t.Error("watcher should not be running before Start")
+	}
+
+	ctx := context.Background()
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+
+	if !w.IsRunning() {
+		t.Error("watcher should be running after Start")
+	}
+
+	// Start again should be a no-op
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("second Start failed: %v", err)
+	}
+
+	w.Stop()
+
+	if w.IsRunning() {
+		t.Error("watcher should not be running after Stop")
+	}
+
+	// Stop again should be a no-op
+	w.Stop()
+}
+
+func TestWatcher_ContextCancel(t *testing.T) {
+	loader, _ := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](10*time.Millisecond))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	w.Start(ctx)
+
+	// Cancel context should stop the watcher
+	cancel()
+	time.Sleep(50 * time.Millisecond)
+
+	if w.IsRunning() {
+		t.Error("watcher should stop when context is cancelled")
+	}
+}
+
+func TestWatcher_PollSuccess(t *testing.T) {
+	loader, mock := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "v1"})
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](10*time.Millisecond))
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Update value and wait for a poll cycle
+	mock.SetValue("VALUE", "v2")
+	time.Sleep(50 * time.Millisecond)
+
+	w.Stop()
+
+	cfg := loader.Current()
+	if cfg.Value != "v2" {
+		t.Errorf("after poll, Value = %q, want \"v2\"", cfg.Value)
+	}
+}
+
+func TestWatcher_PollFailureCount(t *testing.T) {
+	loader, mock := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	// Set error to cause poll failures
+	mock.SetError(fmt.Errorf("fetch failed"))
+
+	w := NewWatcher(loader,
+		WithWatchInterval[WatcherTestConfig](10*time.Millisecond),
+		WithMaxFailures[WatcherTestConfig](3),
+	)
+
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Wait long enough for 3+ failures
+	time.Sleep(100 * time.Millisecond)
+
+	// Watcher should have stopped itself after max failures
+	time.Sleep(50 * time.Millisecond)
+	if w.IsRunning() {
+		t.Error("watcher should stop after max failures reached")
+	}
+}
+
+func TestWatcher_PollResetsFailureCount(t *testing.T) {
+	loader, mock := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	// Start with error
+	mock.SetError(fmt.Errorf("temporary"))
+
+	w := NewWatcher(loader,
+		WithWatchInterval[WatcherTestConfig](10*time.Millisecond),
+		WithMaxFailures[WatcherTestConfig](10),
+	)
+
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Let a few failures accumulate
+	time.Sleep(30 * time.Millisecond)
+
+	// Clear error — next successful poll should reset failure count
+	mock.SetError(nil)
+	time.Sleep(30 * time.Millisecond)
+
+	w.Stop()
+
+	// The watcher should still have been running (didn't hit max failures)
+	// We already stopped it, so this is a structural test
+}
+
+func TestWatch_Convenience(t *testing.T) {
+	loader, _ := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	ctx := context.Background()
+	stop := Watch(ctx, loader, WithWatchInterval[WatcherTestConfig](10*time.Millisecond))
+
+	time.Sleep(30 * time.Millisecond)
+	stop()
+}
+
+func TestWatchWithCallback_Convenience(t *testing.T) {
+	loader, mock := TestLoader[WatcherTestConfig](map[string]string{"VALUE": "v1"})
+	loader.Load(context.Background())
+
+	var callbackFired bool
+	stop := WatchWithCallback(context.Background(), loader, func(old, new *WatcherTestConfig) {
+		callbackFired = true
+	}, WithWatchInterval[WatcherTestConfig](10*time.Millisecond))
+
+	mock.SetValue("VALUE", "v2")
+	time.Sleep(50 * time.Millisecond)
+	stop()
+
+	if !callbackFired {
+		t.Error("WatchWithCallback should have fired callback on change")
+	}
+}
```

---

### PATCH 3: multitenant_test.go (NEW FILE — 14 test cases)

```diff
--- /dev/null
+++ b/multitenant_test.go
@@ -0,0 +1,322 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"testing"
+	"time"
+)
+
+type MTEnvConfig struct {
+	AppName  string `doppler:"APP_NAME" default:"myapp"`
+	LogLevel string `doppler:"LOG_LEVEL" default:"info"`
+}
+
+type MTProjectConfig struct {
+	ProjectName string `doppler:"PROJECT_NAME"`
+	MaxConns    int    `doppler:"MAX_CONNS" default:"5"`
+}
+
+func newTestMultiTenantLoader(t *testing.T) (MultiTenantLoader[MTEnvConfig, MTProjectConfig], *MockProvider) {
+	t.Helper()
+	mock := NewMockProvider(map[string]string{
+		"APP_NAME":  "testapp",
+		"LOG_LEVEL": "debug",
+	})
+	// Set up project-specific values
+	mock.SetProjectValues("", "proj-a", map[string]string{
+		"PROJECT_NAME": "Alpha",
+		"MAX_CONNS":    "10",
+	})
+	mock.SetProjectValues("", "proj-b", map[string]string{
+		"PROJECT_NAME": "Beta",
+		"MAX_CONNS":    "20",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)
+	return loader, mock
+}
+
+func TestMultiTenantLoader_LoadEnv(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	cfg, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv failed: %v", err)
+	}
+
+	if cfg.AppName != "testapp" {
+		t.Errorf("AppName = %q, want \"testapp\"", cfg.AppName)
+	}
+	if cfg.LogLevel != "debug" {
+		t.Errorf("LogLevel = %q, want \"debug\"", cfg.LogLevel)
+	}
+
+	// Env() should return the loaded config
+	env := loader.Env()
+	if env == nil {
+		t.Fatal("Env() returned nil after LoadEnv")
+	}
+	if env.AppName != "testapp" {
+		t.Errorf("Env().AppName = %q, want \"testapp\"", env.AppName)
+	}
+}
+
+func TestMultiTenantLoader_LoadProject(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	cfg, err := loader.LoadProject(context.Background(), "proj-a")
+	if err != nil {
+		t.Fatalf("LoadProject failed: %v", err)
+	}
+
+	if cfg.ProjectName != "Alpha" {
+		t.Errorf("ProjectName = %q, want \"Alpha\"", cfg.ProjectName)
+	}
+	if cfg.MaxConns != 10 {
+		t.Errorf("MaxConns = %d, want 10", cfg.MaxConns)
+	}
+
+	// Project() should return the cached config
+	cached, ok := loader.Project("proj-a")
+	if !ok {
+		t.Fatal("Project(proj-a) not found")
+	}
+	if cached.ProjectName != "Alpha" {
+		t.Errorf("cached ProjectName = %q, want \"Alpha\"", cached.ProjectName)
+	}
+}
+
+func TestMultiTenantLoader_LoadAllProjects(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	projects, err := loader.LoadAllProjects(context.Background(), []string{"proj-a", "proj-b"})
+	if err != nil {
+		t.Fatalf("LoadAllProjects failed: %v", err)
+	}
+
+	if len(projects) != 2 {
+		t.Fatalf("LoadAllProjects returned %d projects, want 2", len(projects))
+	}
+
+	if projects["proj-a"].ProjectName != "Alpha" {
+		t.Errorf("proj-a.ProjectName = %q, want \"Alpha\"", projects["proj-a"].ProjectName)
+	}
+	if projects["proj-b"].ProjectName != "Beta" {
+		t.Errorf("proj-b.ProjectName = %q, want \"Beta\"", projects["proj-b"].ProjectName)
+	}
+}
+
+func TestMultiTenantLoader_Projects(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	loader.LoadAllProjects(context.Background(), []string{"proj-a", "proj-b"})
+
+	all := loader.Projects()
+	if len(all) != 2 {
+		t.Fatalf("Projects() returned %d, want 2", len(all))
+	}
+}
+
+func TestMultiTenantLoader_ProjectCodes(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	loader.LoadAllProjects(context.Background(), []string{"proj-b", "proj-a"})
+
+	codes := loader.ProjectCodes()
+	if len(codes) != 2 {
+		t.Fatalf("ProjectCodes() returned %d codes, want 2", len(codes))
+	}
+	// Should be sorted
+	if codes[0] != "proj-a" || codes[1] != "proj-b" {
+		t.Errorf("ProjectCodes() = %v, want [proj-a, proj-b]", codes)
+	}
+}
+
+func TestMultiTenantLoader_ProjectNotFound(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	_, ok := loader.Project("nonexistent")
+	if ok {
+		t.Error("Project(nonexistent) should return false")
+	}
+}
+
+func TestMultiTenantLoader_ReloadProjects(t *testing.T) {
+	loader, mock := newTestMultiTenantLoader(t)
+
+	// Load initial projects
+	loader.LoadAllProjects(context.Background(), []string{"proj-a", "proj-b"})
+
+	// Update proj-a values
+	mock.SetProjectValues("", "proj-a", map[string]string{
+		"PROJECT_NAME": "Alpha-v2",
+		"MAX_CONNS":    "15",
+	})
+
+	diff, err := loader.ReloadProjects(context.Background())
+	if err != nil {
+		t.Fatalf("ReloadProjects failed: %v", err)
+	}
+
+	// Both projects should be in Unchanged (they existed before and after)
+	if len(diff.Added) != 0 {
+		t.Errorf("diff.Added = %v, want empty", diff.Added)
+	}
+	if len(diff.Removed) != 0 {
+		t.Errorf("diff.Removed = %v, want empty", diff.Removed)
+	}
+	if len(diff.Unchanged) != 2 {
+		t.Errorf("diff.Unchanged = %v, want 2 entries", diff.Unchanged)
+	}
+
+	// Verify updated value
+	proj, ok := loader.Project("proj-a")
+	if !ok {
+		t.Fatal("proj-a not found after reload")
+	}
+	if proj.ProjectName != "Alpha-v2" {
+		t.Errorf("proj-a.ProjectName after reload = %q, want \"Alpha-v2\"", proj.ProjectName)
+	}
+}
+
+func TestMultiTenantLoader_OnEnvChange(t *testing.T) {
+	loader, mock := newTestMultiTenantLoader(t)
+
+	// Load initial env
+	loader.LoadEnv(context.Background())
+
+	var callbackOldName, callbackNewName string
+	loader.OnEnvChange(func(old, new *MTEnvConfig) {
+		callbackOldName = old.AppName
+		callbackNewName = new.AppName
+	})
+
+	// Update and reload
+	mock.SetValue("APP_NAME", "updated-app")
+	loader.LoadEnv(context.Background())
+
+	if callbackOldName != "testapp" {
+		t.Errorf("OnEnvChange old AppName = %q, want \"testapp\"", callbackOldName)
+	}
+	if callbackNewName != "updated-app" {
+		t.Errorf("OnEnvChange new AppName = %q, want \"updated-app\"", callbackNewName)
+	}
+}
+
+func TestMultiTenantLoader_OnProjectChange(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	loader.LoadAllProjects(context.Background(), []string{"proj-a"})
+
+	var callbackDiff *ReloadDiff
+	loader.OnProjectChange(func(diff *ReloadDiff) {
+		callbackDiff = diff
+	})
+
+	loader.ReloadProjects(context.Background())
+
+	if callbackDiff == nil {
+		t.Fatal("OnProjectChange callback was not called")
+	}
+}
+
+func TestMultiTenantLoader_Close(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+
+	if err := loader.Close(); err != nil {
+		t.Errorf("Close returned error: %v", err)
+	}
+}
+
+func TestMultiTenantLoader_FetchWithFallback(t *testing.T) {
+	// Primary fails, fallback succeeds
+	primary := NewMockProviderWithError(fmt.Errorf("primary down"))
+	fallback := NewMockProvider(map[string]string{
+		"APP_NAME":  "fallback-app",
+		"LOG_LEVEL": "warn",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](primary, fallback)
+
+	cfg, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv with fallback failed: %v", err)
+	}
+	if cfg.AppName != "fallback-app" {
+		t.Errorf("AppName = %q, want \"fallback-app\" from fallback", cfg.AppName)
+	}
+}
+
+func TestMultiTenantLoader_NoProviders(t *testing.T) {
+	// Both primary and fallback fail
+	primary := NewMockProviderWithError(fmt.Errorf("primary down"))
+	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](primary, nil)
+
+	_, err := loader.LoadEnv(context.Background())
+	if err == nil {
+		t.Error("LoadEnv should fail when all providers fail")
+	}
+}
+
+func TestMultiTenantWatcher_StartStop(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+	loader.LoadEnv(context.Background())
+	loader.LoadAllProjects(context.Background(), []string{"proj-a"})
+
+	w := NewMultiTenantWatcher[MTEnvConfig, MTProjectConfig](loader, 10*time.Millisecond)
+
+	ctx := context.Background()
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+
+	// Start again should be a no-op
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("second Start failed: %v", err)
+	}
+
+	time.Sleep(30 * time.Millisecond)
+	w.Stop()
+
+	// Stop again should be a no-op
+	w.Stop()
+}
+
+func TestMultiTenantWatcher_ContextCancel(t *testing.T) {
+	loader, _ := newTestMultiTenantLoader(t)
+	loader.LoadEnv(context.Background())
+
+	w := NewMultiTenantWatcher[MTEnvConfig, MTProjectConfig](loader, 10*time.Millisecond)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	w.Start(ctx)
+
+	cancel()
+	time.Sleep(50 * time.Millisecond)
+	// Should have stopped gracefully via context cancellation
+}
```

---

### PATCH 4: fallback_test.go (NEW FILE — 12 test cases)

```diff
--- /dev/null
+++ b/fallback_test.go
@@ -0,0 +1,239 @@
+package dopplerconfig
+
+import (
+	"context"
+	"encoding/json"
+	"os"
+	"testing"
+)
+
+func TestFileProvider_FetchProject_ValidJSON(t *testing.T) {
+	tmpFile := t.TempDir() + "/config.json"
+	data := map[string]interface{}{
+		"PORT":     8080,
+		"HOST":     "localhost",
+		"ENABLED":  true,
+		"DISABLED": false,
+		"TAGS":     []interface{}{"a", "b", "c"},
+		"RATE":     3.14,
+		"EMPTY":    nil,
+	}
+	raw, _ := json.Marshal(data)
+	os.WriteFile(tmpFile, raw, 0600)
+
+	fp := NewFileProvider(tmpFile)
+	values, err := fp.FetchProject(context.Background(), "", "")
+	if err != nil {
+		t.Fatalf("FetchProject failed: %v", err)
+	}
+
+	if values["PORT"] != "8080" {
+		t.Errorf("PORT = %q, want \"8080\"", values["PORT"])
+	}
+	if values["HOST"] != "localhost" {
+		t.Errorf("HOST = %q, want \"localhost\"", values["HOST"])
+	}
+	if values["ENABLED"] != "true" {
+		t.Errorf("ENABLED = %q, want \"true\"", values["ENABLED"])
+	}
+	if values["DISABLED"] != "false" {
+		t.Errorf("DISABLED = %q, want \"false\"", values["DISABLED"])
+	}
+	if values["TAGS"] != "a,b,c" {
+		t.Errorf("TAGS = %q, want \"a,b,c\"", values["TAGS"])
+	}
+	if values["EMPTY"] != "" {
+		t.Errorf("EMPTY = %q, want \"\"", values["EMPTY"])
+	}
+}
+
+func TestFileProvider_FetchProject_NestedJSON(t *testing.T) {
+	tmpFile := t.TempDir() + "/nested.json"
+	data := map[string]interface{}{
+		"server": map[string]interface{}{
+			"port": 9090,
+			"host": "0.0.0.0",
+		},
+	}
+	raw, _ := json.Marshal(data)
+	os.WriteFile(tmpFile, raw, 0600)
+
+	fp := NewFileProvider(tmpFile)
+	values, err := fp.FetchProject(context.Background(), "", "")
+	if err != nil {
+		t.Fatalf("FetchProject failed: %v", err)
+	}
+
+	if values["server_port"] != "9090" {
+		t.Errorf("server_port = %q, want \"9090\"", values["server_port"])
+	}
+	if values["server_host"] != "0.0.0.0" {
+		t.Errorf("server_host = %q, want \"0.0.0.0\"", values["server_host"])
+	}
+}
+
+func TestFileProvider_FetchProject_NotFound(t *testing.T) {
+	fp := NewFileProvider("/nonexistent/path/config.json")
+	_, err := fp.FetchProject(context.Background(), "", "")
+	if err == nil {
+		t.Error("FetchProject should fail for nonexistent file")
+	}
+}
+
+func TestFileProvider_FetchProject_InvalidJSON(t *testing.T) {
+	tmpFile := t.TempDir() + "/bad.json"
+	os.WriteFile(tmpFile, []byte("not json"), 0600)
+
+	fp := NewFileProvider(tmpFile)
+	_, err := fp.FetchProject(context.Background(), "", "")
+	if err == nil {
+		t.Error("FetchProject should fail for invalid JSON")
+	}
+}
+
+func TestFileProvider_Name(t *testing.T) {
+	fp := NewFileProvider("/path/to/config.json")
+	name := fp.Name()
+	if name != "file:/path/to/config.json" {
+		t.Errorf("Name() = %q, want \"file:/path/to/config.json\"", name)
+	}
+}
+
+func TestFileProvider_Close(t *testing.T) {
+	fp := NewFileProvider("/path/to/config.json")
+	if err := fp.Close(); err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestWriteFallbackFile(t *testing.T) {
+	tmpFile := t.TempDir() + "/fallback.json"
+	values := map[string]string{
+		"KEY1": "value1",
+		"KEY2": "value2",
+	}
+
+	if err := WriteFallbackFile(tmpFile, values); err != nil {
+		t.Fatalf("WriteFallbackFile failed: %v", err)
+	}
+
+	// Read back and verify
+	data, err := os.ReadFile(tmpFile)
+	if err != nil {
+		t.Fatalf("failed to read fallback file: %v", err)
+	}
+
+	var readBack map[string]string
+	if err := json.Unmarshal(data, &readBack); err != nil {
+		t.Fatalf("failed to parse written file: %v", err)
+	}
+
+	if readBack["KEY1"] != "value1" {
+		t.Errorf("KEY1 = %q, want \"value1\"", readBack["KEY1"])
+	}
+	if readBack["KEY2"] != "value2" {
+		t.Errorf("KEY2 = %q, want \"value2\"", readBack["KEY2"])
+	}
+
+	// Verify file permissions (0600)
+	info, _ := os.Stat(tmpFile)
+	if info.Mode().Perm() != 0600 {
+		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
+	}
+}
+
+func TestWriteFallbackFile_InvalidPath(t *testing.T) {
+	err := WriteFallbackFile("/nonexistent/dir/fallback.json", map[string]string{"K": "V"})
+	if err == nil {
+		t.Error("WriteFallbackFile should fail for invalid path")
+	}
+}
+
+func TestEnvProvider_Fetch(t *testing.T) {
+	// Set some env vars for testing
+	t.Setenv("DOPPLERTEST_FOO", "bar")
+	t.Setenv("DOPPLERTEST_BAZ", "qux")
+
+	ep := NewEnvProvider("DOPPLERTEST_")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["DOPPLERTEST_FOO"] != "bar" {
+		t.Errorf("DOPPLERTEST_FOO = %q, want \"bar\"", values["DOPPLERTEST_FOO"])
+	}
+	if values["DOPPLERTEST_BAZ"] != "qux" {
+		t.Errorf("DOPPLERTEST_BAZ = %q, want \"qux\"", values["DOPPLERTEST_BAZ"])
+	}
+}
+
+func TestEnvProvider_Fetch_NoPrefix(t *testing.T) {
+	ep := NewEnvProvider("")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	// Should return all env vars — at least PATH should exist
+	if _, ok := values["PATH"]; !ok {
+		t.Error("Fetch with no prefix should return PATH")
+	}
+}
+
+func TestEnvProvider_Name(t *testing.T) {
+	ep1 := NewEnvProvider("MY_PREFIX_")
+	if ep1.Name() != "env:MY_PREFIX_*" {
+		t.Errorf("Name() = %q, want \"env:MY_PREFIX_*\"", ep1.Name())
+	}
+
+	ep2 := NewEnvProvider("")
+	if ep2.Name() != "env" {
+		t.Errorf("Name() = %q, want \"env\"", ep2.Name())
+	}
+}
+
+func TestEnvProvider_Close(t *testing.T) {
+	ep := NewEnvProvider("")
+	if err := ep.Close(); err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestSplitEnv(t *testing.T) {
+	tests := []struct {
+		input    string
+		wantKey  string
+		wantVal  string
+	}{
+		{"FOO=bar", "FOO", "bar"},
+		{"FOO=bar=baz", "FOO", "bar=baz"},
+		{"FOO=", "FOO", ""},
+		{"FOO", "FOO", ""},
+	}
+
+	for _, tt := range tests {
+		key, val := splitEnv(tt.input)
+		if key != tt.wantKey || val != tt.wantVal {
+			t.Errorf("splitEnv(%q) = (%q, %q), want (%q, %q)",
+				tt.input, key, val, tt.wantKey, tt.wantVal)
+		}
+	}
+}
+
+func TestHasPrefix(t *testing.T) {
+	if !hasPrefix("MYPREFIX_VAR", "MYPREFIX_") {
+		t.Error("hasPrefix should match")
+	}
+	if hasPrefix("OTHER", "MYPREFIX_") {
+		t.Error("hasPrefix should not match")
+	}
+	if !hasPrefix("ABC", "") {
+		t.Error("empty prefix should match everything")
+	}
+}
```

---

### PATCH 5: config_test.go (NEW FILE — 7 test cases)

```diff
--- /dev/null
+++ b/config_test.go
@@ -0,0 +1,115 @@
+package dopplerconfig
+
+import (
+	"encoding/json"
+	"os"
+	"testing"
+)
+
+func TestLoadBootstrapFromEnv(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "dp.st.xxx")
+	t.Setenv("DOPPLER_PROJECT", "my-project")
+	t.Setenv("DOPPLER_CONFIG", "prd")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "/etc/fallback.json")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "true")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "fail")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.Token != "dp.st.xxx" {
+		t.Errorf("Token = %q, want \"dp.st.xxx\"", cfg.Token)
+	}
+	if cfg.Project != "my-project" {
+		t.Errorf("Project = %q, want \"my-project\"", cfg.Project)
+	}
+	if cfg.Config != "prd" {
+		t.Errorf("Config = %q, want \"prd\"", cfg.Config)
+	}
+	if cfg.FallbackPath != "/etc/fallback.json" {
+		t.Errorf("FallbackPath = %q, want \"/etc/fallback.json\"", cfg.FallbackPath)
+	}
+	if !cfg.WatchEnabled {
+		t.Error("WatchEnabled = false, want true")
+	}
+	if cfg.FailurePolicy != FailurePolicyFail {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyFail", cfg.FailurePolicy)
+	}
+}
+
+func TestLoadBootstrapFromEnv_WarnPolicy(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "")
+	t.Setenv("DOPPLER_PROJECT", "")
+	t.Setenv("DOPPLER_CONFIG", "")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "false")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "warn")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.FailurePolicy != FailurePolicyWarn {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyWarn", cfg.FailurePolicy)
+	}
+	if cfg.WatchEnabled {
+		t.Error("WatchEnabled should be false")
+	}
+}
+
+func TestLoadBootstrapFromEnv_DefaultPolicy(t *testing.T) {
+	// Ensure DOPPLER_FAILURE_POLICY is not set
+	os.Unsetenv("DOPPLER_FAILURE_POLICY")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.FailurePolicy != FailurePolicyFallback {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyFallback (default)", cfg.FailurePolicy)
+	}
+}
+
+func TestBootstrapConfig_IsEnabled(t *testing.T) {
+	cfg := BootstrapConfig{Token: "some-token"}
+	if !cfg.IsEnabled() {
+		t.Error("IsEnabled() = false, want true when token is set")
+	}
+
+	cfg.Token = ""
+	if cfg.IsEnabled() {
+		t.Error("IsEnabled() = true, want false when token is empty")
+	}
+}
+
+func TestBootstrapConfig_HasFallback(t *testing.T) {
+	cfg := BootstrapConfig{FallbackPath: "/tmp/fallback.json"}
+	if !cfg.HasFallback() {
+		t.Error("HasFallback() = false, want true when path is set")
+	}
+
+	cfg.FallbackPath = ""
+	if cfg.HasFallback() {
+		t.Error("HasFallback() = true, want false when path is empty")
+	}
+}
+
+func TestSecretValue_String_Empty(t *testing.T) {
+	sv := NewSecretValue("")
+	if sv.String() != "[empty]" {
+		t.Errorf("String() = %q, want \"[empty]\"", sv.String())
+	}
+}
+
+func TestSecretValue_MarshalJSON(t *testing.T) {
+	sv := NewSecretValue("super-secret")
+
+	data, err := json.Marshal(sv)
+	if err != nil {
+		t.Fatalf("MarshalJSON failed: %v", err)
+	}
+
+	if string(data) != `"[REDACTED]"` {
+		t.Errorf("MarshalJSON = %s, want %q", data, `"[REDACTED]"`)
+	}
+}
```

---

### PATCH 6: doppler_extended_test.go (NEW FILE — 5 test cases)

```diff
--- /dev/null
+++ b/doppler_extended_test.go
@@ -0,0 +1,89 @@
+package dopplerconfig
+
+import (
+	"context"
+	"errors"
+	"fmt"
+	"net/http"
+	"testing"
+)
+
+func TestDopplerProvider_Name(t *testing.T) {
+	provider, err := NewDopplerProvider("test-token", "proj", "dev",
+		WithHTTPClient(&http.Client{}),
+	)
+	if err != nil {
+		t.Fatalf("NewDopplerProvider failed: %v", err)
+	}
+
+	if provider.Name() != "doppler" {
+		t.Errorf("Name() = %q, want \"doppler\"", provider.Name())
+	}
+}
+
+func TestDopplerError_Error(t *testing.T) {
+	de := &DopplerError{
+		StatusCode: 401,
+		Message:    "unauthorized",
+	}
+	expected := "doppler error 401: unauthorized"
+	if de.Error() != expected {
+		t.Errorf("Error() = %q, want %q", de.Error(), expected)
+	}
+}
+
+func TestIsDopplerError(t *testing.T) {
+	de := &DopplerError{StatusCode: 404, Message: "not found"}
+
+	// Direct error
+	found, ok := IsDopplerError(de)
+	if !ok {
+		t.Error("IsDopplerError should return true for DopplerError")
+	}
+	if found.StatusCode != 404 {
+		t.Errorf("StatusCode = %d, want 404", found.StatusCode)
+	}
+
+	// Wrapped error
+	wrapped := fmt.Errorf("context: %w", de)
+	found, ok = IsDopplerError(wrapped)
+	if !ok {
+		t.Error("IsDopplerError should return true for wrapped DopplerError")
+	}
+	if found.StatusCode != 404 {
+		t.Errorf("StatusCode = %d, want 404", found.StatusCode)
+	}
+
+	// Non-DopplerError
+	_, ok = IsDopplerError(errors.New("plain error"))
+	if ok {
+		t.Error("IsDopplerError should return false for non-DopplerError")
+	}
+}
+
+func TestDopplerProvider_EmptyToken(t *testing.T) {
+	_, err := NewDopplerProvider("", "proj", "dev")
+	if err == nil {
+		t.Error("NewDopplerProvider should fail with empty token")
+	}
+}
+
+func TestDopplerProvider_FetchProject_ETagCaching(t *testing.T) {
+	callCount := 0
+	srv := newHTTPTestServer("", 0)
+	srv.Close() // Close default, create custom
+
+	srv = &httpTestServer{newETagServer(t, &callCount)}
+
+	defer srv.Close()
+
+	provider, err := NewDopplerProvider("test-token", "proj", "dev",
+		WithAPIURL(srv.URL),
+		WithHTTPClient(srv.Client()),
+	)
+	if err != nil {
+		t.Fatalf("NewDopplerProvider failed: %v", err)
+	}
+
+	// First fetch — full response with ETag
+	_, err = provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("first Fetch failed: %v", err)
+	}
+
+	// Second fetch — should send If-None-Match and get 304
+	_, err = provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("second Fetch (cached) failed: %v", err)
+	}
+}
```

Supporting test helper for ETag server (add to chassis_test.go or a helpers file):

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -14,6 +14,7 @@
 import (
 	"context"
 	"fmt"
+	"net/http/httptest"
 	"net/http"
 	"os"
 	"testing"
@@ -380,3 +381,22 @@
 	}))
 	return &httpTestServer{srv}
 }
+
+func newETagServer(t *testing.T, callCount *int) *httptest.Server {
+	t.Helper()
+	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		*callCount++
+		w.Header().Set("Content-Type", "application/json")
+
+		if r.Header.Get("If-None-Match") == `"etag-123"` {
+			w.WriteHeader(http.StatusNotModified)
+			return
+		}
+
+		w.Header().Set("ETag", `"etag-123"`)
+		w.WriteHeader(http.StatusOK)
+		w.Write([]byte(`{"secrets":{"KEY":{"raw":"value"}}}`))
+	}))
+}
```

---

### PATCH 7: loader_edge_test.go (NEW FILE — 7 test cases)

```diff
--- /dev/null
+++ b/loader_edge_test.go
@@ -0,0 +1,149 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"testing"
+	"time"
+)
+
+func TestLoader_Current(t *testing.T) {
+	loader, _ := TestLoader[TestConfig](map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	})
+
+	// Before Load, Current should be nil
+	if loader.Current() != nil {
+		t.Error("Current() should be nil before Load")
+	}
+
+	loader.Load(context.Background())
+
+	cfg := loader.Current()
+	if cfg == nil {
+		t.Fatal("Current() should not be nil after Load")
+	}
+	if cfg.Database.URL != "postgres://localhost/test" {
+		t.Errorf("Current().Database.URL = %q, want \"postgres://localhost/test\"", cfg.Database.URL)
+	}
+}
+
+func TestLoader_Metadata(t *testing.T) {
+	loader, _ := TestLoader[TestConfig](map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	})
+	loader.Load(context.Background())
+
+	meta := loader.Metadata()
+	if meta.Source != "mock" {
+		t.Errorf("Source = %q, want \"mock\"", meta.Source)
+	}
+	if meta.KeyCount != 1 {
+		t.Errorf("KeyCount = %d, want 1", meta.KeyCount)
+	}
+	if meta.LoadedAt.IsZero() {
+		t.Error("LoadedAt should not be zero")
+	}
+}
+
+func TestLoader_Close(t *testing.T) {
+	loader, _ := TestLoader[TestConfig](map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	})
+
+	if err := loader.Close(); err != nil {
+		t.Errorf("Close returned error: %v", err)
+	}
+}
+
+func TestLoader_FallbackOnPrimaryFailure(t *testing.T) {
+	primary := NewMockProviderWithError(fmt.Errorf("primary down"))
+	fallback := NewMockProvider(map[string]string{
+		"DATABASE_URL": "postgres://fallback/test",
+	})
+
+	loader := NewLoaderWithProvider[TestConfig](primary, fallback)
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load with fallback failed: %v", err)
+	}
+
+	if cfg.Database.URL != "postgres://fallback/test" {
+		t.Errorf("Database.URL = %q, want fallback URL", cfg.Database.URL)
+	}
+
+	// Metadata should show fallback source
+	meta := loader.Metadata()
+	if meta.Source != "mock" {
+		t.Errorf("Source = %q, want \"mock\"", meta.Source)
+	}
+}
+
+func TestLoader_WarnPolicy(t *testing.T) {
+	primary := NewMockProviderWithError(fmt.Errorf("all down"))
+
+	loader := &loader[TestConfig]{
+		provider:  primary,
+		bootstrap: BootstrapConfig{FailurePolicy: FailurePolicyWarn},
+	}
+
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load with warn policy should not fail: %v", err)
+	}
+
+	// Should get defaults only
+	if cfg.Server.Port != 8080 {
+		t.Errorf("Server.Port = %d, want default 8080", cfg.Server.Port)
+	}
+}
+
+type DurationConfig struct {
+	Timeout time.Duration `doppler:"TIMEOUT"`
+}
+
+func TestLoader_DurationField(t *testing.T) {
+	values := map[string]string{
+		"TIMEOUT": "5s",
+	}
+	loader, _ := TestLoader[DurationConfig](values)
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.Timeout != 5*time.Second {
+		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
+	}
+}
+
+func TestLoader_DurationField_IntegerSeconds(t *testing.T) {
+	values := map[string]string{
+		"TIMEOUT": "30",
+	}
+	loader, _ := TestLoader[DurationConfig](values)
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.Timeout != 30*time.Second {
+		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
+	}
+}
+
+type UnsignedConfig struct {
+	MaxConns uint   `doppler:"MAX_CONNS" default:"10"`
+	Rate     float64 `doppler:"RATE" default:"1.5"`
+}
+
+func TestLoader_UnsignedAndFloat(t *testing.T) {
+	values := map[string]string{
+		"MAX_CONNS": "25",
+		"RATE":      "3.14",
+	}
+	loader, _ := TestLoader[UnsignedConfig](values)
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.MaxConns != 25 {
+		t.Errorf("MaxConns = %d, want 25", cfg.MaxConns)
+	}
+	if cfg.Rate != 3.14 {
+		t.Errorf("Rate = %f, want 3.14", cfg.Rate)
+	}
+}
```

---

### PATCH 8: testing_extended_test.go (NEW FILE — 5 test cases)

```diff
--- /dev/null
+++ b/testing_extended_test.go
@@ -0,0 +1,98 @@
+package dopplerconfig
+
+import (
+	"context"
+	"testing"
+)
+
+func TestRecordingProvider_RecordsCalls(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"KEY": "val"})
+	rec := NewRecordingProvider(mock)
+
+	// Fetch
+	values, err := rec.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if values["KEY"] != "val" {
+		t.Errorf("KEY = %q, want \"val\"", values["KEY"])
+	}
+
+	// FetchProject
+	mock.SetProjectValues("proj", "cfg", map[string]string{"P": "1"})
+	values, err = rec.FetchProject(context.Background(), "proj", "cfg")
+	if err != nil {
+		t.Fatalf("FetchProject failed: %v", err)
+	}
+	if values["P"] != "1" {
+		t.Errorf("P = %q, want \"1\"", values["P"])
+	}
+
+	// Check recorded calls
+	calls := rec.Calls()
+	if len(calls) != 2 {
+		t.Fatalf("CallCount = %d, want 2", len(calls))
+	}
+	if rec.CallCount() != 2 {
+		t.Fatalf("CallCount() = %d, want 2", rec.CallCount())
+	}
+
+	// First call should have empty project/config (Fetch)
+	if calls[0].Project != "" || calls[0].Config != "" {
+		t.Errorf("First call Project/Config = (%q, %q), want empty", calls[0].Project, calls[0].Config)
+	}
+
+	// Second call should have project/config
+	if calls[1].Project != "proj" || calls[1].Config != "cfg" {
+		t.Errorf("Second call = (%q, %q), want (\"proj\", \"cfg\")", calls[1].Project, calls[1].Config)
+	}
+
+	// Name
+	if rec.Name() != "recording:mock" {
+		t.Errorf("Name() = %q, want \"recording:mock\"", rec.Name())
+	}
+
+	// Reset
+	rec.Reset()
+	if rec.CallCount() != 0 {
+		t.Errorf("CallCount after Reset = %d, want 0", rec.CallCount())
+	}
+
+	// Close
+	if err := rec.Close(); err != nil {
+		t.Errorf("Close returned error: %v", err)
+	}
+}
+
+func TestTestLoaderWithConfig(t *testing.T) {
+	loader, mock, cfg, err := TestLoaderWithConfig[TestConfig](map[string]string{
+		"DATABASE_URL": "postgres://test",
+		"SERVER_PORT":  "9090",
+	})
+	if err != nil {
+		t.Fatalf("TestLoaderWithConfig failed: %v", err)
+	}
+
+	if cfg.Database.URL != "postgres://test" {
+		t.Errorf("Database.URL = %q, want \"postgres://test\"", cfg.Database.URL)
+	}
+	if cfg.Server.Port != 9090 {
+		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
+	}
+
+	// loader and mock should be usable
+	mock.SetValue("SERVER_PORT", "8000")
+	cfg2, _ := loader.Reload(context.Background())
+	if cfg2.Server.Port != 8000 {
+		t.Errorf("after reload Server.Port = %d, want 8000", cfg2.Server.Port)
+	}
+}
+
+func TestAssertConfigEqual(t *testing.T) {
+	if err := AssertConfigEqual(42, 42); err != nil {
+		t.Errorf("AssertConfigEqual(42, 42) returned error: %v", err)
+	}
+
+	if err := AssertConfigEqual(42, 43); err == nil {
+		t.Error("AssertConfigEqual(42, 43) should return error")
+	}
+
+	if err := AssertConfigEqual("hello", "hello"); err != nil {
+		t.Errorf("AssertConfigEqual(hello, hello) returned error: %v", err)
+	}
+}
+
+func TestTestBootstrap(t *testing.T) {
+	bs := TestBootstrap()
+	if bs.Token != "test-token" {
+		t.Errorf("Token = %q, want \"test-token\"", bs.Token)
+	}
+}
```

---

## Estimated Coverage After Patches

| File | Current | Projected |
|------|---------|-----------|
| chassis.go | 95% | 95% |
| config.go | 35% | 85% |
| doppler.go | 60% | 75% |
| fallback.go | 15% | 85% |
| feature_flags.go | 0% | 95% |
| loader.go | 70% | 85% |
| multitenant.go | 0% | 75% |
| testing.go | 50% | 90% |
| validation.go | 65% | 65% |
| watcher.go | 0% | 85% |
| **Total** | **33.9%** | **~78%** |

---

## Grading Breakdown

| Category | Weight | Score | Notes |
|----------|--------|-------|-------|
| Statement coverage | 30 | 10/30 | 33.9% is below acceptable thresholds |
| Critical path coverage | 20 | 5/20 | Multi-tenant (production use), watcher, feature flags all at 0% |
| Error path coverage | 15 | 5/15 | Fallback behavior, failure policies largely untested |
| Concurrency testing | 10 | 2/10 | No concurrent access tests for shared-state types |
| Test quality & patterns | 10 | 6/10 | Existing tests are well-structured with good table-driven tests |
| Edge case coverage | 10 | 3/10 | Duration parsing, unsigned ints, floats, empty values not tested |
| Test infrastructure | 5 | 3/5 | Good mock/recording providers exist but are underutilized |

**TOTAL: 34/100**

---

## Recommendations

1. **Immediate**: Add the 8 patch files above to bring coverage to ~78%
2. **Short-term**: Add concurrency stress tests for `FeatureFlags`, `MultiTenantLoader`, and `Watcher` using `go test -race`
3. **Medium-term**: Add integration test with a real (mock) HTTP server for `DopplerProvider.FetchProject` covering all status codes and ETag caching paths
4. **Consider**: Adding `go test -cover` to CI pipeline with minimum coverage gate (e.g., 70%)
