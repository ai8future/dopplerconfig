Date Created: 2026-03-21 10:02:00 UTC
TOTAL_SCORE: 82/100

---

# dopplerconfig Combined Analysis Report

**Agent:** Claude:Opus 4.6
**Codebase:** `github.com/ai8future/dopplerconfig` v1.1.6
**Language:** Go 1.25.5 | **Dependency:** chassis-go v9.0.0
**Files Analyzed:** 11 source files, 4 test files, configs

---

## Section 1: AUDIT - Security and Code Quality Issues

### AUDIT-1: Doppler Token Exposed in Query Parameters (LOW)

The `FetchProject` method passes project/config as query parameters over HTTPS, which is fine, but the bearer token is only in headers (correct). However, if a custom `WithAPIURL` points to an HTTP endpoint, the token would be sent in cleartext.

**File:** `doppler.go:245-263`

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -187,6 +187,10 @@ func NewDopplerProvider(token, project, config string, opts ...DopplerProviderOpt
 		return nil, fmt.Errorf("doppler token is required")
 	}

+	if token != "" && len(token) < 8 {
+		return nil, fmt.Errorf("doppler token appears invalid (too short)")
+	}
+
 	breaker := call.GetBreaker("doppler-api", DefaultBreakerThreshold, DefaultBreakerReset)
