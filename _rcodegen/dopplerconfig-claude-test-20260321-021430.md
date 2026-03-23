Date Created: 2026-03-21T02:14:30-07:00
TOTAL_SCORE: 42/100

# dopplerconfig Test Coverage Audit

**Agent:** Claude:Opus 4.6
**Measured Statement Coverage:** 33.9%
**Test Files:** 4 (chassis_test.go, loader_test.go, loader_extended_test.go, validation_test.go)
**Test Functions:** 29
**Total Source Files:** 8 (excluding testing.go utilities)

---

## Score Breakdown

| Category | Points | Max | Notes |
|----------|--------|-----|-------|
| Statement Coverage | 8 | 25 | 33.9% measured by `go test -cover` |
| Function Coverage Breadth | 9 | 25 | ~35% of exported functions have any coverage |
| Critical Path Coverage | 8 | 20 | Core loader/validation good; feature flags, multi-tenant, watcher all 0% |
| Edge Case & Error Paths | 5 | 15 | Many provider failure and parse error paths untested |
| Test Quality & Infrastructure | 12 | 15 | Good mocks/helpers; one failing test (TestChassisVersion) |
| **TOTAL** | **42** | **100** | |

---

## Existing Test Bug

**TestChassisVersion** in `chassis_test.go:247` asserts `ChassisVersion[0] != '7'` but the project is on chassis-go v9. This test will fail if chassis-go returns a version starting with `9` (which it should). The assertion was written for v7 and never updated.

---

## Coverage Gaps by File

### 1. feature_flags.go — 0% Coverage (CRITICAL)

Every function at 0%: `NewFeatureFlags`, `IsEnabled`, `IsDisabled`, `GetInt`, `GetFloat`, `GetString`, `GetStringSlice`, `Update`, `buildKey`, `parseBool`, `FeatureFlagsFromValues`, `RolloutConfig.ShouldEnable`.

### 2. watcher.go — 0% Coverage (CRITICAL)

Every function at 0%: `NewWatcher`, `Start`, `Stop`, `IsRunning`, `run`, `poll`, `Watch`, `WatchWithCallback`, all option functions.

### 3. multitenant.go — 0% Coverage (CRITICAL)

Every function at 0%: `NewMultiTenantLoader`, `NewMultiTenantLoaderWithProvider`, `LoadEnv`, `LoadProject`, `LoadAllProjects`, `ReloadProjects`, `Project`, `Projects`, `ProjectCodes`, `Env`, `OnEnvChange`, `OnProjectChange`, `Close`, `MultiTenantWatcher.*`.

### 4. fallback.go — Mostly 0% Coverage (HIGH)

- `FileProvider.FetchProject`: 30.8% (only secval rejection path tested)
- `flattenJSON`: 0%
- `WriteFallbackFile`: 0%
- `EnvProvider.*`: all 0%
- `splitEnv`, `hasPrefix`: 0%

### 5. config.go — Partial Coverage (MEDIUM)

- `LoadBootstrapFromEnv`: 0%
- `IsEnabled`/`HasFallback`: 0%
- `SecretValue.MarshalJSON`: 0%
- `SecretValue.String` for empty: 0% (only `[REDACTED]` path tested)

### 6. loader.go — Partial Coverage (MEDIUM)

- `NewLoader`: 0% (only `NewLoaderWithProvider` tested)
- `Current`: 0%
- `Metadata`: 0%
- `Close`: 0%
- `FailurePolicyWarn` branch: 0%
- `setFieldValue` for Duration, Uint, Float: ~0%

### 7. doppler.go — Partial Coverage (MEDIUM)

- `FetchProject` ETag/304 path: 0%
- `FetchProject` error body truncation: 0%
- `IsDopplerError`: 0%
- `DopplerError.Error`: 0%
- `DopplerProvider.Name`: 0%
- `WithCallOptions`: 0%

---

## Proposed Tests with Patch-Ready Diffs

### NEW FILE: feature_flags_test.go

