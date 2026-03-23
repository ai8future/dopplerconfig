Date Created: 2026-03-21T09:15:00-05:00
TOTAL_SCORE: 38/100

# dopplerconfig — Unit Test Coverage Report

**Agent:** Claude:Opus 4.6
**Module:** `github.com/ai8future/dopplerconfig`
**Go Version:** 1.25.5
**Primary Dependency:** chassis-go v9.0.0

---

## Executive Summary

The dopplerconfig module has **significant test coverage gaps**. Of ~130 exported and internal functions/methods, approximately **50 (38%) have direct or indirect test coverage**. Four entire source files — `feature_flags.go`, `watcher.go`, `multitenant.go`, and `fallback.go` — are **partially or completely untested**.

### Coverage by File

| File | Functions | Tested | Coverage |
|------|-----------|--------|----------|
| `doppler.go` | 14 | 10 | 71% |
| `config.go` | 8 | 6 | 75% |
| `loader.go` | 12 | 10 | 83% |
| `validation.go` | 14 | 14 | 100% |
| `chassis.go` | 5 | 5 | 100% |
| `multitenant.go` | 19 | 0 | **0%** |
| `fallback.go` | 12 | 1 | **8%** |
| `feature_flags.go` | 14 | 0 | **0%** |
| `watcher.go` | 8 | 0 | **0%** |
| `testing.go` | 12 | 8 | 67% |

### Score Breakdown

| Category | Max Points | Score | Notes |
|----------|-----------|-------|-------|
| Core loading (loader.go) | 20 | 17 | Good coverage, missing edge cases |
| Validation (validation.go) | 15 | 15 | Fully tested |
| Chassis integration | 10 | 10 | Well tested |
| Multi-tenant (multitenant.go) | 20 | 0 | **Completely untested** |
| Feature flags (feature_flags.go) | 15 | 0 | **Completely untested** |
| Watcher (watcher.go) | 10 | 0 | **Completely untested** |
| Fallback providers (fallback.go) | 10 | 1 | Only secval tested via chassis_test.go |

**TOTAL: 38/100** — Critical gaps in multi-tenant, feature flags, and watcher functionality.

---

## Untested Functions Inventory

### feature_flags.go (0% coverage — 14 functions)
- `NewFeatureFlags()`
- `FeatureFlags.IsEnabled()`
- `FeatureFlags.IsDisabled()`
- `FeatureFlags.GetInt()`
- `FeatureFlags.GetFloat()`
- `FeatureFlags.GetString()`
- `FeatureFlags.GetStringSlice()`
- `FeatureFlags.Update()`
- `FeatureFlags.buildKey()`
- `parseBool()`
- `RolloutConfig.ShouldEnable()`
- `FeatureFlagsFromValues()`

### multitenant.go (0% coverage — 19 functions)
- `NewMultiTenantLoader[E, P]()`
- `NewMultiTenantLoaderWithProvider[E, P]()`
- `multiTenantLoader.LoadEnv()`
- `multiTenantLoader.LoadProject()`
- `multiTenantLoader.LoadAllProjects()`
- `multiTenantLoader.ReloadProjects()`
- `multiTenantLoader.Project()`
- `multiTenantLoader.Projects()`
- `multiTenantLoader.ProjectCodes()`
- `multiTenantLoader.Env()`
- `multiTenantLoader.OnEnvChange()`
- `multiTenantLoader.OnProjectChange()`
- `multiTenantLoader.Close()`
- `multiTenantLoader.fetchWithFallback()`
- `multiTenantLoader.fetchAndParse()`
- `multiTenantLoader.updateProjectKeys()`
- `NewMultiTenantWatcher[E, P]()`
- `MultiTenantWatcher.Start()`
- `MultiTenantWatcher.Stop()`

### watcher.go (0% coverage — 8 functions)
- `NewWatcher[T]()`
- `Watcher.Start()`
- `Watcher.Stop()`
- `Watcher.IsRunning()`
- `Watcher.poll()`
- `Watcher.run()`
- `Watch[T]()`
- `WatchWithCallback[T]()`

### fallback.go (8% coverage — 11 of 12 untested)
- `NewFileProvider()`
- `FileProvider.Fetch()`
- `FileProvider.FetchProject()`
- `flattenJSON()`
- `WriteFallbackFile()`
- `NewEnvProvider()`
- `EnvProvider.Fetch()`
- `EnvProvider.FetchProject()`
- `splitEnv()`
- `hasPrefix()`
- `EnvProvider.Name()`

### config.go (missing edge cases)
- `LoadBootstrapFromEnv()` — failure policy parsing untested
- `SecretValue.String()` — empty case untested (returns "[empty]")
- `SecretValue.MarshalJSON()` — no direct test

---

## Proposed Test Files — Patch-Ready Diffs

### 1. feature_flags_test.go (NEW FILE)