```

### AUDIT-2: No Response Body Size Limit on Success Path (MEDIUM)

On successful 200 responses, the entire response body is read without any size limit via `io.ReadAll`. A malicious or misconfigured Doppler endpoint could return an extremely large response body, causing memory exhaustion.

**File:** `doppler.go:315`

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,10 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config stri
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit response body to 10MB to prevent memory exhaustion from
+	// malicious or misconfigured endpoints.
+	const maxResponseBodySize = 10 * 1024 * 1024
+	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

### AUDIT-3: WriteFallbackFile Does Not Validate Directory Traversal (LOW)

`WriteFallbackFile` writes to any path without checking for directory traversal patterns. While callers control the path, a defense-in-depth check would be prudent.

**File:** `fallback.go:113-124`

```diff
--- a/fallback.go
+++ b/fallback.go
@@ -111,6 +111,11 @@ func WriteFallbackFile(path string, values map[string]string) error {
+	// Defense-in-depth: reject paths with directory traversal
+	if strings.Contains(path, "..") {
+		return fmt.Errorf("fallback file path must not contain '..': %s", path)
+	}
+
 	data, err := json.MarshalIndent(values, "", "  ")
 	if err != nil {
 		return fmt.Errorf("failed to marshal values: %w", err)
```

### AUDIT-4: ETag Cache Returns Stale Data Without Validation (LOW)

When a 304 Not Modified is received, cached data is returned. If the cache was populated from a previous session and the circuit breaker resets, stale secrets could be served indefinitely. The cache has no TTL.

**File:** `doppler.go:284-296`

No diff needed - this is an architectural observation. Consider adding a `cacheAge` field and rejecting cache entries older than a configurable threshold.

### AUDIT-5: FeatureFlags Case-Insensitive Lookup Iterates All Values (LOW)

When a key is not found, `IsEnabled` iterates all values with `strings.EqualFold`. For large value maps, this is O(n) per lookup on first access (cached afterward).

**File:** `feature_flags.go:53-61`

No immediate security concern, but could be a denial-of-service vector if an attacker can control the number of config keys.

---

## Section 2: TESTS - Proposed Unit Tests for Untested Code

### TEST-1: DopplerProvider ETag Caching (304 Not Modified)

**File:** `chassis_test.go` (new tests to add)

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -381,3 +381,55 @@ func TestSecvalRejectsDangerousKeys_FallbackFile(t *testing.T) {
 	}
 }
+
+func TestDopplerProvider_ETagCaching(t *testing.T) {
+	callCount := 0
+	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+		callCount++
+		if etag := r.Header.Get("If-None-Match"); etag == `"test-etag"` {
+			w.WriteHeader(http.StatusNotModified)
+			return
+		}
+		w.Header().Set("ETag", `"test-etag"`)
+		w.WriteHeader(http.StatusOK)
+		w.Write([]byte(`{"secrets":{"DB_HOST":{"raw":"localhost"}}}`))
+	}))
+	defer srv.Close()
+
+	provider, err := NewDopplerProvider("test-token", "proj", "dev",
+		WithAPIURL(srv.URL),
+		WithHTTPClient(srv.Client()),
+	)
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	// First call: should get full response
+	vals, err := provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("first fetch: %v", err)
+	}
+	if vals["DB_HOST"] != "localhost" {
+		t.Errorf("DB_HOST = %q, want %q", vals["DB_HOST"], "localhost")
+	}
+
+	// Second call: should get 304 and return cached data
+	vals, err = provider.Fetch(context.Background())
+	if err != nil {
+		t.Fatalf("second fetch: %v", err)
+	}
+	if vals["DB_HOST"] != "localhost" {
+		t.Errorf("cached DB_HOST = %q, want %q", vals["DB_HOST"], "localhost")
+	}
+	if callCount != 2 {
+		t.Errorf("callCount = %d, want 2", callCount)
+	}
+}
```

### TEST-2: DopplerProvider Error Status Codes (401, 403, 404, 429, 5xx)

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -381,3 +381,62 @@ func TestSecvalRejectsDangerousKeys_FallbackFile(t *testing.T) {
 	}
 }
+
+func TestDopplerProvider_ErrorStatusCodes(t *testing.T) {
+	tests := []struct {
+		name       string
+		statusCode int
+		body       string
+		wantMsg    string
+	}{
+		{"unauthorized", 401, "invalid token", "API returned status 401"},
+		{"forbidden", 403, "access denied", "API returned status 403"},
+		{"not_found", 404, "project not found", "API returned status 404"},
+		{"rate_limited", 429, "too many requests", "API returned status 429"},
+		{"server_error", 500, "internal error", "API returned status 500"},
+		{"bad_gateway", 502, "bad gateway", "API returned status 502"},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
+				w.WriteHeader(tt.statusCode)
+				w.Write([]byte(tt.body))
+			}))
+			defer srv.Close()
+
+			provider, err := NewDopplerProvider("test-token", "proj", "dev",
+				WithAPIURL(srv.URL),
+				WithHTTPClient(srv.Client()),
+			)
+			if err != nil {
+				t.Fatal(err)
+			}
+
+			_, err = provider.Fetch(context.Background())
+			if err == nil {
+				t.Fatal("expected error, got nil")
+			}
+
+			de, ok := IsDopplerError(err)
+			if !ok {
+				t.Fatalf("expected DopplerError, got %T: %v", err, err)
+			}
+			if de.StatusCode != tt.statusCode {
+				t.Errorf("StatusCode = %d, want %d", de.StatusCode, tt.statusCode)
+			}
+		})
+	}
+}
```

### TEST-3: Watcher MaxFailures Behavior

```diff
--- /dev/null
+++ b/watcher_test.go
@@ -0,0 +1,56 @@
+package dopplerconfig
+
+import (
+	"context"
+	"fmt"
+	"testing"
+	"time"
+)
+
+func TestWatcher_MaxFailures(t *testing.T) {
+	mock := NewMockProviderWithError(fmt.Errorf("fetch error"))
+	loader := NewLoaderWithProvider[struct{}](mock, nil)
+
+	w := NewWatcher(loader,
+		WithWatchInterval[struct{}](50*time.Millisecond),
+		WithMaxFailures[struct{}](3),
+	)
+
+	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
+	defer cancel()
+
+	if err := w.Start(ctx); err != nil {
+		t.Fatal(err)
+	}
+
+	// Wait for watcher to stop itself after max failures
+	deadline := time.After(1 * time.Second)
+	ticker := time.NewTicker(50 * time.Millisecond)
+	defer ticker.Stop()
+
+	for {
+		select {
+		case <-deadline:
+			t.Fatal("watcher did not stop after max failures")
+		case <-ticker.C:
+			if !w.IsRunning() {
+				return // Success
+			}
+		}
+	}
+}
+
+func TestWatcher_ContextCancel(t *testing.T) {
+	mock := NewMockProvider(map[string]string{"KEY": "val"})
+	loader := NewLoaderWithProvider[struct{}](mock, nil)
+
+	w := NewWatcher(loader, WithWatchInterval[struct{}](50*time.Millisecond))
+
+	ctx, cancel := context.WithCancel(context.Background())
+	w.Start(ctx)
+
+	if !w.IsRunning() {
+		t.Fatal("watcher should be running")
+	}
+
+	cancel()
+	time.Sleep(200 * time.Millisecond)
+
+	if w.IsRunning() {
+		t.Fatal("watcher should have stopped after context cancel")
+	}
+}
```

### TEST-4: FileProvider Error Handling

```diff
--- /dev/null
+++ b/fallback_test.go
@@ -0,0 +1,64 @@
+package dopplerconfig
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestFileProvider_NotFound(t *testing.T) {
+	p := NewFileProvider("/nonexistent/path/config.json")
+	_, err := p.Fetch(context.Background())
+	if err == nil {
+		t.Fatal("expected error for missing file")
+	}
+}
+
+func TestFileProvider_InvalidJSON(t *testing.T) {
+	tmp := filepath.Join(t.TempDir(), "bad.json")
+	os.WriteFile(tmp, []byte("not json"), 0600)
+
+	p := NewFileProvider(tmp)
+	_, err := p.Fetch(context.Background())
+	if err == nil {
+		t.Fatal("expected error for invalid JSON")
+	}
+}
+
+func TestFileProvider_NestedJSON(t *testing.T) {
+	tmp := filepath.Join(t.TempDir(), "nested.json")
+	os.WriteFile(tmp, []byte(`{"server":{"port":8080,"host":"localhost"},"debug":true}`), 0600)
+
+	p := NewFileProvider(tmp)
+	vals, err := p.Fetch(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	if vals["server_port"] != "8080" {
+		t.Errorf("server_port = %q, want %q", vals["server_port"], "8080")
+	}
+	if vals["server_host"] != "localhost" {
+		t.Errorf("server_host = %q, want %q", vals["server_host"], "localhost")
+	}
+	if vals["debug"] != "true" {
+		t.Errorf("debug = %q, want %q", vals["debug"], "true")
+	}
+}
+
+func TestWriteFallbackFile_RoundTrip(t *testing.T) {
+	tmp := filepath.Join(t.TempDir(), "fallback.json")
+	values := map[string]string{"KEY": "value", "PORT": "8080"}
+
+	if err := WriteFallbackFile(tmp, values); err != nil {
+		t.Fatal(err)
+	}
+
+	p := NewFileProvider(tmp)
+	got, err := p.Fetch(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+
+	if got["KEY"] != "value" {
+		t.Errorf("KEY = %q, want %q", got["KEY"], "value")
+	}
+}
+
+func TestEnvProvider_Name(t *testing.T) {
+	p := NewEnvProvider("APP_")
+	if p.Name() != "env:APP_*" {
+		t.Errorf("Name() = %q, want %q", p.Name(), "env:APP_*")
+	}
+}
```

### TEST-5: FeatureFlags Edge Cases

```diff
--- /dev/null
+++ b/feature_flags_test.go
@@ -0,0 +1,80 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestFeatureFlags_IsEnabled(t *testing.T) {
+	flags := NewFeatureFlags(map[string]string{
+		"FEATURE_RAG_ENABLED": "true",
+		"FEATURE_BETA":       "false",
+		"FEATURE_LEGACY":     "1",
+		"FEATURE_NEW":        "yes",
+	}, "FEATURE_")
+
+	tests := []struct {
+		name string
+		want bool
+	}{
+		{"RAG_ENABLED", true},
+		{"BETA", false},
+		{"LEGACY", true},
+		{"NEW", true},
+		{"NONEXISTENT", false},
+	}
+
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if got := flags.IsEnabled(tt.name); got != tt.want {
+				t.Errorf("IsEnabled(%q) = %v, want %v", tt.name, got, tt.want)
+			}
+		})
+	}
+}
+
+func TestFeatureFlags_Update(t *testing.T) {
+	flags := NewFeatureFlags(map[string]string{
+		"FEATURE_A": "true",
+	}, "FEATURE_")
+
+	if !flags.IsEnabled("A") {
+		t.Error("A should be enabled initially")
+	}
+
+	flags.Update(map[string]string{
+		"FEATURE_A": "false",
+		"FEATURE_B": "true",
+	})
+
+	if flags.IsEnabled("A") {
+		t.Error("A should be disabled after update")
+	}
+	if !flags.IsEnabled("B") {
+		t.Error("B should be enabled after update")
+	}
+}
+
+func TestRolloutConfig_ShouldEnable(t *testing.T) {
+	rc := &RolloutConfig{
+		Percentage:   50,
+		AllowedUsers: []string{"admin"},
+		BlockedUsers: []string{"banned"},
+	}
+	hashFunc := func(s string) uint32 {
+		var h uint32
+		for _, c := range s {
+			h = h*31 + uint32(c)
+		}
+		return h
+	}
+
+	// Allowed user always gets feature
+	if !rc.ShouldEnable("admin", hashFunc) {
+		t.Error("admin should always be enabled")
+	}
+
+	// Blocked user never gets feature
+	if rc.ShouldEnable("banned", hashFunc) {
+		t.Error("banned should always be disabled")
+	}
+
+	// 0% rollout
+	rc.Percentage = 0
+	if rc.ShouldEnable("regularuser", hashFunc) {
+		t.Error("0% rollout should disable all non-allowed users")
+	}
+
+	// 100% rollout
+	rc.Percentage = 100
+	if !rc.ShouldEnable("regularuser", hashFunc) {
+		t.Error("100% rollout should enable all non-blocked users")
+	}
+}
```

### TEST-6: MultiTenantLoader Parallel Loading

```diff
--- /dev/null
+++ b/multitenant_test.go
@@ -0,0 +1,62 @@
+package dopplerconfig
+
+import (
+	"context"
+	"testing"
+)
+
+type testProjectConfig struct {
+	Name string `doppler:"PROJECT_NAME"`
+	Port int    `doppler:"PORT" default:"8080"`
+}
+
+type testEnvConfig struct {
+	Region string `doppler:"REGION" default:"us-east-1"`
+}
+
+func TestMultiTenantLoader_LoadAllProjects(t *testing.T) {
+	mock := NewMockProvider(map[string]string{
+		"PROJECT_NAME": "default",
+		"PORT":         "9090",
+		"REGION":       "us-west-2",
+	})
+
+	loader := NewMultiTenantLoaderWithProvider[testEnvConfig, testProjectConfig](mock, nil)
+
+	// Load env
+	env, err := loader.LoadEnv(context.Background())
+	if err != nil {
+		t.Fatal(err)
+	}
+	if env.Region != "us-west-2" {
+		t.Errorf("Region = %q, want %q", env.Region, "us-west-2")
+	}
+
+	// Load projects
+	projects, err := loader.LoadAllProjects(context.Background(), []string{"proj-a", "proj-b"})
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(projects) != 2 {
+		t.Errorf("got %d projects, want 2", len(projects))
+	}
+
+	// Check project codes
+	codes := loader.ProjectCodes()
+	if len(codes) != 2 {
+		t.Errorf("got %d codes, want 2", len(codes))
+	}
+}
+
+func TestMultiTenantLoader_Project_NotLoaded(t *testing.T) {
+	mock := NewMockProvider(map[string]string{})
+	loader := NewMultiTenantLoaderWithProvider[testEnvConfig, testProjectConfig](mock, nil)
+
+	_, ok := loader.Project("nonexistent")
+	if ok {
+		t.Error("expected false for unloaded project")
+	}
+}
```

### TEST-7: LoadBootstrapFromEnv

```diff
--- /dev/null
+++ b/config_test.go
@@ -0,0 +1,49 @@
+package dopplerconfig
+
+import (
+	"testing"
+)
+
+func TestLoadBootstrapFromEnv(t *testing.T) {
+	t.Setenv("DOPPLER_TOKEN", "dp.st.test-token")
+	t.Setenv("DOPPLER_PROJECT", "my-project")
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
+	if cfg.Project != "my-project" {
+		t.Errorf("Project = %q, want %q", cfg.Project, "my-project")
+	}
+	if !cfg.WatchEnabled {
+		t.Error("WatchEnabled should be true")
+	}
+	if cfg.FailurePolicy != FailurePolicyFail {
+		t.Errorf("FailurePolicy = %d, want %d", cfg.FailurePolicy, FailurePolicyFail)
+	}
+	if !cfg.IsEnabled() {
+		t.Error("IsEnabled should be true")
+	}
+	if !cfg.HasFallback() {
+		t.Error("HasFallback should be true")
+	}
+}
+
+func TestSecretValue(t *testing.T) {
+	sv := NewSecretValue("my-secret")
+	if sv.Value() != "my-secret" {
+		t.Errorf("Value() = %q, want %q", sv.Value(), "my-secret")
+	}
+	if sv.String() != "[REDACTED]" {
+		t.Errorf("String() = %q, want %q", sv.String(), "[REDACTED]")
+	}
+
+	empty := NewSecretValue("")
+	if empty.String() != "[empty]" {
+		t.Errorf("empty String() = %q, want %q", empty.String(), "[empty]")
+	}
+
+	data, _ := sv.MarshalJSON()
+	if string(data) != `"[REDACTED]"` {
+		t.Errorf("MarshalJSON() = %q, want %q", string(data), `"[REDACTED]"`)
+	}
+}
```

---

## Section 3: FIXES - Bugs, Issues, and Code Smells

### FIX-1: Test Expects Wrong Chassis Version (BUG)

`chassis_test.go:247` expects ChassisVersion to start with `'7'` but chassis-go was upgraded to v9 in the latest release.

**File:** `chassis_test.go:242-250`

```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -244,8 +244,8 @@ func TestChassisVersion(t *testing.T) {
 		t.Error("ChassisVersion should not be empty")
 	}
