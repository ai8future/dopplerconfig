Date Created: 2026-03-21T02:55:00-07:00
TOTAL_SCORE: 68/100

# dopplerconfig Quick Analysis Report

**Agent:** Claude:Opus 4.6
**Package:** github.com/ai8future/dopplerconfig
**Files analyzed:** 14 Go files (~1,100 LOC production, ~500 LOC test)
**Dependencies:** chassis-go/v9 (call, config, errors, secval, work, testkit)

**Score Breakdown:**
- Architecture & Design: 18/20 (clean generics, good interface design, functional options)
- Security: 12/20 (unbounded reads, regex cache DoS vector, token handling)
- Test Coverage: 10/25 (no tests for watcher, multitenant, feature_flags, fallback, env provider)
- Bug-free: 14/20 (wrong version assertion, response body unbounded, reload error handling)
- Code Quality: 14/15 (minor duplication, clean overall)

---

## 1. AUDIT — Security & Code Quality Issues

### AUDIT-1: Unbounded response body read on success path (HIGH)

**File:** `doppler.go:315`
**Issue:** The error path correctly limits reads to 1KB via `io.LimitReader`, but the success path calls `io.ReadAll(resp.Body)` with no size limit. A compromised or misbehaving Doppler API could return an arbitrarily large response body, causing OOM.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,9 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config stri
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit successful response reads to 10MB to prevent OOM from malformed responses
+	const maxResponseSize = 10 * 1024 * 1024
+	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

### AUDIT-2: Unbounded regex cache is a DoS vector (MEDIUM)

**File:** `validation.go:425-441`
**Issue:** `regexCache` (sync.Map) grows without bound. If user-supplied patterns reach `validateRegex`, an attacker could exhaust memory by submitting many unique regex patterns. The cache has no eviction.

```diff
--- a/validation.go
+++ b/validation.go
@@ -423,16 +423,22 @@ func validateOneOf(value reflect.Value, param string, fieldName string) *Validat
 // regexCache caches compiled regular expressions for validation.
 var regexCache sync.Map

+// maxRegexCacheSize limits the number of cached regex patterns to prevent memory abuse.
+const maxRegexCacheSize = 100
+
+// regexCacheCount tracks the approximate number of cached patterns.
+var regexCacheCount int64
+
 // getCompiledRegex returns a cached compiled regex, or compiles and caches it.
 func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
 	if cached, ok := regexCache.Load(pattern); ok {
 		return cached.(*regexp.Regexp), nil
 	}

 	re, err := regexp.Compile(pattern)
 	if err != nil {
 		return nil, err
 	}

-	// Store in cache (may race with another goroutine, but that's fine)
-	regexCache.Store(pattern, re)
+	// Only cache if under size limit to prevent unbounded growth
+	if atomic.AddInt64(&regexCacheCount, 1) <= maxRegexCacheSize {
+		regexCache.Store(pattern, re)
+	} else {
+		atomic.AddInt64(&regexCacheCount, -1)
+	}
 	return re, nil
 }
```

### AUDIT-3: API token not zeroed on provider close (LOW)