```diff
--- /dev/null
+++ b/feature_flags_test.go
@@ -0,0 +1,312 @@
+package dopplerconfig
+
+import (
+	"sync"
+	"testing"
+)
+
+func TestNewFeatureFlags(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_DARK_MODE": "true",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+	if ff == nil {
+		t.Fatal("NewFeatureFlags returned nil")
+	}
+	if ff.prefix != "FEATURE_" {
+		t.Errorf("prefix = %q, want %q", ff.prefix, "FEATURE_")
+	}
+}
+
+func TestFeatureFlags_IsEnabled(t *testing.T) {
+	tests := []struct {
+		name     string
+		values   map[string]string
+		prefix   string
+		flag     string
+		expected bool
+	}{
+		{
+			name:     "true value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "true"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "1 value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "1"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "yes value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "yes"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "on value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "on"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "enabled value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "enabled"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "enable value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "enable"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "false value",
+			values:   map[string]string{"FEATURE_DARK_MODE": "false"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: false,
+		},
+		{
+			name:     "empty string",
+			values:   map[string]string{"FEATURE_DARK_MODE": ""},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: false,
+		},
+		{
+			name:     "missing flag",
+			values:   map[string]string{},
+			prefix:   "FEATURE_",
+			flag:     "NONEXISTENT",
+			expected: false,
+		},
+		{
+			name:     "no prefix",
+			values:   map[string]string{"DARK_MODE": "true"},
+			prefix:   "",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+		{
+			name:     "case-insensitive lookup",
+			values:   map[string]string{"feature_dark_mode": "true"},
+			prefix:   "FEATURE_",
+			flag:     "DARK_MODE",
+			expected: true,
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			ff := NewFeatureFlags(tt.values, tt.prefix)
+			got := ff.IsEnabled(tt.flag)
+			if got != tt.expected {
+				t.Errorf("IsEnabled(%q) = %v, want %v", tt.flag, got, tt.expected)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_IsEnabled_CacheHit(t *testing.T) {
+	values := map[string]string{"FEATURE_X": "true"}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// First call populates cache
+	result1 := ff.IsEnabled("X")
+	// Second call should hit cache
+	result2 := ff.IsEnabled("X")
+
+	if result1 != result2 {
+		t.Errorf("cache inconsistency: first=%v, second=%v", result1, result2)
+	}
+	if !result1 {
+		t.Error("expected true from cache")
+	}
+}
+
+func TestFeatureFlags_IsDisabled(t *testing.T) {
+	values := map[string]string{"FEATURE_X": "false"}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if !ff.IsDisabled("X") {
+		t.Error("IsDisabled should return true for disabled flag")
+	}
+
+	values2 := map[string]string{"FEATURE_Y": "true"}
+	ff2 := NewFeatureFlags(values2, "FEATURE_")
+
+	if ff2.IsDisabled("Y") {
+		t.Error("IsDisabled should return false for enabled flag")
+	}
+}
+
+func TestFeatureFlags_GetInt(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_MAX_RETRIES": "5",
+		"FEATURE_BAD_INT":     "not-a-number",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetInt("MAX_RETRIES", 3); got != 5 {
+		t.Errorf("GetInt(MAX_RETRIES) = %d, want 5", got)
+	}
+	if got := ff.GetInt("BAD_INT", 3); got != 3 {
+		t.Errorf("GetInt(BAD_INT) = %d, want default 3", got)
+	}
+	if got := ff.GetInt("MISSING", 42); got != 42 {
+		t.Errorf("GetInt(MISSING) = %d, want default 42", got)
+	}
+}
+
+func TestFeatureFlags_GetFloat(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_RATE":     "0.75",
+		"FEATURE_BAD_RATE": "abc",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetFloat("RATE", 1.0); got != 0.75 {
+		t.Errorf("GetFloat(RATE) = %f, want 0.75", got)
+	}
+	if got := ff.GetFloat("BAD_RATE", 1.0); got != 1.0 {
+		t.Errorf("GetFloat(BAD_RATE) = %f, want default 1.0", got)
+	}
+	if got := ff.GetFloat("MISSING", 2.5); got != 2.5 {
+		t.Errorf("GetFloat(MISSING) = %f, want default 2.5", got)
+	}
+}
+
+func TestFeatureFlags_GetString(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_REGION": "us-east-1",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	if got := ff.GetString("REGION", "us-west-2"); got != "us-east-1" {
+		t.Errorf("GetString(REGION) = %q, want %q", got, "us-east-1")
+	}
+	if got := ff.GetString("MISSING", "fallback"); got != "fallback" {
+		t.Errorf("GetString(MISSING) = %q, want %q", got, "fallback")
+	}
+}
+
+func TestFeatureFlags_GetStringSlice(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_REGIONS":   "us-east-1, eu-west-1, ap-south-1",
+		"FEATURE_EMPTY_VAL": "",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	got := ff.GetStringSlice("REGIONS", nil)
+	expected := []string{"us-east-1", "eu-west-1", "ap-south-1"}
+	if len(got) != len(expected) {
+		t.Fatalf("GetStringSlice length = %d, want %d", len(got), len(expected))
+	}
+	for i, v := range expected {
+		if got[i] != v {
+			t.Errorf("GetStringSlice[%d] = %q, want %q", i, got[i], v)
+		}
+	}
+
+	// Empty value returns default
+	defaultSlice := []string{"default"}
+	got2 := ff.GetStringSlice("EMPTY_VAL", defaultSlice)
+	if len(got2) != 1 || got2[0] != "default" {
+		t.Errorf("GetStringSlice(empty) = %v, want %v", got2, defaultSlice)
+	}
+
+	// Missing key returns default
+	got3 := ff.GetStringSlice("MISSING", defaultSlice)
+	if len(got3) != 1 || got3[0] != "default" {
+		t.Errorf("GetStringSlice(missing) = %v, want %v", got3, defaultSlice)
+	}
+}
+
+func TestFeatureFlags_Update(t *testing.T) {
+	values := map[string]string{"FEATURE_X": "true"}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	// Populate cache
+	if !ff.IsEnabled("X") {
+		t.Fatal("expected X to be enabled initially")
+	}
+
+	// Update to disable X
+	ff.Update(map[string]string{"FEATURE_X": "false"})
+
+	// Cache should be cleared, new value should be returned
+	if ff.IsEnabled("X") {
+		t.Error("expected X to be disabled after Update")
+	}
+}
+
+func TestFeatureFlags_BuildKey(t *testing.T) {
+	ff := NewFeatureFlags(nil, "FEATURE_")
+
+	tests := []struct {
+		input    string
+		expected string
+	}{
+		{"dark_mode", "FEATURE_DARK_MODE"},
+		{"DARK_MODE", "FEATURE_DARK_MODE"},
+		{"dark-mode", "FEATURE_DARK_MODE"},
+		{"dark mode", "FEATURE_DARK_MODE"},
+		{"FEATURE_ALREADY_PREFIXED", "FEATURE_ALREADY_PREFIXED"},
+	}
+
+	for _, tt := range tests {
+		got := ff.buildKey(tt.input)
+		if got != tt.expected {
+			t.Errorf("buildKey(%q) = %q, want %q", tt.input, got, tt.expected)
+		}
+	}
+}
+
+func TestFeatureFlags_BuildKey_NoPrefix(t *testing.T) {
+	ff := NewFeatureFlags(nil, "")
+
+	// Without a prefix, buildKey returns the name as-is
+	got := ff.buildKey("my_flag")
+	if got != "my_flag" {
+		t.Errorf("buildKey with no prefix = %q, want %q", got, "my_flag")
+	}
+}
+
+func TestParseBool(t *testing.T) {
+	truths := []string{"true", "1", "yes", "on", "enabled", "enable", "TRUE", "Yes", " true "}
+	for _, s := range truths {
+		if !parseBool(s) {
+			t.Errorf("parseBool(%q) = false, want true", s)
+		}
+	}
+
+	falses := []string{"false", "0", "no", "off", "disabled", "", "random"}
+	for _, s := range falses {
+		if parseBool(s) {
+			t.Errorf("parseBool(%q) = true, want false", s)
+		}
+	}
+}
+
+func TestFeatureFlagsFromValues(t *testing.T) {
+	values := map[string]string{"FEATURE_X": "true"}
+	ff := FeatureFlagsFromValues(values)
+
+	if ff.prefix != "FEATURE_" {
+		t.Errorf("prefix = %q, want %q", ff.prefix, "FEATURE_")
+	}
+	if !ff.IsEnabled("X") {
+		t.Error("expected X to be enabled via FeatureFlagsFromValues")
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
+		name     string
+		config   RolloutConfig
+		userID   string
+		expected bool
+	}{
+		{
+			name:     "allowed user always enabled",
+			config:   RolloutConfig{Percentage: 0, AllowedUsers: []string{"admin"}},
+			userID:   "admin",
+			expected: true,
+		},
+		{
+			name:     "blocked user always disabled",
+			config:   RolloutConfig{Percentage: 100, BlockedUsers: []string{"banned"}},
+			userID:   "banned",
+			expected: false,
+		},
+		{
+			name:     "0 percent disables all",
+			config:   RolloutConfig{Percentage: 0},
+			userID:   "user123",
+			expected: false,
+		},
+		{
+			name:     "100 percent enables all",
+			config:   RolloutConfig{Percentage: 100},
+			userID:   "user123",
+			expected: true,
+		},
+		{
+			name:     "negative percentage disables",
+			config:   RolloutConfig{Percentage: -5},
+			userID:   "user123",
+			expected: false,
+		},
+		{
+			name:     "allowed takes priority over blocked",
+			config:   RolloutConfig{Percentage: 0, AllowedUsers: []string{"both"}, BlockedUsers: []string{"both"}},
+			userID:   "both",
+			expected: true, // AllowedUsers is checked first
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := tt.config.ShouldEnable(tt.userID, hash)
+			if got != tt.expected {
+				t.Errorf("ShouldEnable(%q) = %v, want %v", tt.userID, got, tt.expected)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_ConcurrentAccess(t *testing.T) {
+	values := map[string]string{
+		"FEATURE_X": "true",
+		"FEATURE_Y": "false",
+	}
+	ff := NewFeatureFlags(values, "FEATURE_")
+
+	var wg sync.WaitGroup
+	for i := 0; i < 100; i++ {
+		wg.Add(1)
+		go func() {
+			defer wg.Done()
+			ff.IsEnabled("X")
+			ff.IsDisabled("Y")
+			ff.GetInt("X", 0)
+			ff.GetString("X", "")
+		}()
+	}
+	wg.Wait()
+	// No race condition = pass
+}
```