-	// Should be a semver starting with "7."
-	if ChassisVersion[0] != '7' {
-		t.Errorf("ChassisVersion = %q, want major version 7", ChassisVersion)
+	// Should be a semver starting with "9."
+	if ChassisVersion[0] != '9' {
+		t.Errorf("ChassisVersion = %q, want major version 9", ChassisVersion)
 	}
 }
```

### FIX-2: Regex Cache Should Use LoadOrStore for Thread Safety (CODE SMELL)

`getCompiledRegex` has a benign race where two goroutines may compile and store the same pattern. While functionally correct, `LoadOrStore` is the idiomatic Go approach.

**File:** `validation.go:428-441`

```diff
--- a/validation.go
+++ b/validation.go
@@ -427,14 +427,11 @@ var regexCache sync.Map
 // getCompiledRegex returns a cached compiled regex, or compiles and caches it.
 func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
-	if cached, ok := regexCache.Load(pattern); ok {
-		return cached.(*regexp.Regexp), nil
-	}
-
 	re, err := regexp.Compile(pattern)
 	if err != nil {
+		if cached, ok := regexCache.Load(pattern); ok {
+			return cached.(*regexp.Regexp), nil
+		}
 		return nil, err
 	}

-	// Store in cache (may race with another goroutine, but that's fine)
-	regexCache.Store(pattern, re)
-	return re, nil
+	actual, _ := regexCache.LoadOrStore(pattern, re)
+	return actual.(*regexp.Regexp), nil
 }