**File:** `doppler.go:357-359`
**Issue:** `DopplerProvider.Close()` is a no-op. The bearer token remains in memory after close. In security-sensitive environments, the token field should be cleared.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -355,7 +355,11 @@ func (p *DopplerProvider) Name() string {

 // Close releases resources.
 func (p *DopplerProvider) Close() error {
+	p.mu.Lock()
+	p.token = ""
+	p.cache = nil
+	p.etag = ""
+	p.mu.Unlock()
 	return nil
 }
```

### AUDIT-4: SecretValue can be deserialized from JSON without redaction (LOW)

**File:** `config.go:166-169`
**Issue:** `SecretValue` implements `MarshalJSON` for redaction, but has no `UnmarshalJSON`. If a SecretValue struct is deserialized from JSON (e.g., in tests or logging round-trips), the raw value leaks into the field without going through `NewSecretValue`.

```diff
--- a/config.go
+++ b/config.go
@@ -168,3 +168,9 @@ func (s SecretValue) MarshalJSON() ([]byte, error) {
 	return []byte(`"[REDACTED]"`), nil
 }
+
+// UnmarshalJSON always returns an error to prevent deserializing secrets from JSON.
+// Use NewSecretValue() to create SecretValue instances programmatically.
+func (s *SecretValue) UnmarshalJSON(data []byte) error {
+	return fmt.Errorf("SecretValue cannot be deserialized from JSON; use NewSecretValue()")
+}
```

---

## 2. TESTS — Proposed Unit Tests for Untested Code

### TEST-1: Watcher tests (watcher.go — 0% coverage)

```diff
--- /dev/null
+++ b/watcher_test.go
@@ -0,0 +1,89 @@
+package dopplerconfig
+
+import (
+	"context"
+	"testing"
+	"time"
+)
+
+func TestWatcher_StartStop(t *testing.T) {
+	values := map[string]string{"DATABASE_URL": "postgres://localhost/test"}
+	loader, _ := TestLoader[TestConfig](values)
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader, WithWatchInterval[TestConfig](50*time.Millisecond))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Start failed: %v", err)
+	}
+	if !w.IsRunning() {
+		t.Error("Watcher should be running after Start")
+	}
+
+	// Double-start should be no-op
+	if err := w.Start(ctx); err != nil {
+		t.Fatalf("Double Start failed: %v", err)
+	}
+
+	w.Stop()
+	if w.IsRunning() {
+		t.Error("Watcher should not be running after Stop")
+	}
+}
+
+func TestWatcher_DetectsChanges(t *testing.T) {
+	values := map[string]string{
+		"SERVER_PORT":  "8080",
+		"DATABASE_URL": "postgres://localhost/test",
+	}
+	loader, mock := TestLoader[TestConfig](values)
+	loader.Load(context.Background())
+
+	changed := make(chan struct{}, 1)
+	loader.OnChange(func(old, new *TestConfig) {
+		select {
+		case changed <- struct{}{}:
+		default:
+		}
+	})
+
+	w := NewWatcher(loader, WithWatchInterval[TestConfig](50*time.Millisecond))
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	w.Start(ctx)
+	defer w.Stop()
+
+	mock.SetValue("SERVER_PORT", "9090")
+
+	select {
+	case <-changed:
+		// OK
+	case <-time.After(2 * time.Second):
+		t.Error("Watcher did not trigger OnChange within timeout")
+	}
+}
+
+func TestWatcher_MaxFailures(t *testing.T) {
+	values := map[string]string{"DATABASE_URL": "postgres://localhost/test"}
+	loader, mock := TestLoader[TestConfig](values)
+	loader.Load(context.Background())
+
+	w := NewWatcher(loader,
+		WithWatchInterval[TestConfig](50*time.Millisecond),
+		WithMaxFailures[TestConfig](2),
+	)
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	w.Start(ctx)
+
+	// Make the provider fail
+	mock.SetError(fmt.Errorf("simulated failure"))
+
+	// Wait for watcher to stop itself after max failures
+	time.Sleep(500 * time.Millisecond)
+	if w.IsRunning() {
+		t.Error("Watcher should have stopped after max failures")
+	}
+}
+
+func TestWatch_Convenience(t *testing.T) {
+	values := map[string]string{"DATABASE_URL": "postgres://localhost/test"}
+	loader, _ := TestLoader[TestConfig](values)
+	loader.Load(context.Background())
+
+	ctx, cancel := context.WithCancel(context.Background())
+	defer cancel()
+	stop := Watch(ctx, loader, WithWatchInterval[TestConfig](50*time.Millisecond))
+	stop()
+}
```

### TEST-2: FeatureFlags tests (feature_flags.go — 0% coverage)

```diff
--- /dev/null
+++ b/feature_flags_test.go
@@ -0,0 +1,100 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestFeatureFlags_IsEnabled(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{
+		"FEATURE_RAG_ENABLED":  "true",
+		"FEATURE_BETA_MODE":    "false",
+		"FEATURE_NEW_UI":       "1",
+		"FEATURE_OLD_FEATURE":  "",
+	}, "FEATURE_")
+
+	tests := []struct {
+		name string
+		want bool
+	}{
+		{"RAG_ENABLED", true},
+		{"BETA_MODE", false},
+		{"NEW_UI", true},
+		{"OLD_FEATURE", false},
+		{"NONEXISTENT", false},
+	}
+
+	for _, tt := range tests {
+		if got := ff.IsEnabled(tt.name); got != tt.want {
+			t.Errorf("IsEnabled(%q) = %v, want %v", tt.name, got, tt.want)
+		}
+	}
+}
+
+func TestFeatureFlags_IsDisabled(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{"FEATURE_X": "true"}, "FEATURE_")
+	if ff.IsDisabled("X") {
+		t.Error("IsDisabled('X') should be false when flag is enabled")
+	}
+}
+
+func TestFeatureFlags_GetInt(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{
+		"FEATURE_MAX_RETRIES": "5",
+		"FEATURE_BAD_INT":     "abc",
+	}, "FEATURE_")
+
+	if got := ff.GetInt("MAX_RETRIES", 3); got != 5 {
+		t.Errorf("GetInt('MAX_RETRIES') = %d, want 5", got)
+	}
+	if got := ff.GetInt("BAD_INT", 3); got != 3 {
+		t.Errorf("GetInt('BAD_INT') = %d, want default 3", got)
+	}
+	if got := ff.GetInt("MISSING", 10); got != 10 {
+		t.Errorf("GetInt('MISSING') = %d, want default 10", got)
+	}
+}
+
+func TestFeatureFlags_GetString(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{"FEATURE_MODE": "dark"}, "FEATURE_")
+	if got := ff.GetString("MODE", "light"); got != "dark" {
+		t.Errorf("GetString('MODE') = %q, want 'dark'", got)
+	}
+	if got := ff.GetString("MISSING", "light"); got != "light" {
+		t.Errorf("GetString('MISSING') = %q, want default 'light'", got)
+	}
+}
+
+func TestFeatureFlags_GetStringSlice(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{"FEATURE_REGIONS": "us,eu,ap"}, "FEATURE_")
+	got := ff.GetStringSlice("REGIONS", nil)
+	if len(got) != 3 || got[0] != "us" || got[1] != "eu" || got[2] != "ap" {
+		t.Errorf("GetStringSlice('REGIONS') = %v, want [us eu ap]", got)
+	}
+}
+
+func TestFeatureFlags_Update(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{"FEATURE_X": "true"}, "FEATURE_")
+	if !ff.IsEnabled("X") {
+		t.Fatal("X should be enabled initially")
+	}
+
+	ff.Update(map[string]string{"FEATURE_X": "false"})
+	if ff.IsEnabled("X") {
+		t.Error("X should be disabled after update")
+	}
+}
+
+func TestFeatureFlags_CaseInsensitiveLookup(t *testing.T) {
+	ff := NewFeatureFlags(map[string]string{"feature_lower": "true"}, "")
+	if !ff.IsEnabled("FEATURE_LOWER") {
+		t.Error("Case-insensitive lookup should find 'feature_lower' via 'FEATURE_LOWER'")
+	}
+}
+
+func TestRolloutConfig_ShouldEnable(t *testing.T) {
+	r := &RolloutConfig{
+		Percentage:   50,
+		AllowedUsers: []string{"vip1"},
+		BlockedUsers: []string{"blocked1"},
+	}
+	hash := func(s string) uint32 {
+		if s == "user-low" { return 10 }   // 10 % 100 = 10 < 50 → enabled
+		if s == "user-high" { return 75 }  // 75 % 100 = 75 >= 50 → disabled
+		return 0
+	}
+	if !r.ShouldEnable("vip1", hash) {
+		t.Error("AllowedUser should always be enabled")
+	}
+	if r.ShouldEnable("blocked1", hash) {
+		t.Error("BlockedUser should always be disabled")
+	}
+	if !r.ShouldEnable("user-low", hash) {
+		t.Error("user-low hash 10 should be enabled at 50%")
+	}
+	if r.ShouldEnable("user-high", hash) {
+		t.Error("user-high hash 75 should be disabled at 50%")
+	}
+}
```

### TEST-3: FileProvider and EnvProvider tests (fallback.go — 0% coverage)

```diff
--- /dev/null
+++ b/fallback_test.go
@@ -0,0 +1,95 @@
+package dopplerconfig
+
+import (
+	"context"
+	"os"
+	"testing"
+)
+
+func TestFileProvider_Fetch(t *testing.T) {
+	tmpFile := t.TempDir() + "/config.json"
+	err := os.WriteFile(tmpFile, []byte(`{"PORT": "8080", "HOST": "localhost"}`), 0600)
+	if err != nil {
+		t.Fatalf("write temp file: %v", err)
+	}
+
+	fp := NewFileProvider(tmpFile)
+	values, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if values["PORT"] != "8080" {
+		t.Errorf("PORT = %q, want '8080'", values["PORT"])
+	}
+	if values["HOST"] != "localhost" {
+		t.Errorf("HOST = %q, want 'localhost'", values["HOST"])
+	}
+}
+
+func TestFileProvider_NestedJSON(t *testing.T) {
+	tmpFile := t.TempDir() + "/nested.json"
+	err := os.WriteFile(tmpFile, []byte(`{"server": {"port": 8080, "debug": true}}`), 0600)
+	if err != nil {
+		t.Fatalf("write temp file: %v", err)
+	}
+
+	fp := NewFileProvider(tmpFile)
+	values, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if values["server_port"] != "8080" {
+		t.Errorf("server_port = %q, want '8080'", values["server_port"])
+	}
+	if values["server_debug"] != "true" {
+		t.Errorf("server_debug = %q, want 'true'", values["server_debug"])
+	}
+}
+
+func TestFileProvider_NotFound(t *testing.T) {
+	fp := NewFileProvider("/nonexistent/file.json")
+	_, err := fp.Fetch(context.Background())
+	if err == nil {
+		t.Error("Fetch should fail for nonexistent file")
+	}
+}
+
+func TestFileProvider_Name(t *testing.T) {
+	fp := NewFileProvider("/tmp/config.json")
+	if fp.Name() != "file:/tmp/config.json" {
+		t.Errorf("Name() = %q, want 'file:/tmp/config.json'", fp.Name())
+	}
+}
+
+func TestWriteFallbackFile(t *testing.T) {
+	path := t.TempDir() + "/fallback.json"
+	values := map[string]string{"KEY1": "val1", "KEY2": "val2"}
+	if err := WriteFallbackFile(path, values); err != nil {
+		t.Fatalf("WriteFallbackFile failed: %v", err)
+	}
+
+	// Read back
+	fp := NewFileProvider(path)
+	got, err := fp.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if got["KEY1"] != "val1" || got["KEY2"] != "val2" {
+		t.Errorf("Round-trip values mismatch: %v", got)
+	}
+}
+
+func TestEnvProvider_Fetch(t *testing.T) {
+	t.Setenv("TESTAPP_PORT", "3000")
+	t.Setenv("TESTAPP_HOST", "localhost")
+	t.Setenv("UNRELATED_VAR", "xyz")
+
+	ep := NewEnvProvider("TESTAPP_")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if values["TESTAPP_PORT"] != "3000" {
+		t.Errorf("TESTAPP_PORT = %q, want '3000'", values["TESTAPP_PORT"])
+	}
+	if _, ok := values["UNRELATED_VAR"]; ok {
+		t.Error("EnvProvider should filter by prefix")
+	}
+}
+
+func TestEnvProvider_NoPrefix(t *testing.T) {
+	ep := NewEnvProvider("")
+	values, err := ep.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Fetch failed: %v", err)
+	}
+	if len(values) == 0 {
+		t.Error("EnvProvider with no prefix should return all env vars")
+	}
+}
```

### TEST-4: MultiTenantLoader tests (multitenant.go — 0% coverage)

```diff
--- /dev/null
+++ b/multitenant_test.go
@@ -0,0 +1,80 @@
+package dopplerconfig
+
+import (
+	"context"
+	"testing"
+)
+
+type EnvConfig struct {
+	LogLevel string `doppler:"LOG_LEVEL" default:"info"`
+}
+
+type ProjectConfig struct {
+	Name string `doppler:"PROJECT_NAME" required:"true"`
+	Port int    `doppler:"PORT" default:"8080"`
+}
+
+func TestMultiTenantLoader_LoadEnv(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"LOG_LEVEL": "debug"})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	cfg, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatalf("LoadEnv failed: %v", err)
+	}
+	if cfg.LogLevel != "debug" {
+		t.Errorf("LogLevel = %q, want 'debug'", cfg.LogLevel)
+	}
+	if loader.Env().LogLevel != "debug" {
+		t.Errorf("Env().LogLevel = %q, want 'debug'", loader.Env().LogLevel)
+	}
+}
+
+func TestMultiTenantLoader_LoadProject(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"PROJECT_NAME": "alpha", "PORT": "9090"})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	cfg, err := loader.LoadProject(context.Background(), "alpha")
+	if err != nil {
+		t.Fatalf("LoadProject failed: %v", err)
+	}
+	if cfg.Name != "alpha" {
+		t.Errorf("Name = %q, want 'alpha'", cfg.Name)
+	}
+	if cfg.Port != 9090 {
+		t.Errorf("Port = %d, want 9090", cfg.Port)
+	}
+
+	// Should be retrievable from cache
+	cached, ok := loader.Project("alpha")
+	if !ok || cached.Name != "alpha" {
+		t.Error("Project('alpha') should return cached config")
+	}
+}
+
+func TestMultiTenantLoader_ProjectCodes(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"PROJECT_NAME": "x"})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	loader.LoadProject(context.Background(), "beta")
+	loader.LoadProject(context.Background(), "alpha")
+
+	codes := loader.ProjectCodes()
+	if len(codes) != 2 || codes[0] != "alpha" || codes[1] != "beta" {
+		t.Errorf("ProjectCodes() = %v, want [alpha beta]", codes)
+	}
+}
+
+func TestMultiTenantLoader_Close(t *testing.T) {
+	mock := NewMockProvider(map[string]string{})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+	if err := loader.Close(); err != nil {
+		t.Errorf("Close failed: %v", err)
+	}
+}
+
+func TestMultiTenantLoader_Projects(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"PROJECT_NAME": "svc"})
+	loader := NewMultiTenantLoaderWithProvider[EnvConfig, ProjectConfig](mock, nil)
+
+	loader.LoadProject(context.Background(), "svc1")
+	loader.LoadProject(context.Background(), "svc2")
+
+	projects := loader.Projects()
+	if len(projects) != 2 {
+		t.Errorf("Projects() returned %d items, want 2", len(projects))
+	}
+}
```

### TEST-5: LoadBootstrapFromEnv tests (config.go — 0% coverage)

```diff
--- /dev/null
+++ b/config_test.go
@@ -0,0 +1,45 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestLoadBootstrapFromEnv(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "dp.test.xyz")
+	t.Setenv("DOPPLER_PROJECT", "proj1")
+	t.Setenv("DOPPLER_CONFIG", "stg")
+	t.Setenv("DOPPLER_FALLBACK_PATH", "/tmp/fb.json")
+	t.Setenv("DOPPLER_WATCH_ENABLED", "true")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "warn")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.Token != "dp.test.xyz" { t.Errorf("Token = %q", cfg.Token) }
+	if cfg.Project != "proj1" { t.Errorf("Project = %q", cfg.Project) }
+	if cfg.Config != "stg" { t.Errorf("Config = %q", cfg.Config) }
+	if cfg.FallbackPath != "/tmp/fb.json" { t.Errorf("FallbackPath = %q", cfg.FallbackPath) }
+	if !cfg.WatchEnabled { t.Error("WatchEnabled should be true") }
+	if cfg.FailurePolicy != FailurePolicyWarn { t.Errorf("FailurePolicy = %d", cfg.FailurePolicy) }
+}
+
+func TestLoadBootstrapFromEnv_Defaults(t *testing.T) {
+	// Clear all DOPPLER_ vars
+	t.Setenv("DOPPLER_TOKEN", "")
+	t.Setenv("DOPPLER_FAILURE_POLICY", "")
+
+	cfg := LoadBootstrapFromEnv()
+
+	if cfg.Token != "" { t.Errorf("Token = %q, want empty", cfg.Token) }
+	if cfg.FailurePolicy != FailurePolicyFallback { t.Errorf("FailurePolicy = %d, want Fallback", cfg.FailurePolicy) }
+	if cfg.WatchEnabled { t.Error("WatchEnabled should default to false") }
+}
+
+func TestBootstrapConfig_Helpers(t *testing.T) {
+	cfg := BootstrapConfig{}
+	if cfg.IsEnabled() { t.Error("empty config should not be enabled") }
+	if cfg.HasFallback() { t.Error("empty config should not have fallback") }
+
+	cfg.Token = "tok"
+	if !cfg.IsEnabled() { t.Error("config with token should be enabled") }
+
+	cfg.FallbackPath = "/tmp/fb.json"
+	if !cfg.HasFallback() { t.Error("config with fallback path should have fallback") }
+}
```

### TEST-6: DopplerProvider Fetch with ETag caching (doppler.go — partial coverage)

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -380,0 +381,40 @@
+
+func TestDopplerProvider_FetchCachesETag(t *testing.T) {
+	callCount := 0
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		callCount++
+		if r.Header.Get("If-None-Match") == "etag-123" {
+			w.WriteHeader(http.StatusNotModified)
+			return
+		}
+		w.Header().Set("ETag", "etag-123")
+		w.Header().Set("Content-Type", "application/json")
+		w.WriteHeader(200)
+		w.Write([]byte(`{"secrets":{"KEY":{"raw":"value"}}}`))
+	}))
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
+	// First fetch: should get data + ETag
+	values, err := provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("First Fetch failed: %v", err)
+	}
+	if values["KEY"] != "value" {
+		t.Errorf("KEY = %q, want 'value'", values["KEY"])
+	}
+
+	// Second fetch: should use ETag and get 304
+	values, err = provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("Second Fetch failed: %v", err)
+	}
+	if values["KEY"] != "value" {
+		t.Errorf("Cached KEY = %q, want 'value'", values["KEY"])
+	}
+	if callCount != 2 {
+		t.Errorf("Expected 2 HTTP calls, got %d", callCount)
+	}
+}
```

