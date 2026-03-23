Date Created: 2026-03-21T09:05:00-07:00
TOTAL_SCORE: 77/100

# dopplerconfig Code Audit Report

**Agent:** Claude:Opus 4.6
**Codebase:** github.com/ai8future/dopplerconfig
**Version:** 1.1.6
**Go Version:** 1.25.5
**Dependency:** chassis-go v9.0.0 (local replace)

---

## Executive Summary

The dopplerconfig package is a well-structured Go library providing Doppler-backed configuration management with generics, multi-tenant support, and hot-reload. The code demonstrates strong architectural patterns (provider interface, options pattern, generic loader). However, the audit found **1 failing test** (blocking), **2 bugs**, **4 medium code smells**, and **5 minor issues**. The most critical finding is a stale test assertion left over from the chassis-go v7-to-v9 upgrade chain.

---

## Score Breakdown

| Category        | Points | Max | Notes                                                    |
|-----------------|--------|-----|----------------------------------------------------------|
| Correctness     | 20     | 30  | Failing test, ETag cache bug, empty-string-as-missing    |
| Security        | 13     | 15  | Unbounded io.ReadAll on success path                     |
| Robustness      | 13     | 15  | buildKey normalization inconsistency, doc mismatch       |
| Test Quality    | 8      | 15  | 1 failing test, no coverage for several files            |
| Code Quality    | 13     | 15  | Reinvented stdlib functions, minor doc issues            |
| Architecture    | 10     | 10  | Clean generics, proper interfaces, good patterns         |
| **TOTAL**       | **77** | **100** |                                                     |

---

## Findings

### FINDING 1: FAILING TEST - Stale Version Assertion [BUG / HIGH]

**File:** `chassis_test.go:246-249`
**Severity:** High (test suite fails)

The test `TestChassisVersion` asserts that `ChassisVersion` starts with `'7'`, but the project has been upgraded through v7 -> v8 -> v9. The actual value is `"9.0.2"`.

```
--- FAIL: TestChassisVersion (0.00s)
    chassis_test.go:248: ChassisVersion = "9.0.2", want major version 7
```

**Root Cause:** The test was written when chassis-go was at v7 and was not updated during the v7->v8->v9 upgrade chain (commits ae656c2, 4323251, 9b1eb33).

**Patch-Ready Diff:**
```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -244,8 +244,8 @@ func TestChassisVersion(t *testing.T) {
 	if ChassisVersion == "" {
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

---

### FINDING 2: ETag Cache Not Scoped to Project/Config [BUG / MEDIUM]

**File:** `doppler.go:134-136, 285-296, 340-346`
**Severity:** Medium (incorrect behavior in multi-project use)

The `DopplerProvider` has a single flat cache and single ETag, but `FetchProject` accepts arbitrary project/config parameters. If called with different project/config combos, the ETag from project A is sent when fetching project B. A `304 Not Modified` response would then return project A's cached data as if it were project B's.

**Scenario:**
1. `FetchProject(ctx, "proj-a", "dev")` -> stores results + ETag
2. `FetchProject(ctx, "proj-b", "stg")` -> sends proj-a's ETag in `If-None-Match`
3. If server returns 304, caller receives proj-a's data labeled as proj-b

In typical single-project usage via `Fetch()` this is harmless, but the `Provider` interface exposes `FetchProject` and the `multiTenantLoader` uses it.

**Patch-Ready Diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -131,8 +131,8 @@ type DopplerProvider struct {
 	breaker *call.CircuitBreaker
 	logger  *slog.Logger
 	mu      sync.RWMutex
-	cache   map[string]string
-	etag    string
+	cache   map[string]map[string]string // key: "project/config"
+	etag    map[string]string            // key: "project/config"
 }

@@ -193,6 +193,8 @@ func NewDopplerProvider(token, project, config string, opts ...DopplerProviderOpt
 		breaker: breaker,
 		logger:  slog.Default(),
+		cache:   make(map[string]map[string]string),
+		etag:    make(map[string]string),
 		client: call.New(
 			call.WithTimeout(DefaultTimeout),
 			call.WithRetry(DefaultRetryAttempts, DefaultRetryDelay),
@@ -244,6 +246,7 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 func (p *DopplerProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
 	url := fmt.Sprintf("%s/configs/config/secrets", p.apiURL)
+	cacheKey := project + "/" + config

 	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
 	if err != nil {
@@ -265,8 +268,8 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin

 	// Add ETag for caching if available
 	p.mu.RLock()
-	if p.etag != "" {
-		req.Header.Set("If-None-Match", p.etag)
+	if etag, ok := p.etag[cacheKey]; ok && etag != "" {
+		req.Header.Set("If-None-Match", etag)
 	}
 	p.mu.RUnlock()

@@ -285,7 +288,7 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 		)
 		p.mu.RLock()
-		cached := make(map[string]string, len(p.cache))
-		for k, v := range p.cache {
+		cachedData := p.cache[cacheKey]
+		cached := make(map[string]string, len(cachedData))
+		for k, v := range cachedData {
 			cached[k] = v
 		}
 		p.mu.RUnlock()
@@ -340,8 +343,8 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin

 	// Update cache with new ETag
 	p.mu.Lock()
-	p.cache = result
+	p.cache[cacheKey] = result
 	if etag := resp.Header.Get("ETag"); etag != "" {
-		p.etag = etag
+		p.etag[cacheKey] = etag
 	}
 	p.mu.Unlock()
```