---

### 2. fallback_test.go (NEW FILE)

```diff
--- /dev/null
+++ b/fallback_test.go
@@ -0,0 +1,238 @@
+package dopplerconfig
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestFileProvider_Fetch(t *testing.T) {
+	// Create a temp file with valid JSON
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.json")
+	err := os.WriteFile(path, []byte(`{"SERVER_PORT": "8080", "DB_HOST": "localhost"}`), 0600)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	values, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["SERVER_PORT"] != "8080" {
+		t.Errorf("SERVER_PORT = %q, want %q", values["SERVER_PORT"], "8080")
+	}
+	if values["DB_HOST"] != "localhost" {
+		t.Errorf("DB_HOST = %q, want %q", values["DB_HOST"], "localhost")
+	}
+}
+
+func TestFileProvider_FetchProject(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.json")
+	err := os.WriteFile(path, []byte(`{"KEY": "value"}`), 0600)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	// project/config params are ignored for FileProvider
+	values, err := fp.FetchProject(context.Background(), "any-project", "any-config")
+	if err != nil {
+		t.Fatalf("FetchProject failed: %v", err)
+	}
+	if values["KEY"] != "value" {
+		t.Errorf("KEY = %q, want %q", values["KEY"], "value")
+	}
+}
+
+func TestFileProvider_FileNotFound(t *testing.T) {
+	fp := NewFileProvider("/nonexistent/path/config.json")
+	_, err := fp.Fetch(context.Background())
+	if err == nil {
+		t.Error("expected error for missing file")
+	}
+}
+
+func TestFileProvider_InvalidJSON(t *testing.T) {
+	dir := t.TempDir()
+	path := filepath.Join(dir, "bad.json")
+	err := os.WriteFile(path, []byte(`not json at all`), 0600)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	fp := NewFileProvider(path)
+	_, err = fp.Fetch(context.Background())
+	if err == nil {
+		t.Error("expected error for invalid JSON")
+	}
+}
+
+func TestFileProvider_Name(t *testing.T) {
+	fp := NewFileProvider("/tmp/test.json")
+	if got := fp.Name(); got != "file:/tmp/test.json" {
+		t.Errorf("Name() = %q, want %q", got, "file:/tmp/test.json")
+	}
+}
+
+func TestFileProvider_Close(t *testing.T) {
+	fp := NewFileProvider("/tmp/test.json")
+	if err := fp.Close(); err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestFlattenJSON(t *testing.T) {
+	tests := []struct {
+		name     string
+		input    map[string]interface{}
+		expected map[string]string
+	}{
+		{
+			name:  "flat string map",
+			input: map[string]interface{}{"KEY": "value"},
+			expected: map[string]string{"KEY": "value"},
+		},
+		{
+			name: "nested object",
+			input: map[string]interface{}{
+				"server": map[string]interface{}{
+					"port": float64(8080),
+					"host": "localhost",
+				},
+			},
+			expected: map[string]string{
+				"server_port": "8080",
+				"server_host": "localhost",
+			},
+		},
+		{
+			name:  "integer float",
+			input: map[string]interface{}{"COUNT": float64(42)},
+			expected: map[string]string{"COUNT": "42"},
+		},
+		{
+			name:  "fractional float",
+			input: map[string]interface{}{"RATE": float64(3.14)},
+			expected: map[string]string{"RATE": "3.14"},
+		},
+		{
+			name:  "boolean true",
+			input: map[string]interface{}{"ENABLED": true},
+			expected: map[string]string{"ENABLED": "true"},
+		},
+		{
+			name:  "boolean false",
+			input: map[string]interface{}{"ENABLED": false},
+			expected: map[string]string{"ENABLED": "false"},
+		},
+		{
+			name:  "null value",
+			input: map[string]interface{}{"EMPTY": nil},
+			expected: map[string]string{"EMPTY": ""},
+		},
+		{
+			name:  "array value",
+			input: map[string]interface{}{"TAGS": []interface{}{"a", "b", "c"}},
+			expected: map[string]string{"TAGS": "a,b,c"},
+		},
+		{
+			name: "deeply nested",
+			input: map[string]interface{}{
+				"a": map[string]interface{}{
+					"b": map[string]interface{}{
+						"c": "deep",
+					},
+				},
+			},
+			expected: map[string]string{"a_b_c": "deep"},
+		},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			result := make(map[string]string)
+			flattenJSON("", tt.input, result)
+			for k, want := range tt.expected {
+				if got, ok := result[k]; !ok {
+					t.Errorf("missing key %q", k)
+				} else if got != want {
+					t.Errorf("result[%q] = %q, want %q", k, got, want)
+				}
+			}
+		})
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
+	// Verify the file can be read back by FileProvider
+	fp := NewFileProvider(path)
+	got, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Read back failed: %v", err)
+	}
+
+	if got["KEY1"] != "value1" {
+		t.Errorf("KEY1 = %q, want %q", got["KEY1"], "value1")
+	}
+	if got["KEY2"] != "value2" {
+		t.Errorf("KEY2 = %q, want %q", got["KEY2"], "value2")
+	}
+
+	// Verify file permissions
+	info, err := os.Stat(path)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if info.Mode().Perm() != 0600 {
+		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
+	}
+}
+
+func TestEnvProvider_Fetch(t *testing.T) {
+	// Set test env vars
+	t.Setenv("DOPPLERTEST_KEY1", "val1")
+	t.Setenv("DOPPLERTEST_KEY2", "val2")
+
+	ep := NewEnvProvider("DOPPLERTEST_")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["DOPPLERTEST_KEY1"] != "val1" {
+		t.Errorf("KEY1 = %q, want %q", values["DOPPLERTEST_KEY1"], "val1")
+	}
+	if values["DOPPLERTEST_KEY2"] != "val2" {
+		t.Errorf("KEY2 = %q, want %q", values["DOPPLERTEST_KEY2"], "val2")
+	}
+}
+
+func TestEnvProvider_FetchNoPrefix(t *testing.T) {
+	t.Setenv("DOPPLERTEST_NOPREFIX", "hello")
+
+	ep := NewEnvProvider("")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+
+	if values["DOPPLERTEST_NOPREFIX"] != "hello" {
+		t.Error("expected DOPPLERTEST_NOPREFIX in unprefixed results")
+	}
+}
+
+func TestEnvProvider_Name(t *testing.T) {
+	ep1 := NewEnvProvider("APP_")
+	if got := ep1.Name(); got != "env:APP_*" {
+		t.Errorf("Name() = %q, want %q", got, "env:APP_*")
+	}
+
+	ep2 := NewEnvProvider("")
+	if got := ep2.Name(); got != "env" {
+		t.Errorf("Name() = %q, want %q", got, "env")
+	}
+}
+
+func TestSplitEnv(t *testing.T) {
+	tests := []struct {
+		input    string
+		wantKey  string
+		wantVal  string
+	}{
+		{"KEY=value", "KEY", "value"},
+		{"KEY=", "KEY", ""},
+		{"KEY=val=ue", "KEY", "val=ue"},
+		{"NOEQUALS", "NOEQUALS", ""},
+	}
+
+	for _, tt := range tests {
+		k, v := splitEnv(tt.input)
+		if k != tt.wantKey || v != tt.wantVal {
+			t.Errorf("splitEnv(%q) = (%q, %q), want (%q, %q)", tt.input, k, v, tt.wantKey, tt.wantVal)
+		}
+	}
+}
+
+func TestHasPrefix(t *testing.T) {
+	if !hasPrefix("APP_KEY", "APP_") {
+		t.Error("hasPrefix(APP_KEY, APP_) should be true")
+	}
+	if hasPrefix("OTHER_KEY", "APP_") {
+		t.Error("hasPrefix(OTHER_KEY, APP_) should be false")
+	}
+	if hasPrefix("AP", "APP_") {
+		t.Error("hasPrefix(AP, APP_) should be false (shorter than prefix)")
+	}
+}
```