```diff
--- /dev/null
+++ b/feature_flags_test.go
@@ -0,0 +1,280 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestFeatureFlags_IsEnabled(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_DARK_MODE":    "true",
+		"FEATURE_BETA":         "1",
+		"FEATURE_LEGACY":       "false",
+		"FEATURE_DISABLED_ONE": "0",
+		"FEATURE_YES_FLAG":     "yes",
+		"FEATURE_ON_FLAG":      "on",
+		"FEATURE_ENABLED_FLAG": "enabled",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	tests := []struct {
+		name     string
+		flag     string
+		expected bool
+	}{
+		{"true value", "DARK_MODE", true},
+		{"1 value", "BETA", true},
+		{"false value", "LEGACY", false},
+		{"0 value", "DISABLED_ONE", false},
+		{"yes value", "YES_FLAG", true},
+		{"on value", "ON_FLAG", true},
+		{"enabled value", "ENABLED_FLAG", true},
+		{"missing flag", "NONEXISTENT", false},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := ff.IsEnabled(tt.flag)
+			if got != tt.expected {
+				t.Errorf("IsEnabled(%q) = %v, want %v", tt.flag, got, tt.expected)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_IsEnabled_CaseInsensitive(t *testing.T) {
+	values := map[string]string{
+		"feature_dark_mode": "true",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// buildKey uppercases, so lookup for "FEATURE_DARK_MODE" should find "feature_dark_mode" via case-insensitive fallback
+	if !ff.IsEnabled("dark_mode") {
+		t.Error("IsEnabled should find case-insensitive match")
+	}
+}
+
+func TestFeatureFlags_IsEnabled_Caching(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_CACHED": "true",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// First call populates cache
+	result1 := ff.IsEnabled("CACHED")
+	// Second call should hit cache
+	result2 := ff.IsEnabled("CACHED")
+
+	if result1 != result2 || result1 != true {
+		t.Errorf("IsEnabled caching inconsistent: first=%v, second=%v", result1, result2)
+	}
+}
+
+func TestFeatureFlags_IsDisabled(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_ON":  "true",
+		"FEATURE_OFF": "false",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if ff.IsDisabled("ON") {
+		t.Error("IsDisabled(ON) should be false")
+	}
+	if !ff.IsDisabled("OFF") {
+		t.Error("IsDisabled(OFF) should be true")
+	}
+	if !ff.IsDisabled("MISSING") {
+		t.Error("IsDisabled(MISSING) should be true")
+	}
+}
+
+func TestFeatureFlags_GetInt(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MAX_RETRIES": "5",
+		"FEATURE_BAD_INT":     "abc",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetInt("MAX_RETRIES", 3); got != 5 {
+		t.Errorf("GetInt(MAX_RETRIES) = %d, want 5", got)
+	}
+	if got := ff.GetInt("BAD_INT", 3); got != 3 {
+		t.Errorf("GetInt(BAD_INT) = %d, want default 3", got)
+	}
+	if got := ff.GetInt("MISSING", 99); got != 99 {
+		t.Errorf("GetInt(MISSING) = %d, want default 99", got)
+	}
+}
+
+func TestFeatureFlags_GetFloat(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_RATE":      "0.75",
+		"FEATURE_BAD_FLOAT": "xyz",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetFloat("RATE", 0.5); got != 0.75 {
+		t.Errorf("GetFloat(RATE) = %f, want 0.75", got)
+	}
+	if got := ff.GetFloat("BAD_FLOAT", 0.5); got != 0.5 {
+		t.Errorf("GetFloat(BAD_FLOAT) = %f, want default 0.5", got)
+	}
+	if got := ff.GetFloat("MISSING", 1.0); got != 1.0 {
+		t.Errorf("GetFloat(MISSING) = %f, want default 1.0", got)
+	}
+}
+
+func TestFeatureFlags_GetString(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MODE": "dark",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetString("MODE", "light"); got != "dark" {
+		t.Errorf("GetString(MODE) = %q, want \"dark\"", got)
+	}
+	if got := ff.GetString("MISSING", "light"); got != "light" {
+		t.Errorf("GetString(MISSING) = %q, want default \"light\"", got)
+	}
+}
+
+func TestFeatureFlags_GetStringSlice(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_REGIONS":     "us-east, eu-west, ap-south",
+		"FEATURE_EMPTY_SLICE": "",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	got := ff.GetStringSlice("REGIONS", nil)
+	expected := []string{"us-east", "eu-west", "ap-south"}
+	if len(got) != len(expected) {
+		t.Fatalf("GetStringSlice(REGIONS) len = %d, want %d", len(got), len(expected))
+	}
+	for i, v := range expected {
+		if got[i] != v {
+			t.Errorf("GetStringSlice(REGIONS)[%d] = %q, want %q", i, got[i], v)
+		}
+	}
+
+	// Empty value returns default
+	defSlice := []string{"default"}
+	got2 := ff.GetStringSlice("EMPTY_SLICE", defSlice)
+	if len(got2) != 1 || got2[0] != "default" {
+		t.Errorf("GetStringSlice(EMPTY_SLICE) = %v, want %v", got2, defSlice)
+	}
+
+	// Missing returns default
+	got3 := ff.GetStringSlice("MISSING", defSlice)
+	if len(got3) != 1 || got3[0] != "default" {
+		t.Errorf("GetStringSlice(MISSING) = %v, want %v", got3, defSlice)
+	}
+}
+
+func TestFeatureFlags_Update(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_A": "true",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Populate cache
+	if !ff.IsEnabled("A") {
+		t.Fatal("A should be enabled before update")
+	}
+
+	// Update values
+	ff.Update(map[string]string{
+		"FEATURE_A": "false",
+		"FEATURE_B": "true",
+	})
+
+	if ff.IsEnabled("A") {
+		t.Error("A should be disabled after update")
+	}
+	if !ff.IsEnabled("B") {
+		t.Error("B should be enabled after update")
+	}
+}
+
+func TestFeatureFlags_BuildKey_Normalization(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MY_FLAG": "true",
+	}
+
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Hyphens and spaces should be normalized to underscores
+	if !ff.IsEnabled("my-flag") {
+		t.Error("Hyphenated flag name should be normalized")
+	}
+	if !ff.IsEnabled("my flag") {
+		t.Error("Spaced flag name should be normalized")
+	}
+}
+
+func TestFeatureFlags_NoPrefix(t *testing.T) {
+	values := map[string]string{
+		"DARK_MODE": "true",
+	}
+
+	ff := NewFeatureFlags(values, "")
+
+	if !ff.IsEnabled("DARK_MODE") {
+		t.Error("Flag without prefix should work")
+	}
+}
+
+func TestFeatureFlagsFromValues(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_TEST": "true",
+	}
+
+	ff := FeatureFlagsFromValues(values)
+	if !ff.IsEnabled("TEST") {
+		t.Error("FeatureFlagsFromValues should use FEATURE_ prefix")
+	}
+}
+
+func TestParseBool(t *testing.T) {
+	tests := []struct {
+		input    string
+		expected bool
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
+		{"disabled", false},
+		{"", false},
+		{"random", false},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.input, func(t *testing.T) {
+			if got := parseBool(tt.input); got != tt.expected {
+				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.expected)
+			}
+		})
+	}
+}
+
+func TestRolloutConfig_ShouldEnable(t *testing.T) {
+	hashFunc := func(s string) uint32 {
+		// Simple deterministic hash for testing
+		var h uint32
+		for _, c := range s {
+			h = h*31 + uint32(c)
+		}
+		return h
+	}
+
+	t.Run("allow list overrides percentage", func(t *testing.T) {
+		rc := &RolloutConfig{
+			Percentage:   0,
+			AllowedUsers: []string{"vip-user"},
+		}
+		if !rc.ShouldEnable("vip-user", hashFunc) {
+			t.Error("Allowed user should be enabled even with 0% rollout")
+		}
+	})
+
+	t.Run("block list overrides percentage", func(t *testing.T) {
+		rc := &RolloutConfig{
+			Percentage:   100,
+			BlockedUsers: []string{"blocked-user"},
+		}
+		if rc.ShouldEnable("blocked-user", hashFunc) {
+			t.Error("Blocked user should be disabled even with 100% rollout")
+		}
+	})
+
+	t.Run("0 percent disables all", func(t *testing.T) {
+		rc := &RolloutConfig{Percentage: 0}
+		if rc.ShouldEnable("anyone", hashFunc) {
+			t.Error("0% rollout should disable everyone")
+		}
+	})
+
+	t.Run("100 percent enables all", func(t *testing.T) {
+		rc := &RolloutConfig{Percentage: 100}
+		if !rc.ShouldEnable("anyone", hashFunc) {
+			t.Error("100% rollout should enable everyone")
+		}
+	})
+
+	t.Run("percentage-based rollout is deterministic", func(t *testing.T) {
+		rc := &RolloutConfig{Percentage: 50}
+		result1 := rc.ShouldEnable("test-user", hashFunc)
+		result2 := rc.ShouldEnable("test-user", hashFunc)
+		if result1 != result2 {
+			t.Error("Same user should get same result")
+		}
+	})
+}
```