---

## 3. FIXES — Bugs, Issues & Code Smells

### FIX-1: Wrong chassis version assertion in test (BUG)

**File:** `chassis_test.go:247`
**Issue:** The test asserts `ChassisVersion[0] != '7'` but the project uses `chassis-go/v9`. This test will pass when the version string starts with '7' (which it apparently does as a hardcoded value in chassis-go) but the comment says "Should be a semver starting with '7.'" which is wrong for v9.

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -243,8 +243,8 @@ func TestChassisVersion(t *testing.T) {
 	if ChassisVersion == "" {
 		t.Error("ChassisVersion should not be empty")
 	}
-	// Should be a semver starting with "7."
-	if ChassisVersion[0] != '7' {
-		t.Errorf("ChassisVersion = %q, want major version 7", ChassisVersion)
+	// ChassisVersion is set by chassis-go's internal version, just verify it's a semver
+	if len(ChassisVersion) < 3 {
+		t.Errorf("ChassisVersion = %q, expected semver format", ChassisVersion)
 	}
 }
```

### FIX-2: ReloadProjects error handling uses unreliable index correlation (BUG)

**File:** `multitenant.go:232-239`
**Issue:** After `work.Map` returns, the code correlates `results[i]` with `codes[i]` to identify failed projects. However, when `r.cfg != nil` is false, it checks `mapErr != nil` — but `mapErr` is a single aggregate error, not per-item. Also, zero-value `reloadResult{}` has an empty `code` field, so `newProjects[""]` would be set for failed items.

```diff
--- a/multitenant.go
+++ b/multitenant.go
@@ -230,14 +230,12 @@ func (l *multiTenantLoader[E, P]) ReloadProjects(ctx context.Context) (*ReloadD

 	// Collect successful reloads (work.Map returns results for all items, including failed ones).
 	newProjects := make(map[string]*P, len(results))
 	var reloadErrors []string
 	for i, r := range results {
-		if r.cfg != nil {
+		if r.code != "" && r.cfg != nil {
 			newProjects[r.code] = r.cfg
-		} else if mapErr != nil {
+		} else {
 			reloadErrors = append(reloadErrors, codes[i])
 		}
 	}
```

### FIX-3: flattenJSON does not uppercase keys (CODE SMELL)

**File:** `fallback.go:61-98`
**Issue:** The docstring says `{"server": {"port": 8080}}` becomes `{"SERVER_PORT": "8080"}` but the actual implementation preserves original case, producing `{"server_port": "8080"}`. This means nested fallback keys won't match uppercase doppler struct tags.

```diff
--- a/fallback.go
+++ b/fallback.go
@@ -58,8 +58,8 @@ func (p *FileProvider) FetchProject(ctx context.Context, project, config string)

-// flattenJSON recursively flattens a nested map into a single-level map.
-// Nested keys are joined with underscores (e.g., {"server": {"port": 8080}} -> {"SERVER_PORT": "8080"}).
+// flattenJSON recursively flattens a nested map into a single-level map with original key casing.
+// Nested keys are joined with underscores (e.g., {"server": {"port": 8080}} -> {"server_port": "8080"}).
+// Note: Keys are NOT uppercased. Fallback file keys should match your struct tag casing.
 func flattenJSON(prefix string, data map[string]interface{}, result map[string]string) {
```

### FIX-4: Empty string values silently use defaults (CODE SMELL)

**File:** `loader.go:298`
**Issue:** `if !exists || rawValue == ""` treats explicitly-set empty string values the same as missing values, falling back to defaults. This means you can't set a config value to empty string via Doppler — it always gets overridden by the default.

```diff
--- a/loader.go
+++ b/loader.go
@@ -295,7 +295,7 @@ func unmarshalStruct(values map[string]string, v reflect.Value, prefix string, w
 		rawValue, exists := values[dopplerKey]

 		// Use default if not found
-		if !exists || rawValue == "" {
+		if !exists {
 			defaultValue := field.Tag.Get(TagDefault)
 			if defaultValue != "" {
 				rawValue = defaultValue
```

### FIX-5: DopplerProvider.FetchProject error path truncation is misleading (MINOR)

**File:** `doppler.go:301-307`
**Issue:** `io.LimitReader(resp.Body, 1024)` limits to exactly 1024 bytes. Then the check `if len(rawBody) >= maxErrorBodySize` is always true when the body was exactly 1024 bytes (the max from LimitReader), causing truncation even if the body was exactly 1024 bytes. Should check if the body was actually truncated by trying to read one more byte.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -299,11 +299,12 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config stri
 	if resp.StatusCode != http.StatusOK {
 		// Limit error body read to 1KB to prevent memory issues and limit exposure
-		const maxErrorBodySize = 1024
-		limitedReader := io.LimitReader(resp.Body, maxErrorBodySize)
+		const maxErrorBodySize = 1025
+		limitedReader := io.LimitReader(resp.Body, maxErrorBodySize)
 		body, _ := io.ReadAll(limitedReader)
 		rawBody := string(body)
-		if len(rawBody) >= maxErrorBodySize {
-			rawBody = rawBody[:maxErrorBodySize-3] + "..."
+		if len(rawBody) >= maxErrorBodySize {
+			rawBody = rawBody[:1021] + "..."
 		}
```

---

## 4. REFACTOR — Opportunities to Improve Code Quality

### REFACTOR-1: Duplicated Close() error-gathering pattern

**Files:** `loader.go:220-236`, `multitenant.go:339-355`
**Issue:** Both `loader.Close()` and `multiTenantLoader.Close()` have identical patterns collecting errors from provider and fallback Close() calls. Extract to a shared helper.

### REFACTOR-2: Duplicated fallback fetch logic

**Files:** `loader.go:122-164`, `multitenant.go:357-378`
**Issue:** `loader.loadFromProvider` and `multiTenantLoader.fetchWithFallback` both implement the same primary-then-fallback pattern with slightly different error handling. The multitenant version is simpler and cleaner. Consider extracting a shared `fetchWithFallback(provider, fallback, ctx)` function that both can call.

### REFACTOR-3: Boolean parsing is implemented twice with divergent values

**Files:** `loader.go:369-379` (in `setFieldValue`), `feature_flags.go:173-185` (in `parseBool`)
**Issue:** The loader accepts "yes", "y", "on", "enabled" while `parseBool` in feature_flags accepts "yes", "on", "enabled", "enable" (no "y"). Also loader uses `strconv.ParseBool` first and falls back to custom values, while `parseBool` is entirely custom. These should use a shared implementation to avoid subtle inconsistencies.

### REFACTOR-4: MultiTenantBootstrap is an empty wrapper

**File:** `multitenant.go:78-81`
**Issue:** `MultiTenantBootstrap` embeds `BootstrapConfig` with no additional fields. It was likely intended for future extension but adds an extra type for consumers to construct. Consider accepting `BootstrapConfig` directly or documenting why the wrapper exists.

### REFACTOR-5: FeatureFlags.buildKey normalizes inconsistently

**File:** `feature_flags.go:158-171`
**Issue:** `buildKey` uppercases the name and normalizes hyphens/spaces but does NOT uppercase the prefix. If someone creates `NewFeatureFlags(values, "feature_")` (lowercase prefix), and calls `IsEnabled("X")`, the key becomes `feature_X` — mixing cases. The prefix should also be normalized.

### REFACTOR-6: EnvProvider has custom splitEnv and hasPrefix helpers

**File:** `fallback.go:157-168`
**Issue:** `splitEnv` and `hasPrefix` reimplement `strings.SplitN` and `strings.HasPrefix`. While these are marginally faster by avoiding allocations, they reduce readability for no meaningful performance gain in a function that iterates `os.Environ()`.

### REFACTOR-7: Loader callback execution under no lock protection

**Files:** `loader.go:188-193`, `multitenant.go:141-145`
**Issue:** Callbacks are copied from the slice while holding the lock, then executed outside the lock. This is correct for not holding the lock during callback execution, but there's a subtle issue: the callback slice is read into a local variable while under the write lock, but new callbacks registered concurrently (via OnChange) after the lock release won't be seen. This is an acceptable trade-off but should be documented in the interface contract.