---

### 3. watcher_test.go (NEW FILE)

```diff
--- /dev/null
+++ b/watcher_test.go
@@ -0,0 +1,174 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"testing"
+	"time"
+)
+
+type WatchTestConfig struct {
+	Value string `doppler:"VALUE" default:"initial"`
+}
+
+func TestNewWatcher_Defaults(t *testing.T) {
+	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	w := NewWatcher[WatchTestConfig](loader)
+
+	if w.interval != 30*time.Second {
+		t.Errorf("default interval = %v, want 30s", w.interval)
+	}
+	if w.maxFailures != 0 {
+		t.Errorf("default maxFailures = %d, want 0 (unlimited)", w.maxFailures)
+	}
+	if w.IsRunning() {
+		t.Error("watcher should not be running before Start")
+	}
+}
+
+func TestNewWatcher_WithOptions(t *testing.T) {
+	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	w := NewWatcher[WatchTestConfig](loader,
+		WithWatchInterval[WatchTestConfig](5*time.Second),
+		WithMaxFailures[WatchTestConfig](3),
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
+	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	w := NewWatcher[WatchTestConfig](loader,
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+	)
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
+	// Double start should be a no-op
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("double Start failed: %v", err)
+	}
+
+	w.Stop()
+
+	if w.IsRunning() {
+		t.Error("watcher should not be running after Stop")
+	}
+
+	// Double stop should be safe
+	w.Stop()
+}
+
+func TestWatcher_ContextCancellation(t *testing.T) {
+	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	w := NewWatcher[WatchTestConfig](loader,
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+	)
+
+	ctx, cancel := context.WithCancel(context.Background())
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+
+	cancel()
+
+	// Wait for watcher to stop (should happen quickly after context cancel)
+	deadline := time.After(2 * time.Second)
+	for {
+		if !w.IsRunning() {
+			break
+		}
+		select {
+		case <-deadline:
+			t.Fatal("watcher did not stop after context cancellation")
+		default:
+			time.Sleep(5 * time.Millisecond)
+		}
+	}
+}
+
+func TestWatcher_PollReloadsConfig(t *testing.T) {
+	values := map[string]string{"VALUE": "original"}
+	loader, mock := TestLoader[WatchTestConfig](values)
+	loader.Load(context.Background())
+
+	w := NewWatcher[WatchTestConfig](loader,
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+	)
+
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Update mock values
+	mock.SetValue("VALUE", "updated")
+
+	// Wait for at least one poll cycle
+	time.Sleep(50 * time.Millisecond)
+
+	w.Stop()
+
+	cfg := loader.Current()
+	if cfg.Value != "updated" {
+		t.Errorf("config value = %q, want %q", cfg.Value, "updated")
+	}
+}
+
+func TestWatcher_MaxFailures(t *testing.T) {
+	loader, mock := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	w := NewWatcher[WatchTestConfig](loader,
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+		WithMaxFailures[WatchTestConfig](2),
+	)
+
+	// Make provider fail
+	mock.SetError(fmt.Errorf("provider failure"))
+
+	ctx := context.Background()
+	w.Start(ctx)
+
+	// Wait for failures to accumulate and watcher to self-stop
+	deadline := time.After(2 * time.Second)
+	for {
+		if !w.IsRunning() {
+			break
+		}
+		select {
+		case <-deadline:
+			t.Fatal("watcher did not stop after max failures")
+		default:
+			time.Sleep(10 * time.Millisecond)
+		}
+	}
+}
+
+func TestWatch_Convenience(t *testing.T) {
+	loader, _ := TestLoader[WatchTestConfig](map[string]string{"VALUE": "x"})
+	loader.Load(context.Background())
+
+	ctx := context.Background()
+	stop := Watch[WatchTestConfig](ctx, loader,
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+	)
+
+	time.Sleep(30 * time.Millisecond)
+	stop()
+}
+
+func TestWatchWithCallback_Convenience(t *testing.T) {
+	values := map[string]string{"VALUE": "first"}
+	loader, mock := TestLoader[WatchTestConfig](values)
+	loader.Load(context.Background())
+
+	var callbackFired bool
+	stop := WatchWithCallback[WatchTestConfig](context.Background(), loader,
+		func(old, new *WatchTestConfig) {
+			callbackFired = true
+		},
+		WithWatchInterval[WatchTestConfig](10*time.Millisecond),
+	)
+
+	mock.SetValue("VALUE", "second")
+	time.Sleep(50 * time.Millisecond)
+	stop()
+
+	if !callbackFired {
+		t.Error("expected callback to fire on config change")
+	}
+}
```