---

### FINDING 3: Unbounded io.ReadAll on Success Response [SECURITY / MEDIUM]

**File:** `doppler.go:315`
**Severity:** Medium (potential OOM with malicious/misconfigured server)

The error response body is properly limited to 1KB (line 301), but the success response body at line 315 uses `io.ReadAll(resp.Body)` without any size limit. A malicious or misconfigured Doppler server could send a very large response body causing out-of-memory.

**Patch-Ready Diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,10 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit response body to 10MB to prevent OOM from malicious/misconfigured servers.
+	const maxResponseSize = 10 * 1024 * 1024
+	limitedBody := io.LimitReader(resp.Body, maxResponseSize)
+	body, err := io.ReadAll(limitedBody)
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

---

### FINDING 4: Empty String Treated as Missing Value [BUG / LOW]

**File:** `loader.go:298`
**Severity:** Low (could mask intentional empty values)

In `unmarshalStruct`, the condition `if !exists || rawValue == ""` treats an explicitly empty string value the same as a missing key. If a user intentionally sets a Doppler secret to `""` (empty string), the default value is used instead, silently overriding the user's intent.

```go
// Line 298
if !exists || rawValue == "" {
    defaultValue := field.Tag.Get(TagDefault)
    if defaultValue != "" {
        rawValue = defaultValue
        exists = true
    }
}
```

**Patch-Ready Diff:**
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

**Note:** This changes behavior for fields where a key exists with an empty value -- they will now get the empty string instead of the default. Verify this is the desired behavior for your use cases.

---

### FINDING 5: FeatureFlags.buildKey Normalization Inconsistency [CODE SMELL / MEDIUM]

**File:** `feature_flags.go:158-171`
**Severity:** Medium (surprising behavior)

When `prefix` is empty, `buildKey` returns the name as-is (no normalization). When `prefix` is set, the name is uppercased and hyphens/spaces are replaced with underscores. This means:

- `NewFeatureFlags(vals, "")` + `IsEnabled("my-flag")` looks up `"my-flag"`
- `NewFeatureFlags(vals, "FF_")` + `IsEnabled("my-flag")` looks up `"FF_MY_FLAG"`