```

### FIX-3: Loader FailurePolicy Default Falls Through to Error (CODE SMELL)

In `loader.go:150-163`, the `default` case in the FailurePolicy switch returns an error even when `FailurePolicyFallback` is the policy. This is correct behavior (fallback already failed), but the error message could be clearer.

**File:** `loader.go:150-163`

```diff
--- a/loader.go
+++ b/loader.go
@@ -156,7 +156,7 @@ func (l *loader[T]) loadFromProvider(ctx context.Context, isReload bool) (*T, er
 		default:
 			if err != nil {
-				return nil, fmt.Errorf("failed to load configuration: %w", err)
+				return nil, fmt.Errorf("failed to load configuration (policy: fallback, all sources exhausted): %w", err)
 			}
 			return nil, fmt.Errorf("no configuration available")
 		}
```

### FIX-4: MultiTenantLoader fetchWithFallback Returns nil Error When Both Providers Are nil (BUG)

If both `provider` and `fallback` are nil (which shouldn't happen via normal construction but could via `NewMultiTenantLoaderWithProvider`), `fetchWithFallback` returns `nil, nil` — a nil error with nil values, which could cause a nil pointer dereference in `unmarshalConfig`.

**File:** `multitenant.go:357-378`

```diff
--- a/multitenant.go
+++ b/multitenant.go
@@ -375,7 +375,10 @@ func (l *multiTenantLoader[E, P]) fetchWithFallback(ctx context.Context, project
 		}
 	}