---

### 4. multitenant_test.go (NEW FILE)

```diff
--- /dev/null
+++ b/multitenant_test.go
@@ -0,0 +1,274 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"testing"
+)
+
+type EnvConfig struct {
+	Region   string `doppler:"REGION" default:"us-east-1"`
+	LogLevel string `doppler:"LOG_LEVEL" default:"info"`
+}
+
+type ProjectConfig struct {
+	Name     string `doppler:"PROJECT_NAME" required:"true"`
+	MaxConns int    `doppler:"MAX_CONNS" default:"10"`
+}
+
+func TestMultiTenantLoader_LoadEnv(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"REGION":    "eu-west-1",
+		"LOG_LEVEL": "debug",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	env, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv failed: %v", err)
+	}
+
+	if env.Region != "eu-west-1" {
+		t.Errorf("Region = %q, want %q", env.Region, "eu-west-1")
+	}
+	if env.LogLevel != "debug" {
+		t.Errorf("LogLevel = %q, want %q", env.LogLevel, "debug")
+	}
+
+	// Verify Env() returns the loaded config
+	if loader.Env() == nil {
+		t.Fatal("Env() returned nil after LoadEnv")
+	}
+	if loader.Env().Region != "eu-west-1" {
+		t.Errorf("Env().Region = %q, want %q", loader.Env().Region, "eu-west-1")
+	}
+}
+
+func TestMultiTenantLoader_LoadProject(t *testing.T) {
+	mock := NewMockProvider(nil)
+	mock.SetProjectValues("", "proj-a", map[string]string{
+		"PROJECT_NAME": "Project Alpha",
+		"MAX_CONNS":    "25",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	cfg, err := loader.LoadProject(context.Background(), "proj-a")
+	if err != nil {
+		t.Fatalf("LoadProject failed: %v", err)
+	}
+
+	if cfg.Name != "Project Alpha" {
+		t.Errorf("Name = %q, want %q", cfg.Name, "Project Alpha")
+	}
+	if cfg.MaxConns != 25 {
+		t.Errorf("MaxConns = %d, want 25", cfg.MaxConns)
+	}
+
+	// Verify Project() returns the loaded config
+	cached, ok := loader.Project("proj-a")
+	if !ok {
+		t.Fatal("Project(proj-a) not found")
+	}
+	if cached.Name != "Project Alpha" {
+		t.Errorf("cached Name = %q, want %q", cached.Name, "Project Alpha")
+	}
+}
+
+func TestMultiTenantLoader_ProjectCodes(t *testing.T) {
+	mock := NewMockProvider(nil)
+	mock.SetProjectValues("", "b-proj", map[string]string{"PROJECT_NAME": "B"})
+	mock.SetProjectValues("", "a-proj", map[string]string{"PROJECT_NAME": "A"})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	loader.LoadProject(context.Background(), "b-proj")
+	loader.LoadProject(context.Background(), "a-proj")
+
+	codes := loader.ProjectCodes()
+	if len(codes) != 2 {
+		t.Fatalf("ProjectCodes length = %d, want 2", len(codes))
+	}
+	// Should be sorted
+	if codes[0] != "a-proj" || codes[1] != "b-proj" {
+		t.Errorf("ProjectCodes = %v, want [a-proj, b-proj]", codes)
+	}
+}
+
+func TestMultiTenantLoader_Projects(t *testing.T) {
+	mock := NewMockProvider(nil)
+	mock.SetProjectValues("", "proj-x", map[string]string{"PROJECT_NAME": "X"})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+	loader.LoadProject(context.Background(), "proj-x")
+
+	projects := loader.Projects()
+	if len(projects) != 1 {
+		t.Fatalf("Projects length = %d, want 1", len(projects))
+	}
+	if projects["proj-x"].Name != "X" {
+		t.Errorf("project name = %q, want %q", projects["proj-x"].Name, "X")
+	}
+}
+
+func TestMultiTenantLoader_ProjectNotFound(t *testing.T) {
+	mock := NewMockProvider(map[string]string{})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	_, ok := loader.Project("nonexistent")
+	if ok {
+		t.Error("expected Project() to return false for unloaded project")
+	}
+}
+
+func TestMultiTenantLoader_OnEnvChange(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"REGION":    "us-east-1",
+		"LOG_LEVEL": "info",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	// Load initial env
+	loader.LoadEnv(context.Background())
+
+	var oldRegion, newRegion string
+	loader.OnEnvChange(func(old, new *EnvConfig) {
+		oldRegion = old.Region
+		newRegion = new.Region
+	})
+
+	// Update and reload
+	mock.SetValues(map[string]string{
+		"REGION":    "ap-south-1",
+		"LOG_LEVEL": "warn",
+	})
+	loader.LoadEnv(context.Background())
+
+	if oldRegion != "us-east-1" {
+		t.Errorf("old region = %q, want %q", oldRegion, "us-east-1")
+	}
+	if newRegion != "ap-south-1" {
+		t.Errorf("new region = %q, want %q", newRegion, "ap-south-1")
+	}
+}
+
+func TestMultiTenantLoader_OnProjectChange(t *testing.T) {
+	mock := NewMockProvider(nil)
+	mock.SetProjectValues("", "proj-a", map[string]string{"PROJECT_NAME": "A"})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+	loader.LoadProject(context.Background(), "proj-a")
+
+	var receivedDiff *ReloadDiff
+	loader.OnProjectChange(func(diff *ReloadDiff) {
+		receivedDiff = diff
+	})
+
+	// Reload projects
+	diff, err := loader.ReloadProjects(context.Background())
+	if err != nil {
+		t.Fatalf("ReloadProjects failed: %v", err)
+	}
+
+	if len(diff.Unchanged) != 1 || diff.Unchanged[0] != "proj-a" {
+		t.Errorf("diff.Unchanged = %v, want [proj-a]", diff.Unchanged)
+	}
+
+	if receivedDiff == nil {
+		t.Fatal("OnProjectChange callback was not called")
+	}
+}
+
+func TestMultiTenantLoader_Close(t *testing.T) {
+	mock := NewMockProvider(map[string]string{})
+	fallback := NewMockProvider(map[string]string{})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, fallback)
+	err := loader.Close()
+	if err != nil {
+		t.Errorf("Close() returned error: %v", err)
+	}
+}
+
+func TestMultiTenantLoader_FetchWithFallback(t *testing.T) {
+	// Primary fails, fallback succeeds
+	primary := NewMockProviderWithError(fmt.Errorf("primary down"))
+	fallback := NewMockProvider(map[string]string{
+		"REGION":    "fallback-region",
+		"LOG_LEVEL": "warn",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](primary, fallback)
+
+	env, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv with fallback failed: %v", err)
+	}
+	if env.Region != "fallback-region" {
+		t.Errorf("Region = %q, want %q (from fallback)", env.Region, "fallback-region")
+	}
+}
+
+func TestMultiTenantLoader_NoProviders(t *testing.T) {
+	// Both providers nil: LoadEnv should fail
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](nil, nil)
+
+	_, err := loader.LoadEnv(context.Background())
+	if err == nil {
+		t.Error("expected error when no providers available")
+	}
+}
+
+func TestMultiTenantLoader_EnvBeforeLoad(t *testing.T) {
+	mock := NewMockProvider(map[string]string{})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	// Env() before LoadEnv should return nil
+	if loader.Env() != nil {
+		t.Error("Env() should be nil before LoadEnv")
+	}
+}
+
+func TestMultiTenantLoader_OnEnvChange_NoCallbackOnFirstLoad(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"REGION":    "us-east-1",
+		"LOG_LEVEL": "info",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	callbackCalled := false
+	loader.OnEnvChange(func(old, new *EnvConfig) {
+		callbackCalled = true
+	})
+
+	// First load should NOT fire callback (no old config)
+	loader.LoadEnv(context.Background())
+
+	if callbackCalled {
+		t.Error("OnEnvChange should not fire on first load")
+	}
+}
```