### NEW FILE: fallback_test.go

```diff
--- /dev/null
+++ b/fallback_test.go
@@ -0,0 +1,210 @@
+package dopplerconfig
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestFileProvider_Fetch_FlatJSON(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.json")
+
+	content := `{
+		"DATABASE_URL": "postgres://localhost/test",
+		"PORT": "8080",
+		"DEBUG": "true"
+	}`
+	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	values, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["DATABASE_URL"] != "postgres://localhost/test" {
+		t.Errorf("DATABASE_URL = %q, want postgres://localhost/test", values["DATABASE_URL"])
+	}
+	if values["PORT"] != "8080" {
+		t.Errorf("PORT = %q, want 8080", values["PORT"])
+	}
+}
+
+func TestFileProvider_Fetch_NestedJSON(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.json")
+
+	content := `{
+		"server": {
+			"port": 9090,
+			"host": "0.0.0.0"
+		},
+		"debug": true,
+		"rate": 3.14,
+		"nothing": null,
+		"tags": ["a", "b", "c"]
+	}`
+	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	values, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["server_port"] != "9090" {
+		t.Errorf("server_port = %q, want \"9090\"", values["server_port"])
+	}
+	if values["server_host"] != "0.0.0.0" {
+		t.Errorf("server_host = %q, want \"0.0.0.0\"", values["server_host"])
+	}
+	if values["debug"] != "true" {
+		t.Errorf("debug = %q, want \"true\"", values["debug"])
+	}
+	if values["rate"] != "3.14" {
+		t.Errorf("rate = %q, want \"3.14\"", values["rate"])
+	}
+	if values["nothing"] != "" {
+		t.Errorf("nothing = %q, want empty", values["nothing"])
+	}
+	if values["tags"] != "a,b,c" {
+		t.Errorf("tags = %q, want \"a,b,c\"", values["tags"])
+	}
+}
+
+func TestFileProvider_Fetch_FileNotFound(t *testing.T) {
+	fp := NewFileProvider("/nonexistent/path.json")
+	_, err := fp.Fetch(context.Background())
+	if err == nil {
+		t.Error("Fetch should fail for missing file")
+	}
+}
+
+func TestFileProvider_Fetch_InvalidJSON(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "bad.json")
+	if err := os.WriteFile(path, []byte(`{not valid`), 0600); err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	_, err := fp.Fetch(context.Background())
+	if err == nil {
+		t.Error("Fetch should fail for invalid JSON")
+	}
+}
+
+func TestFileProvider_Name(t *testing.T) {
+	fp := NewFileProvider("/tmp/config.json")
+	if fp.Name() != "file:/tmp/config.json" {
+		t.Errorf("Name() = %q, want \"file:/tmp/config.json\"", fp.Name())
+	}
+}
+
+func TestFileProvider_Close(t *testing.T) {
+	fp := NewFileProvider("/tmp/config.json")
+	if err := fp.Close(); err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestWriteFallbackFile(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "fallback.json")
+
+	values := map[string]string{
+		"KEY1": "value1",
+		"KEY2": "value2",
+	}
+
+	if err := WriteFallbackFile(path, values); err != nil {
+		t.Fatalf("WriteFallbackFile failed: %v", err)
+	}
+
+	// Read it back via FileProvider
+	fp := NewFileProvider(path)
+	got, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch after write failed: %v", err)
+	}
+
+	if got["KEY1"] != "value1" {
+		t.Errorf("KEY1 = %q, want \"value1\"", got["KEY1"])
+	}
+	if got["KEY2"] != "value2" {
+		t.Errorf("KEY2 = %q, want \"value2\"", got["KEY2"])
+	}
+
+	// Verify file permissions (0600)
+	info, err := os.Stat(path)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if info.Mode().Perm() != 0600 {
+		t.Errorf("File permissions = %o, want 0600", info.Mode().Perm())
+	}
+}
+
+func TestEnvProvider_Fetch(t *testing.T) {
+	t.Setenv("TEST_DOPPLER_FOO", "bar")
+	t.Setenv("TEST_DOPPLER_BAZ", "qux")
+
+	ep := NewEnvProvider("TEST_DOPPLER_")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["TEST_DOPPLER_FOO"] != "bar" {
+		t.Errorf("TEST_DOPPLER_FOO = %q, want \"bar\"", values["TEST_DOPPLER_FOO"])
+	}
+	if values["TEST_DOPPLER_BAZ"] != "qux" {
+		t.Errorf("TEST_DOPPLER_BAZ = %q, want \"qux\"", values["TEST_DOPPLER_BAZ"])
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
+	// Should contain at least PATH (a standard env var)
+	if _, ok := values["PATH"]; !ok {
+		t.Error("Expected to find PATH in env provider with empty prefix")
+	}
+}
+
+func TestEnvProvider_Name(t *testing.T) {
+	ep1 := NewEnvProvider("APP_")
+	if ep1.Name() != "env:APP_*" {
+		t.Errorf("Name() = %q, want \"env:APP_*\"", ep1.Name())
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
+		input string
+		key   string
+		value string
+	}{
+		{"FOO=bar", "FOO", "bar"},
+		{"KEY=val=ue", "KEY", "val=ue"},
+		{"EMPTY=", "EMPTY", ""},
+		{"NOEQUALS", "NOEQUALS", ""},
+	}
+
+	for _, tt := range tests {
+		k, v := splitEnv(tt.input)
+		if k != tt.key || v != tt.value {
+			t.Errorf("splitEnv(%q) = (%q, %q), want (%q, %q)", tt.input, k, v, tt.key, tt.value)
+		}
+	}
+}
```

### NEW FILE: watcher_test.go

```diff
--- /dev/null
+++ b/watcher_test.go
@@ -0,0 +1,134 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"sync/atomic"
+	"testing"
+	"time"
+)
+
+type WatcherTestConfig struct {
+	Value string `doppler:"VALUE" default:"initial"`
+}
+
+func TestWatcher_StartStop(t *testing.T) {
+	values := map[string]string{"VALUE": "hello"}
+	loader, _ := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](50*time.Millisecond))
+
+	if w.IsRunning() {
+		t.Error("Watcher should not be running before Start")
+	}
+
+	ctx := context.Background()
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+
+	if !w.IsRunning() {
+		t.Error("Watcher should be running after Start")
+	}
+
+	// Start again should be no-op
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Second Start failed: %v", err)
+	}
+
+	w.Stop()
+
+	if w.IsRunning() {
+		t.Error("Watcher should not be running after Stop")
+	}
+
+	// Stop again should be no-op
+	w.Stop()
+}
+
+func TestWatcher_ContextCancel(t *testing.T) {
+	values := map[string]string{"VALUE": "hello"}
+	loader, _ := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](50*time.Millisecond))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	w.Start(ctx)
+
+	cancel()
+
+	// Wait for watcher to stop
+	time.Sleep(100 * time.Millisecond)
+
+	if w.IsRunning() {
+		t.Error("Watcher should stop when context is cancelled")
+	}
+}
+
+func TestWatcher_PollReloadsConfig(t *testing.T) {
+	values := map[string]string{"VALUE": "original"}
+	loader, mock := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	var callCount atomic.Int32
+	loader.OnChange(func(old, new *WatcherTestConfig) {
+		callCount.Add(1)
+	})
+
+	w := NewWatcher(loader, WithWatchInterval[WatcherTestConfig](50*time.Millisecond))
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Change value
+	mock.SetValue("VALUE", "updated")
+
+	// Wait for at least one poll
+	time.Sleep(150 * time.Millisecond)
+	w.Stop()
+
+	if callCount.Load() == 0 {
+		t.Error("OnChange callback should have been called at least once")
+	}
+}
+
+func TestWatcher_MaxFailures(t *testing.T) {
+	values := map[string]string{"VALUE": "hello"}
+	loader, mock := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	// Make provider fail
+	mock.SetError(fmt.Errorf("provider down"))
+
+	w := NewWatcher(loader,
+		WithWatchInterval[WatcherTestConfig](50*time.Millisecond),
+		WithMaxFailures[WatcherTestConfig](3),
+	)
+
+	w.Start(context.Background())
+
+	// Wait for watcher to exceed max failures and stop itself
+	time.Sleep(500 * time.Millisecond)
+
+	if w.IsRunning() {
+		t.Error("Watcher should stop after max failures")
+	}
+}
+
+func TestWatch_ConvenienceFunction(t *testing.T) {
+	values := map[string]string{"VALUE": "hello"}
+	loader, _ := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	ctx := context.Background()
+	stop := Watch(ctx, loader, WithWatchInterval[WatcherTestConfig](50*time.Millisecond))
+
+	// Should be running
+	time.Sleep(20 * time.Millisecond)
+
+	stop()
+	// Just verify it doesn't panic
+}
+
+func TestWatchWithCallback(t *testing.T) {
+	values := map[string]string{"VALUE": "hello"}
+	loader, mock := TestLoader[WatcherTestConfig](values)
+	loader.Load(context.Background())
+
+	var changed atomic.Bool
+	stop := WatchWithCallback(ctx(), loader, func(old, new *WatcherTestConfig) {
+		changed.Store(true)
+	}, WithWatchInterval[WatcherTestConfig](50*time.Millisecond))
+
+	mock.SetValue("VALUE", "world")
+	time.Sleep(150 * time.Millisecond)
+	stop()
+
+	if !changed.Load() {
+		t.Error("Callback should have been called")
+	}
+}
+
+func ctx() context.Context { return context.Background() }
```

### NEW FILE: multitenant_test.go

```diff
--- /dev/null
+++ b/multitenant_test.go
@@ -0,0 +1,194 @@
+package dopplerconfig
+
+import (
+	"context"
+	"testing"
+	"time"
+)
+
+type EnvConfig struct {
+	Environment string `doppler:"ENVIRONMENT" default:"dev"`
+	LogLevel    string `doppler:"LOG_LEVEL" default:"info"`
+}
+
+type ProjectConfig struct {
+	Name    string `doppler:"NAME"`
+	APIKey  string `doppler:"API_KEY"`
+	Enabled bool   `doppler:"ENABLED" default:"true"`
+}
+
+func TestMultiTenantLoader_LoadEnv(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"ENVIRONMENT": "production",
+		"LOG_LEVEL":   "warn",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	cfg, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv failed: %v", err)
+	}
+
+	if cfg.Environment != "production" {
+		t.Errorf("Environment = %q, want \"production\"", cfg.Environment)
+	}
+	if cfg.LogLevel != "warn" {
+		t.Errorf("LogLevel = %q, want \"warn\"", cfg.LogLevel)
+	}
+
+	// Env() should return the loaded config
+	env := loader.Env()
+	if env == nil {
+		t.Fatal("Env() returned nil after LoadEnv")
+	}
+	if env.Environment != "production" {
+		t.Errorf("Env().Environment = %q, want \"production\"", env.Environment)
+	}
+}
+
+func TestMultiTenantLoader_LoadProject(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"NAME":    "Project Alpha",
+		"API_KEY": "key-123",
+		"ENABLED": "true",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	cfg, err := loader.LoadProject(context.Background(), "alpha")
+	if err != nil {
+		t.Fatalf("LoadProject failed: %v", err)
+	}
+
+	if cfg.Name != "Project Alpha" {
+		t.Errorf("Name = %q, want \"Project Alpha\"", cfg.Name)
+	}
+
+	// Project() should return cached config
+	cached, ok := loader.Project("alpha")
+	if !ok {
+		t.Fatal("Project(alpha) not found")
+	}
+	if cached.Name != "Project Alpha" {
+		t.Errorf("Cached Name = %q, want \"Project Alpha\"", cached.Name)
+	}
+}
+
+func TestMultiTenantLoader_LoadAllProjects(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"NAME":    "Default",
+		"ENABLED": "true",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	projects, err := loader.LoadAllProjects(context.Background(), []string{"alpha", "beta", "gamma"})
+	if err != nil {
+		t.Fatalf("LoadAllProjects failed: %v", err)
+	}
+
+	if len(projects) != 3 {
+		t.Errorf("LoadAllProjects returned %d projects, want 3", len(projects))
+	}
+
+	// ProjectCodes should be sorted
+	codes := loader.ProjectCodes()
+	if len(codes) != 3 {
+		t.Fatalf("ProjectCodes len = %d, want 3", len(codes))
+	}
+	if codes[0] != "alpha" || codes[1] != "beta" || codes[2] != "gamma" {
+		t.Errorf("ProjectCodes = %v, want [alpha beta gamma]", codes)
+	}
+
+	// Projects() should return all
+	all := loader.Projects()
+	if len(all) != 3 {
+		t.Errorf("Projects() len = %d, want 3", len(all))
+	}
+}
+
+func TestMultiTenantLoader_OnEnvChange(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"ENVIRONMENT": "dev",
+		"LOG_LEVEL":   "info",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	var oldEnv, newEnv string
+	loader.OnEnvChange(func(old, new *EnvConfig) {
+		oldEnv = old.Environment
+		newEnv = new.Environment
+	})
+
+	// First load -- no callback (old is nil)
+	loader.LoadEnv(context.Background())
+
+	// Change and reload
+	mock.SetValues(map[string]string{
+		"ENVIRONMENT": "staging",
+		"LOG_LEVEL":   "debug",
+	})
+	loader.LoadEnv(context.Background())
+
+	if oldEnv != "dev" {
+		t.Errorf("oldEnv = %q, want \"dev\"", oldEnv)
+	}
+	if newEnv != "staging" {
+		t.Errorf("newEnv = %q, want \"staging\"", newEnv)
+	}
+}
+
+func TestMultiTenantLoader_ReloadProjects(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"NAME":    "Test",
+		"ENABLED": "true",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	// Load initial projects
+	loader.LoadAllProjects(context.Background(), []string{"a", "b"})
+
+	// Register callback
+	var gotDiff *ReloadDiff
+	loader.OnProjectChange(func(diff *ReloadDiff) {
+		gotDiff = diff
+	})
+
+	// Reload
+	diff, err := loader.ReloadProjects(context.Background())
+	if err != nil {
+		t.Fatalf("ReloadProjects failed: %v", err)
+	}
+
+	if len(diff.Unchanged) != 2 {
+		t.Errorf("Unchanged = %d, want 2", len(diff.Unchanged))
+	}
+	if len(diff.Added) != 0 {
+		t.Errorf("Added = %d, want 0", len(diff.Added))
+	}
+	if len(diff.Removed) != 0 {
+		t.Errorf("Removed = %d, want 0", len(diff.Removed))
+	}
+
+	if gotDiff == nil {
+		t.Error("OnProjectChange callback was not called")
+	}
+}
+
+func TestMultiTenantLoader_Close(t *testing.T) {
+	mock := NewMockProvider(nil)
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	if err := loader.Close(); err != nil {
+		t.Errorf("Close failed: %v", err)
+	}
+}
+
+func TestMultiTenantWatcher_StartStop(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"ENVIRONMENT": "dev",
+		"NAME":        "test",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+	loader.LoadEnv(context.Background())
+	loader.LoadAllProjects(context.Background(), []string{"test"})
+
+	w := NewMultiTenantWatcher[EnvConfig, ProjectConfig](loader, 50*time.Millisecond)
+
+	if err := w.Start(context.Background()); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+
+	time.Sleep(100 * time.Millisecond)
+	w.Stop()
+
+	// Verify it stopped cleanly — no panic
+}
```

### NEW FILE: config_test.go

```diff
--- /dev/null
+++ b/config_test.go
@@ -0,0 +1,92 @@
+package dopplerconfig
+
+import (
+	"encoding/json"
+	"testing"
+)
+
+func TestLoadBootstrapFromEnv(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "dp.test.token")
+	t.Setenv("DOPPLER_PROJECT", "myproj")
+	t.Setenv("DOPPLER_CONFIG", "dev")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "/tmp/fallback.json")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "true")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "fail")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.Token != "dp.test.token" {
+		t.Errorf("Token = %q, want \"dp.test.token\"", cfg.Token)
+	}
+	if cfg.Project != "myproj" {
+		t.Errorf("Project = %q, want \"myproj\"", cfg.Project)
+	}
+	if cfg.Config != "dev" {
+		t.Errorf("Config = %q, want \"dev\"", cfg.Config)
+	}
+	if cfg.FallbackPath != "/tmp/fallback.json" {
+		t.Errorf("FallbackPath = %q", cfg.FallbackPath)
+	}
+	if !cfg.WatchEnabled {
+		t.Error("WatchEnabled should be true")
+	}
+	if cfg.FailurePolicy != FailurePolicyFail {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyFail", cfg.FailurePolicy)
+	}
+}
+
+func TestLoadBootstrapFromEnv_Defaults(t *testing.T) {
+	// Clear all env vars
+	t.Setenv("DOPPLER_TOKEN", "")
+	t.Setenv("DOPPLER_PROJECT", "")
+	t.Setenv("DOPPLER_CONFIG", "")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.FailurePolicy != FailurePolicyFallback {
+		t.Errorf("Default FailurePolicy = %d, want FailurePolicyFallback", cfg.FailurePolicy)
+	}
+	if cfg.WatchEnabled {
+		t.Error("Default WatchEnabled should be false")
+	}
+}
+
+func TestLoadBootstrapFromEnv_WarnPolicy(t *testing.T) {
+	t.Setenv("DOPPLER_FAILURE_POLICY", "warn")
+	t.Setenv("DOPPLER_TOKEN", "")
+
+	cfg := LoadBootstrapFromEnv()
+	if cfg.FailurePolicy != FailurePolicyWarn {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyWarn", cfg.FailurePolicy)
+	}
+}
+
+func TestBootstrapConfig_IsEnabled(t *testing.T) {
+	cfg := BootstrapConfig{Token: "dp.test.xxx"}
+	if !cfg.IsEnabled() {
+		t.Error("IsEnabled should be true when token is set")
+	}
+
+	cfg.Token = ""
+	if cfg.IsEnabled() {
+		t.Error("IsEnabled should be false when token is empty")
+	}
+}
+
+func TestBootstrapConfig_HasFallback(t *testing.T) {
+	cfg := BootstrapConfig{FallbackPath: "/tmp/fallback.json"}
+	if !cfg.HasFallback() {
+		t.Error("HasFallback should be true when path is set")
+	}
+
+	cfg.FallbackPath = ""
+	if cfg.HasFallback() {
+		t.Error("HasFallback should be false when path is empty")
+	}
+}
+
+func TestSecretValue_MarshalJSON(t *testing.T) {
+	sv := NewSecretValue("super-secret")
+	data, err := json.Marshal(sv)
+	if err != nil {
+		t.Fatalf("MarshalJSON failed: %v", err)
+	}
+	if string(data) != `"[REDACTED]"` {
+		t.Errorf("MarshalJSON = %s, want \"[REDACTED]\"", string(data))
+	}
+}
+
+func TestSecretValue_String_Empty(t *testing.T) {
+	sv := NewSecretValue("")
+	if sv.String() != "[empty]" {
+		t.Errorf("String() = %q, want \"[empty]\"", sv.String())
+	}
+}
```

### NEW FILE: doppler_test.go

```diff
--- /dev/null
+++ b/doppler_test.go
@@ -0,0 +1,136 @@
+package dopplerconfig
+
+import (
+	"context"
+	"errors"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+)
+
+func TestNewDopplerProvider_EmptyToken(t *testing.T) {
+	_, err := NewDopplerProvider("", "proj", "dev")
+	if err == nil {
+		t.Error("NewDopplerProvider should fail with empty token")
+	}
+}
+
+func TestDopplerProvider_Name(t *testing.T) {
+	p, err := NewDopplerProvider("token", "proj", "dev")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if p.Name() != "doppler" {
+		t.Errorf("Name() = %q, want \"doppler\"", p.Name())
+	}
+}
+
+func TestDopplerProvider_FetchProject_ETagCaching(t *testing.T) {
+	callCount := 0
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		callCount++
+		if r.Header.Get("If-None-Match") == `"etag-123"` {
+			w.WriteHeader(http.StatusNotModified)
+			return
+		}
+		w.Header().Set("ETag", `"etag-123"`)
+		w.Header().Set("Content-Type", "application/json")
+		w.WriteHeader(http.StatusOK)
+		w.Write([]byte(`{"secrets":{"KEY":{"raw":"val"}}}`))
+	}))
+	defer srv.Close()
+
+	p, err := NewDopplerProvider("token", "proj", "dev",
+		WithAPIURL(srv.URL),
+		WithHTTPClient(srv.Client()),
+	)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	// First fetch — full response
+	vals, err := p.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("First fetch failed: %v", err)
+	}
+	if vals["KEY"] != "val" {
+		t.Errorf("KEY = %q, want \"val\"", vals["KEY"])
+	}
+
+	// Second fetch — should get 304 and use cache
+	vals2, err := p.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Second fetch failed: %v", err)
+	}
+	if vals2["KEY"] != "val" {
+		t.Errorf("Cached KEY = %q, want \"val\"", vals2["KEY"])
+	}
+	if callCount != 2 {
+		t.Errorf("Server called %d times, want 2", callCount)
+	}
+}
+
+func TestDopplerProvider_FetchProject_ErrorResponse(t *testing.T) {
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		w.WriteHeader(http.StatusForbidden)
+		w.Write([]byte(`{"error":"forbidden"}`))
+	}))
+	defer srv.Close()
+
+	p, err := NewDopplerProvider("token", "proj", "dev",
+		WithAPIURL(srv.URL),
+		WithHTTPClient(srv.Client()),
+	)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	_, err = p.Fetch(context.Background())
+	if err == nil {
+		t.Fatal("Fetch should fail for 403")
+	}
+
+	de, ok := IsDopplerError(err)
+	if !ok {
+		t.Fatalf("Expected DopplerError, got %T: %v", err, err)
+	}
+	if de.StatusCode != 403 {
+		t.Errorf("StatusCode = %d, want 403", de.StatusCode)
+	}
+}
+
+func TestDopplerError_Error(t *testing.T) {
+	de := &DopplerError{StatusCode: 500, Message: "internal"}
+	expected := "doppler error 500: internal"
+	if de.Error() != expected {
+		t.Errorf("Error() = %q, want %q", de.Error(), expected)
+	}
+}
+
+func TestIsDopplerError(t *testing.T) {
+	de := &DopplerError{StatusCode: 404, Message: "not found"}
+
+	// Direct match
+	found, ok := IsDopplerError(de)
+	if !ok || found.StatusCode != 404 {
+		t.Error("IsDopplerError should find direct DopplerError")
+	}
+
+	// Not a DopplerError
+	_, ok = IsDopplerError(errors.New("something else"))
+	if ok {
+		t.Error("IsDopplerError should return false for non-DopplerError")
+	}
+
+	// Nil error
+	_, ok = IsDopplerError(nil)
+	if ok {
+		t.Error("IsDopplerError should return false for nil")
+	}
+}
+
+func TestDopplerProvider_WithAPIURL(t *testing.T) {
+	p, err := NewDopplerProvider("token", "proj", "dev",
+		WithAPIURL("https://custom.api.example.com"),
+	)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if p.apiURL != "https://custom.api.example.com" {
+		t.Errorf("apiURL = %q, want custom URL", p.apiURL)
+	}
+}
```

### ADDITIONS TO: loader_test.go (additional edge cases)

```diff
--- a/loader_test.go
+++ b/loader_test.go
@@ -179,3 +179,97 @@
 	}
 }
+
+func TestLoader_Current(t *testing.T) {
+	values := map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	}
+
+	loader, _ := TestLoader[TestConfig](values)
+
+	// Current before Load should be nil
+	if loader.Current() != nil {
+		t.Error("Current() should be nil before Load")
+	}
+
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	// Current after Load should match
+	current := loader.Current()
+	if current == nil {
+		t.Fatal("Current() should not be nil after Load")
+	}
+	if current.Database.URL != cfg.Database.URL {
+		t.Errorf("Current().Database.URL = %q, want %q", current.Database.URL, cfg.Database.URL)
+	}
+}
+
+func TestLoader_Metadata(t *testing.T) {
+	values := map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	}
+
+	loader, _ := TestLoader[TestConfig](values)
+	loader.Load(context.Background())
+
+	meta := loader.Metadata()
+	if meta.Source != "mock" {
+		t.Errorf("Metadata().Source = %q, want \"mock\"", meta.Source)
+	}
+	if meta.KeyCount != 1 {
+		t.Errorf("Metadata().KeyCount = %d, want 1", meta.KeyCount)
+	}
+	if meta.LoadedAt.IsZero() {
+		t.Error("Metadata().LoadedAt should not be zero")
+	}
+}
+
+func TestLoader_Close(t *testing.T) {
+	values := map[string]string{
+		"DATABASE_URL": "postgres://localhost/test",
+	}
+
+	loader, _ := TestLoader[TestConfig](values)
+	if err := loader.Close(); err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestLoader_Fallback(t *testing.T) {
+	// Primary provider fails, should fall back to secondary
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
+}
+
+type DurationConfig struct {
+	Timeout  time.Duration `doppler:"TIMEOUT"`
+	TimeoutS time.Duration `doppler:"TIMEOUT_SECS"`
+}
+
+func TestLoader_Duration(t *testing.T) {
+	values := map[string]string{
+		"TIMEOUT":      "30s",
+		"TIMEOUT_SECS": "60",
+	}
+
+	loader, _ := TestLoader[DurationConfig](values)
+	cfg, err := loader.Load(context.Background())
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.Timeout != 30*time.Second {
+		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
+	}
+	if cfg.TimeoutS != 60*time.Second {
+		t.Errorf("TimeoutS = %v, want 60s (parsed from raw seconds)", cfg.TimeoutS)
+	}
+}
```

(Note: the `loader_test.go` diff requires adding `"fmt"` and `"time"` to the import block.)

### ADDITIONS TO: validation_test.go (edge cases)

```diff
--- a/validation_test.go
+++ b/validation_test.go
@@ -132,3 +132,56 @@
 	}
 }
+
+func TestValidationErrors_Error_Multiple(t *testing.T) {
+	errs := ValidationErrors{
+		{Field: "A", Message: "too small", Value: 1},
+		{Field: "B", Message: "required", Value: nil},
+	}
+
+	msg := errs.Error()
+	if !strings.Contains(msg, "2 validation errors") {
+		t.Errorf("Error() should mention count: %q", msg)
+	}
+}
+
+func TestValidationErrors_Error_Empty(t *testing.T) {
+	errs := ValidationErrors{}
+	if errs.Error() != "no validation errors" {
+		t.Errorf("Error() = %q, want \"no validation errors\"", errs.Error())
+	}
+}
+
+func TestValidate_CustomValidator(t *testing.T) {
+	cfg := &customValidatorConfig{Value: "bad"}
+	err := Validate(cfg)
+	if err == nil {
+		t.Error("Expected custom validation error")
+	}
+
+	cfg.Value = "good"
+	if err := Validate(cfg); err != nil {
+		t.Errorf("Unexpected error for valid config: %v", err)
+	}
+}
+
+type customValidatorConfig struct {
+	Value string
+}
+
+func (c *customValidatorConfig) Validate() error {
+	if c.Value == "bad" {
+		return fmt.Errorf("value cannot be bad")
+	}
+	return nil
+}
+
+func TestValidate_NonStruct(t *testing.T) {
+	err := Validate("not a struct")
+	if err == nil {
+		t.Error("Validate should fail for non-struct")
+	}
+}
+
+func TestValidate_HostWithPort(t *testing.T) {
+	c := ValidationConfig{Host: "localhost:8080", Required: "x", MinVal: 10}
+	if err := Validate(c); err != nil {
+		t.Errorf("Host with port should be valid: %v", err)
+	}
+}
```

(Note: the `validation_test.go` diff requires adding `"fmt"` and `"strings"` to the import block.)

---

## Coverage Impact Estimate

If all proposed tests were implemented:

| File | Current Coverage | Projected Coverage |
|------|----------------:|-------------------:|
| feature_flags.go | 0% | ~90% |
| watcher.go | 0% | ~75% |
| multitenant.go | 0% | ~60% |
| fallback.go | ~10% | ~85% |
| config.go | ~25% | ~90% |
| doppler.go | ~55% | ~80% |
| loader.go | ~55% | ~75% |
| validation.go | ~65% | ~80% |
| **Overall** | **33.9%** | **~75%** |

---

## Priority Ranking

1. **feature_flags_test.go** (CRITICAL) — Entire subsystem untested, high business value
2. **fallback_test.go** (CRITICAL) — Fallback providers are the safety net; must be tested
3. **multitenant_test.go** (HIGH) — Complex concurrent loading logic with zero coverage
4. **config_test.go** (HIGH) — Bootstrap and SecretValue used by every consumer
5. **doppler_test.go** (HIGH) — Core provider with ETag caching and error handling gaps
6. **watcher_test.go** (MEDIUM) — Timing-sensitive; important but less likely to have subtle bugs
7. **loader_test.go additions** (MEDIUM) — Fills specific gaps (Current, Metadata, Duration, Fallback)
8. **validation_test.go additions** (LOW) — Existing coverage is decent; adds edge cases

---

## Additional Observations

- **TestChassisVersion** (chassis_test.go:247) checks for version starting with `7` but the project uses chassis-go v9. This test likely fails on every run.
- **No integration test for `NewLoader`** — only `NewLoaderWithProvider` is used in tests. The actual constructor path with `NewDopplerProvider` creation is untested.
- **No test for `FailurePolicyWarn`** branch in `loadFromProvider` — this fallback to empty defaults is a critical safety path.
- **No concurrent access tests** — Many types use `sync.RWMutex` but there are no tests verifying thread-safety under concurrent access (e.g., using `t.Run` with goroutines or `-race` flag).
- **`RecordingProvider`** is defined in testing.go but never used in any test, suggesting it was written speculatively.