-	return nil, err
+	if err != nil {
+		return nil, err
+	}
+	return nil, fmt.Errorf("no configuration source available")
 }
```

### FIX-5: Watcher.Stop() Called From Within poll() Can Deadlock (BUG - EDGE CASE)

In `watcher.go:147`, when max failures are reached, `go w.Stop()` is called. `Stop()` closes `stopCh` and waits on `doneCh`. Since `poll()` is called from within `run()`, and `run()` closes `doneCh` on exit, this works correctly due to the `go` keyword launching it as a goroutine. However, if `Stop()` were ever called synchronously from within `poll()`, it would deadlock. The current code is correct but fragile.

**File:** `watcher.go:143-148`

```diff
--- a/watcher.go
+++ b/watcher.go
@@ -143,6 +143,8 @@ func (w *Watcher[T]) poll(ctx context.Context) {
 		if maxFail > 0 && failures >= maxFail {
 			w.logger.Error("max failures reached, stopping watcher",
 				"max_failures", maxFail,
 			)
-			go w.Stop()
+			// Must be async: Stop() waits on doneCh which is closed by run(),
+			// and we're called from within run(). Synchronous call would deadlock.
+			go w.Stop() // intentionally async - do not change to synchronous
 		}
```

---

## Section 4: REFACTOR - Opportunities to Improve Code Quality

### REFACTOR-1: Extract Boolean Parsing Into Shared Utility

Both `loader.go:370-377` (in `setFieldValue`) and `feature_flags.go:173-185` (`parseBool`) implement boolean parsing with slightly different accepted values. The loader accepts "y", "n", "enabled", "disabled" while `parseBool` accepts "enable" but not "y" or "n". These should be consolidated into a single `parseBool` function with consistent behavior.

**Files:** `loader.go`, `feature_flags.go`

### REFACTOR-2: Consider errors.Join for Multi-Error Aggregation

`loader.go:232-234` and `multitenant.go:351-353` both use `fmt.Errorf("close errors: %v", errs)` to aggregate errors. Go 1.20+ provides `errors.Join()` which preserves the error chain for `errors.Is`/`errors.As` unwrapping.

**Files:** `loader.go:220-236`, `multitenant.go:339-355`

### REFACTOR-3: Add Stringer Interface to FailurePolicy

`FailurePolicy` is an `int` enum but has no `String()` method. Adding one would improve debug logging and error messages.

**File:** `config.go`

### REFACTOR-4: MultiTenantWatcher Logger Injection

`MultiTenantWatcher` has a `WithLogger` method that returns `*MultiTenantWatcher` for chaining, while the single-tenant `Watcher` uses the option pattern (`WithWatchLogger`). These should use a consistent API style.

**Files:** `watcher.go`, `multitenant.go:424-428`

### REFACTOR-5: Provider Interface Could Include Context in Close

`Provider.Close()` does not accept a `context.Context`, which means cleanup operations cannot be cancelled or timed out. This is fine for current implementations (all are no-ops) but could be limiting for future providers that hold HTTP connections or other resources.

**File:** `doppler.go:118-119`

### REFACTOR-6: Consolidate Watcher Patterns

`Watcher[T]` and `MultiTenantWatcher[E, P]` share nearly identical start/stop/run patterns. Consider extracting the lifecycle management into a shared base, reducing ~60 lines of duplicated goroutine management code.

**Files:** `watcher.go`, `multitenant.go:403-488`

### REFACTOR-7: Use Typed Constants for Default Prefix

`FeatureFlagsFromValues` hardcodes `"FEATURE_"` as the default prefix. This should reference a package-level constant for consistency with the rest of the codebase's constant usage pattern.

**File:** `feature_flags.go:189-191`

---

## Score Breakdown

| Category | Points | Max | Notes |
|----------|--------|-----|-------|
| Code Quality | 17 | 20 | Excellent patterns, minor inconsistencies |
| Security | 15 | 20 | Strong posture; missing response body limit on success path |
| Test Coverage | 12 | 20 | Good existing tests but significant gaps in error paths, watcher, fallback, feature flags |
| Error Handling | 16 | 15 | Exceeds expectations - consistent wrapping, custom types, proper chains |
| Architecture | 14 | 15 | Clean layered design, proper interfaces, good separation of concerns |
| Documentation | 8 | 10 | Excellent godoc, good README, minor stale comments |
| **TOTAL** | **82** | **100** | |

**Deductions:**
- -3 for stale test assertion (FIX-1: version '7' should be '9')
- -5 for missing response body size limit on success path (AUDIT-2)
- -8 for test coverage gaps (no tests for watcher, fallback, feature_flags, env bootstrap, ETag caching)
- -2 for inconsistent boolean parsing between loader and feature_flags