---

### 5. config_test.go (NEW FILE — covers config.go gaps)

```diff
--- /dev/null
+++ b/config_test.go
@@ -0,0 +1,98 @@
+package dopplerconfig
+
+import (
+	"encoding/json"
+	"testing"
+)
+
+func TestLoadBootstrapFromEnv(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "dp.st.test-token")
+	t.Setenv("DOPPLER_PROJECT", "myproject")
+	t.Setenv("DOPPLER_CONFIG", "dev")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "/tmp/fallback.json")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "true")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "fail")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.Token != "dp.st.test-token" {
+		t.Errorf("Token = %q, want %q", cfg.Token, "dp.st.test-token")
+	}
+	if cfg.Project != "myproject" {
+		t.Errorf("Project = %q, want %q", cfg.Project, "myproject")
+	}
+	if cfg.Config != "dev" {
+		t.Errorf("Config = %q, want %q", cfg.Config, "dev")
+	}
+	if cfg.FallbackPath != "/tmp/fallback.json" {
+		t.Errorf("FallbackPath = %q, want %q", cfg.FallbackPath, "/tmp/fallback.json")
+	}
+	if !cfg.WatchEnabled {
+		t.Error("WatchEnabled = false, want true")
+	}
+	if cfg.FailurePolicy != FailurePolicyFail {
+		t.Errorf("FailurePolicy = %d, want FailurePolicyFail (%d)", cfg.FailurePolicy, FailurePolicyFail)
+	}
+}
+
+func TestLoadBootstrapFromEnv_FailurePolicies(t *testing.T) {
+	tests := []struct {
+		envVal   string
+		expected FailurePolicy
+	}{
+		{"fail", FailurePolicyFail},
+		{"warn", FailurePolicyWarn},
+		{"fallback", FailurePolicyFallback},
+		{"", FailurePolicyFallback},        // default
+		{"unknown", FailurePolicyFallback}, // unrecognized -> fallback
+	}
+
+	for _, tt := range tests {
+		t.Run("policy_"+tt.envVal, func(t *testing.T) {
+			t.Setenv("DOPPLER_TOKEN", "")
+			t.Setenv("DOPPLER_FAILURE_POLICY", tt.envVal)
+			cfg := LoadBootstrapFromEnv()
+			if cfg.FailurePolicy != tt.expected {
+				t.Errorf("FailurePolicy = %d, want %d for env %q", cfg.FailurePolicy, tt.expected, tt.envVal)
+			}
+		})
+	}
+}
+
+func TestBootstrapConfig_IsEnabled(t *testing.T) {
+	cfg := BootstrapConfig{Token: "some-token"}
+	if !cfg.IsEnabled() {
+		t.Error("IsEnabled() = false, want true when token is set")
+	}
+
+	cfg2 := BootstrapConfig{}
+	if cfg2.IsEnabled() {
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
+	cfg2 := BootstrapConfig{}
+	if cfg2.HasFallback() {
+		t.Error("HasFallback() = true, want false when path is empty")
+	}
+}
+
+func TestSecretValue_EmptyString(t *testing.T) {
+	sv := SecretValue{}
+	if sv.String() != "[empty]" {
+		t.Errorf("String() = %q, want %q", sv.String(), "[empty]")
+	}
+}
+
+func TestSecretValue_MarshalJSON(t *testing.T) {
+	sv := NewSecretValue("super-secret-key")
+	data, err := json.Marshal(sv)
+	if err != nil {
+		t.Fatalf("MarshalJSON failed: %v", err)
+	}
+	if string(data) != `"[REDACTED]"` {
+		t.Errorf("MarshalJSON = %s, want %q", data, `"[REDACTED]"`)
+	}
+}
+
+func TestNewSecretValue(t *testing.T) {
+	sv := NewSecretValue("my-secret")
+	if sv.Value() != "my-secret" {
+		t.Errorf("Value() = %q, want %q", sv.Value(), "my-secret")
+	}
+}
```