**Patch-Ready Diff:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -156,10 +156,11 @@ func (f *FeatureFlags) Update(values map[string]string) {

 func (f *FeatureFlags) buildKey(name string) string {
+	// Always normalize the name
+	name = strings.ToUpper(name)
+	name = strings.ReplaceAll(name, "-", "_")
+	name = strings.ReplaceAll(name, " ", "_")
+
 	if f.prefix == "" {
 		return name
 	}
-	// Normalize the name
-	name = strings.ToUpper(name)
-	name = strings.ReplaceAll(name, "-", "_")
-	name = strings.ReplaceAll(name, " ", "_")

 	if strings.HasPrefix(name, strings.ToUpper(f.prefix)) {
```

---

### FINDING 6: Comment/Function Name Mismatch [CODE SMELL / LOW]

**File:** `feature_flags.go:187-189`
**Severity:** Low (documentation)

The comment says `FeatureFlagsFromLoader` but the function is named `FeatureFlagsFromValues`.

```go
// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
// This is a convenience function for extracting feature flags from a loaded config.
func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

**Patch-Ready Diff:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -187,7 +187,7 @@ func parseBool(s string) bool {
 }

-// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
-// This is a convenience function for extracting feature flags from a loaded config.
+// FeatureFlagsFromValues creates a FeatureFlags instance from a values map.
+// It uses the "FEATURE_" prefix by default.
 func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

---

### FINDING 7: Reinvented Standard Library Functions [CODE SMELL / LOW]

**File:** `fallback.go:157-168`
**Severity:** Low (unnecessary code)

`splitEnv` reimplements `strings.Cut(env, "=")` (available since Go 1.18) and `hasPrefix` reimplements `strings.HasPrefix`.

**Patch-Ready Diff:**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -1,6 +1,7 @@
 package dopplerconfig

 import (
 	"context"
 	"encoding/json"
 	"fmt"
 	"os"
 	"strings"

 	"github.com/ai8future/chassis-go/v9/secval"
@@ -146,20 +147,12 @@ func (p *EnvProvider) Fetch(ctx context.Context) (map[string]string, error) {
 func (p *EnvProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
 	result := make(map[string]string)

 	for _, env := range os.Environ() {
-		key, value := splitEnv(env)
-		if p.prefix == "" || hasPrefix(key, p.prefix) {
+		key, value, _ := strings.Cut(env, "=")
+		if p.prefix == "" || strings.HasPrefix(key, p.prefix) {
 			result[key] = value
 		}
 	}

 	return result, nil
 }

-func splitEnv(env string) (string, string) {
-	for i := 0; i < len(env); i++ {
-		if env[i] == '=' {
-			return env[:i], env[i+1:]
-		}
-	}
-	return env, ""
-}
-
-func hasPrefix(s, prefix string) bool {
-	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
-}
```

---

### FINDING 8: Missing Test Coverage [TEST QUALITY / MEDIUM]

**Severity:** Medium

The following source files have **no dedicated test files**:

| File | Lines | Coverage |
|------|-------|----------|
| `feature_flags.go` | 251 | None (FeatureFlags, RolloutConfig untested) |
| `multitenant.go` | 489 | None (entire multi-tenant system untested) |
| `watcher.go` | 179 | None (Watcher, Watch, WatchWithCallback untested) |
| `fallback.go` | 182 | Partial (FileProvider tested via chassis_test.go, EnvProvider untested) |

These are significant subsystems (feature flags, multi-tenancy, hot-reload) with no dedicated unit tests.

---

### FINDING 9: ReloadProjects May Index Incorrectly on Partial Failure [CODE SMELL / LOW]

**File:** `multitenant.go:232-239`
**Severity:** Low (depends on work.Map contract)

```go
for i, r := range results {
    if r.cfg != nil {
        newProjects[r.code] = r.cfg
    } else if mapErr != nil {
        reloadErrors = append(reloadErrors, codes[i])
    }
}
```

This assumes `work.Map` returns a results slice of the same length and order as the input `codes` slice. If `work.Map` returns partial results on error (fewer items), `codes[i]` could index incorrectly or panic. This should be validated against the `work.Map` contract.

---

### FINDING 10: fetchWithFallback Can Return nil, nil [CODE SMELL / LOW]

**File:** `multitenant.go:357-378`
**Severity:** Low (prevented by constructor)

If both `l.provider` and `l.fallback` are nil, `fetchWithFallback` returns `nil, nil` (both values and error are nil). The constructors prevent this state, but defensively, the function should return an explicit error.

**Patch-Ready Diff:**
```diff
--- a/multitenant.go
+++ b/multitenant.go
@@ -374,5 +374,8 @@ func (l *multiTenantLoader[E, P]) fetchWithFallback(ctx context.Context, project
 		}
 	}

-	return nil, err
+	if err != nil {
+		return nil, err
+	}
+	return nil, fmt.Errorf("no configuration provider available")
 }
```

---

## Summary of All Findings

| # | Severity | Type | File | Description |
|---|----------|------|------|-------------|
| 1 | HIGH | Bug | chassis_test.go:247 | Stale version assertion ("7" vs "9") - **test fails** |
| 2 | MEDIUM | Bug | doppler.go:134 | ETag cache not scoped to project/config |
| 3 | MEDIUM | Security | doppler.go:315 | Unbounded io.ReadAll on success response |
| 4 | LOW | Bug | loader.go:298 | Empty string treated as missing value |
| 5 | MEDIUM | Code Smell | feature_flags.go:158 | buildKey normalization inconsistency |
| 6 | LOW | Code Smell | feature_flags.go:187 | Comment/function name mismatch |
| 7 | LOW | Code Smell | fallback.go:157 | Reinvented stdlib functions |
| 8 | MEDIUM | Test Gap | (multiple) | No test coverage for 4 source files |
| 9 | LOW | Code Smell | multitenant.go:232 | Index assumption on work.Map results |
| 10 | LOW | Code Smell | multitenant.go:374 | fetchWithFallback can return nil, nil |

---

## Positive Observations

- Clean generic type usage throughout (`Loader[T]`, `MultiTenantLoader[E, P]`)
- Proper provider interface with multiple implementations
- Functional options pattern consistently applied
- Good mutex discipline (RLock for reads, Lock for writes, double-check locking in FeatureFlags)
- Security validation via secval on both API responses and fallback files
- SecretValue type with proper redaction in String() and MarshalJSON()
- Defensive error body limiting (1KB) on error responses
- Fallback file written with 0600 permissions
- Circuit breaker integration for resilience
- Good use of chassis-go ecosystem (call, work, secval, config, testkit)