---

## Risk Areas Not Covered by Tests

### 1. Concurrency Safety in MultiTenantLoader
`LoadAllProjects` uses `work.Map` with 5 bounded workers and mutates shared state under a lock. No concurrent test exists to verify this is race-free. A test with `-race` flag and parallel loads would be valuable.

### 2. DopplerProvider ETag Caching
The ETag caching logic (304 Not Modified) in `doppler.go:284-297` is only indirectly tested via `TestHealthCheck_Healthy`. A dedicated test that returns 304 and verifies cached values are returned would strengthen coverage.

### 3. Watcher Self-Stop on Max Failures
The watcher calls `go w.Stop()` (async self-stop) when max failures are reached (`watcher.go:148`). This goroutine-based self-stop pattern could theoretically race. The proposed `TestWatcher_MaxFailures` covers the happy path but a stress test would be more thorough.

### 4. FileProvider Security Validation
While `chassis_test.go` tests secval rejection of dangerous keys, there are no tests for `FileProvider` specifically handling nested JSON structures that get flattened. A maliciously crafted deeply-nested JSON file with key injection via underscores is an untested attack vector.

### 5. EnvProvider Prefix Filtering
The `hasPrefix` function uses raw byte comparison, not Unicode-aware comparison. This is fine for ASCII env var names but should be noted.

---

## Recommendations

1. **Highest Priority:** Add `feature_flags_test.go` — this is pure logic with zero external dependencies and easiest to test.
2. **High Priority:** Add `fallback_test.go` — `flattenJSON` and `WriteFallbackFile` are core functionality with no coverage.
3. **High Priority:** Add `multitenant_test.go` — the multi-tenant pattern is a primary selling point of the library.
4. **Medium Priority:** Add `watcher_test.go` — timing-dependent tests are trickier but the core Start/Stop lifecycle is testable.
5. **Medium Priority:** Add `config_test.go` — edge cases in `LoadBootstrapFromEnv` and `SecretValue`.
6. **Low Priority:** Add `-race` flag to CI pipeline for all tests to catch concurrency bugs.

---

*Report generated by Claude:Opus 4.6 — 2026-03-21T09:15:00-05:00*
