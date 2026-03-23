Date Created: 2026-03-21T08:57:00-07:00
TOTAL_SCORE: 72/100

# dopplerconfig Audit Report

**Auditor:** Claude:Opus 4.6 via Claude Code
**Codebase:** github.com/ai8future/dopplerconfig
**Go Version:** 1.25.5 | **Dependency:** chassis-go/v9
**Files Reviewed:** 12 source files, 4 test files, go.mod, .gitignore

---

## Executive Summary

dopplerconfig is a well-architected Go configuration library integrating with the Doppler secrets manager and chassis-go v9. The codebase demonstrates strong design patterns (generics, options pattern, Provider interface) and includes several security-conscious choices (secret redaction, JSON security validation via secval, bounded error bodies). However, the audit identified **3 security concerns**, **5 bugs/correctness issues**, and **several code quality items** that collectively bring the score to 72/100.

---

## Scoring Breakdown

| Category | Score | Max | Notes |
|---|---|---|---|
| Security | 18 | 25 | Unbounded success body, no TLS enforcement, token in plaintext memory |
| Correctness / Bugs | 15 | 25 | Empty-string-as-missing, stale test, flattenJSON comment lie, EnvProvider prefix |
| Code Quality | 16 | 20 | Committed artifacts, reimplemented stdlib, mismatched doc comment |
| Test Coverage | 10 | 15 | No tests for FeatureFlags, MultiTenantLoader, Watcher, EnvProvider |
| Architecture / Design | 13 | 15 | Clean generics, good separation, minor watcher coupling |
| **TOTAL** | **72** | **100** | |

---

## SECURITY FINDINGS

### SEC-1: Unbounded HTTP Response Body Read (HIGH)

**File:** `doppler.go:315`
**Severity:** High
**Impact:** Denial of service via memory exhaustion

The error response body is correctly limited to 1KB (line 301), but the success path reads the entire response body without any limit. A compromised or malicious Doppler API endpoint (or MITM attacker) could serve a multi-gigabyte response, causing OOM.

```go
// LINE 315 - CURRENT (unbounded)
body, err := io.ReadAll(resp.Body)

// Should be bounded like the error path
```

**Patch-ready diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -78,6 +78,9 @@ const (
 	// DefaultBreakerReset is how long the circuit stays open before allowing a probe.
 	DefaultBreakerReset = 30 * time.Second
+
+	// MaxResponseBodySize is the maximum size of a successful Doppler API response (10MB).
+	MaxResponseBodySize = 10 * 1024 * 1024
 )

@@ -312,7 +315,12 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	limitedBody := io.LimitReader(resp.Body, MaxResponseBodySize+1)
+	body, err := io.ReadAll(limitedBody)
+	if err == nil && int64(len(body)) > MaxResponseBodySize {
+		return nil, fmt.Errorf("doppler response exceeds maximum size (%d bytes)", MaxResponseBodySize)
+	}
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

---

### SEC-2: No TLS Enforcement for API URL or Custom Clients (MEDIUM)

**File:** `doppler.go:142-158`
**Severity:** Medium
**Impact:** Bearer token sent over plaintext HTTP

`WithAPIURL` accepts any URL including `http://`, and `WithHTTPClient` accepts any `*http.Client` without TLS verification. The Doppler bearer token would be transmitted in cleartext.

**Patch-ready diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -140,6 +140,9 @@ type DopplerProviderOption func(*DopplerProvider)
 // WithAPIURL sets a custom Doppler API URL.
 func WithAPIURL(url string) DopplerProviderOption {
 	return func(p *DopplerProvider) {
+		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://localhost") && !strings.HasPrefix(url, "http://127.0.0.1") {
+			p.logger.Warn("doppler API URL is not HTTPS; bearer token may be sent in cleartext", "url", url)
+		}
 		p.apiURL = url
 	}
 }
```

*(Note: a hard block would break test servers; a warning is the pragmatic choice.)*

---

### SEC-3: Regex Cache Unbounded Growth (LOW)

**File:** `validation.go:425-441`
**Severity:** Low
**Impact:** Memory leak if regex patterns come from dynamic sources

`regexCache` is a `sync.Map` that grows without bound. While patterns currently come from struct tags (static), if the validation system is ever extended to accept runtime patterns, this becomes a memory leak.

**Patch-ready diff:**
```diff
--- a/validation.go
+++ b/validation.go
@@ -423,7 +423,15 @@ func validateOneOf(value reflect.Value, param string, fieldName string) *Validat

 // regexCache caches compiled regular expressions for validation.
-var regexCache sync.Map
+// Limited to 1000 entries to prevent unbounded growth.
+var (
+	regexCache      sync.Map
+	regexCacheCount int64
+	maxRegexCache   int64 = 1000
+)

 // getCompiledRegex returns a cached compiled regex, or compiles and caches it.
 func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
@@ -434,8 +442,14 @@ func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
 	if err != nil {
 		return nil, err
 	}

-	// Store in cache (may race with another goroutine, but that's fine)
-	regexCache.Store(pattern, re)
+	// Store in cache if under limit
+	if atomic.LoadInt64(&regexCacheCount) < maxRegexCache {
+		if _, loaded := regexCache.LoadOrStore(pattern, re); !loaded {
+			atomic.AddInt64(&regexCacheCount, 1)
+		}
+	}
 	return re, nil
 }
```

---

## BUG / CORRECTNESS FINDINGS

### BUG-1: Empty String Treated as Missing Value (HIGH)

**File:** `loader.go:298`
**Severity:** High
**Impact:** Explicitly empty Doppler values are silently replaced by defaults

```go
// LINE 298
if !exists || rawValue == "" {
    defaultValue := field.Tag.Get(TagDefault)
```

If a user explicitly sets a Doppler secret to an empty string (`KEY=""`), the loader treats it as "not found" and applies the default value. This is semantically incorrect -- an empty string is a valid value distinct from "not set".

**Patch-ready diff:**
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
@@ -305,7 +305,7 @@ func unmarshalStruct(values map[string]string, v reflect.Value, prefix string, w

 		// Check required
-		if field.Tag.Get(TagRequired) == "true" && !exists {
+		if field.Tag.Get(TagRequired) == "true" && (!exists || rawValue == "") {
 			return *warnings, fmt.Errorf("required field %s (key: %s) not found", field.Name, dopplerKey)
 		}

 		// Skip if no value
-		if !exists || rawValue == "" {
+		if !exists {
 			continue
 		}
```

---

### BUG-2: Stale Chassis Version Test Assertion (MEDIUM)

**File:** `chassis_test.go:247-249`
**Severity:** Medium (test failure in CI)
**Impact:** Tests fail -- `go test` returns FAIL

The test asserts the chassis version starts with `'7'` but the module imports chassis-go v9. This test was not updated during the v7->v8->v9 upgrade chain.

```go
// LINE 247-249 - CURRENT (FAILS)
if ChassisVersion[0] != '7' {
    t.Errorf("ChassisVersion = %q, want major version 7", ChassisVersion)
}
```

**Confirmed failure:**
```
--- FAIL: TestChassisVersion (0.00s)
    chassis_test.go:248: ChassisVersion = "9.0.2", want major version 7
```

**Patch-ready diff:**
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

### BUG-3: flattenJSON Comment Mismatches Behavior (MEDIUM)

**File:** `fallback.go:60-61`
**Severity:** Medium
**Impact:** Misleading documentation; nested JSON keys won't match UPPER_CASE Doppler keys

The doc comment claims: `{"server": {"port": 8080}} -> {"SERVER_PORT": "8080"}`, but the code performs no uppercasing. The actual output would be `{"server_port": "8080"}`, which won't match typical Doppler key patterns like `SERVER_PORT`.

```go
// LINE 60-61 - Comment claims uppercasing that doesn't happen
// flattenJSON recursively flattens a nested map into a single-level map.
// Nested keys are joined with underscores (e.g., {"server": {"port": 8080}} -> {"SERVER_PORT": "8080"}).
```

**Patch-ready diff (fix comment to match behavior):**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -59,7 +59,8 @@ func (p *FileProvider) FetchProject(ctx context.Context, project, config string)

 // flattenJSON recursively flattens a nested map into a single-level map.
-// Nested keys are joined with underscores (e.g., {"server": {"port": 8080}} -> {"SERVER_PORT": "8080"}).
+// Nested keys are joined with underscores, preserving original casing.
+// Example: {"server": {"port": 8080}} -> {"server_port": "8080"}.
 func flattenJSON(prefix string, data map[string]interface{}, result map[string]string) {
```

**Alternative patch (make behavior match comment -- uppercase keys):**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -61,7 +61,8 @@ func (p *FileProvider) FetchProject(ctx context.Context, project, config string)
 func flattenJSON(prefix string, data map[string]interface{}, result map[string]string) {
 	for key, value := range data {
-		fullKey := key
+		fullKey := strings.ToUpper(key)
 		if prefix != "" {
-			fullKey = prefix + "_" + key
+			fullKey = strings.ToUpper(prefix) + "_" + strings.ToUpper(key)
 		}
```

---

### BUG-4: EnvProvider Doesn't Strip Prefix From Keys (LOW)

**File:** `fallback.go:144-155`
**Severity:** Low
**Impact:** When prefix is set, returned map keys include the prefix, which may not match struct tag expectations

If `prefix="APP_"`, the key `APP_PORT` is returned as-is. If the struct tag says `doppler:"PORT"`, it won't match. Users likely expect the prefix to be stripped.

**Patch-ready diff:**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -146,7 +146,11 @@ func (p *EnvProvider) FetchProject(ctx context.Context, project, config string)
 	for _, env := range os.Environ() {
 		key, value := splitEnv(env)
 		if p.prefix == "" || hasPrefix(key, p.prefix) {
-			result[key] = value
+			if p.prefix != "" {
+				result[key[len(p.prefix):]] = value
+			} else {
+				result[key] = value
+			}
 		}
 	}
```

---

### BUG-5: ReloadProjects Assumes work.Map Preserves Order (LOW)

**File:** `multitenant.go:232-239`
**Severity:** Low
**Impact:** Error tracking may attribute failures to wrong project codes if work.Map reorders results

```go
// LINE 234-239
for i, r := range results {
    if r.cfg != nil {
        newProjects[r.code] = r.cfg
    } else if mapErr != nil {
        reloadErrors = append(reloadErrors, codes[i]) // Assumes ordering
    }
}
```

The code uses `codes[i]` to identify which project failed, assuming `results[i]` corresponds to `codes[i]`. This depends on `work.Map` preserving input order in its output. If the contract changes, errors would be misattributed.

**Patch-ready diff:**
```diff
--- a/multitenant.go
+++ b/multitenant.go
@@ -231,10 +231,10 @@ func (l *multiTenantLoader[E, P]) ReloadProjects(ctx context.Context) (*ReloadDi
 	newProjects := make(map[string]*P, len(results))
 	var reloadErrors []string
 	for i, r := range results {
-		if r.cfg != nil {
+		if r.code != "" && r.cfg != nil {
 			newProjects[r.code] = r.cfg
 		} else if mapErr != nil {
-			reloadErrors = append(reloadErrors, codes[i])
+			reloadErrors = append(reloadErrors, codes[i]) // Safe only if work.Map preserves order
 		}
 	}
```

---

## CODE QUALITY FINDINGS

### CQ-1: Generated Artifacts Tracked in Git

**Files:** `coverage.out`, `coverage_analysis.out`

Both `*.out` files are in `.gitignore` but were likely committed before the rule was added. They inflate the repo by ~80KB with transient data.

**Fix:**
```bash
git rm --cached coverage.out coverage_analysis.out
```

---

### CQ-2: Mismatched Doc Comment on FeatureFlagsFromValues

**File:** `feature_flags.go:187-189`

```go
// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
// This is a convenience function for extracting feature flags from a loaded config.
func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

The comment says `FeatureFlagsFromLoader` but the function is `FeatureFlagsFromValues`.

**Patch-ready diff:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -187,7 +187,7 @@ func parseBool(s string) bool {
 }

-// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
-// This is a convenience function for extracting feature flags from a loaded config.
+// FeatureFlagsFromValues creates a FeatureFlags instance from a raw values map.
+// Uses "FEATURE_" as the default prefix.
 func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

---

### CQ-3: Reimplemented Standard Library Functions

**File:** `fallback.go:157-168`

`splitEnv` and `hasPrefix` reimplement `strings.Cut` (Go 1.18+) and `strings.HasPrefix`. Using stdlib is clearer and avoids potential divergence.

**Patch-ready diff:**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -156,14 +156,10 @@ func (p *EnvProvider) FetchProject(ctx context.Context, project, config string)
 }

-func splitEnv(env string) (string, string) {
-	for i := 0; i < len(env); i++ {
-		if env[i] == '=' {
-			return env[:i], env[i+1:]
-		}
+func splitEnv(env string) (key, value string) {
+	key, value, ok := strings.Cut(env, "=")
+	if !ok {
+		return env, ""
 	}
-	return env, ""
-}
-
-func hasPrefix(s, prefix string) bool {
-	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
+	return key, value
 }
```

Then replace `hasPrefix(key, p.prefix)` with `strings.HasPrefix(key, p.prefix)`.

---

### CQ-4: `replace` Directive in go.mod

**File:** `go.mod:7`

```
replace github.com/ai8future/chassis-go/v9 => ../../chassis_suite/chassis-go
```

This local path replacement prevents external consumers from using this module directly (Go modules with replace directives cannot be consumed as dependencies). This should be removed before publishing.

---

### CQ-5: `SecretValue` Has No `UnmarshalJSON`

**File:** `config.go:167-169`

`MarshalJSON` returns `"[REDACTED]"` but there's no `UnmarshalJSON`. JSON round-trips are lossy -- deserializing a previously marshaled `SecretValue` yields an empty value, not `"[REDACTED]"`. This is likely intentional for security but should be documented.

---

## TEST COVERAGE GAPS

| Component | Has Tests? | Risk |
|---|---|---|
| Loader (single-tenant) | Yes | Low |
| Validation | Yes | Low |
| Chassis integration | Yes (with 1 failure) | Medium |
| DopplerProvider (HTTP) | Partial (via integration) | Medium |
| **FeatureFlags** | **No** | **High** -- includes caching, concurrency |
| **MultiTenantLoader** | **No** | **High** -- parallel loading, reload diffing |
| **Watcher** | **No** | **High** -- concurrency, failure counting, stop semantics |
| **EnvProvider** | **No** | **Medium** -- prefix filtering |
| **FileProvider (flattenJSON)** | **No direct tests** | **Medium** -- type coercion, nesting |

Missing test coverage for concurrent components (FeatureFlags, MultiTenantLoader, Watcher) is the largest quality risk. Race conditions in these areas would be caught by `go test -race` only if the code paths are exercised.

---

## POSITIVE OBSERVATIONS

1. **SecretValue redaction** -- `String()` and `MarshalJSON()` both redact, preventing accidental secret logging
2. **secval integration** -- Both Doppler API responses and fallback files are validated for dangerous JSON keys (prototype pollution)
3. **Error body limiting** -- Error responses capped at 1KB prevents info-leak amplification
4. **Circuit breaker** -- DopplerProvider uses chassis-go's circuit breaker for resilience
5. **Fallback file permissions** -- `WriteFallbackFile` uses 0600 (owner-only read/write)
6. **Thread safety** -- Consistent use of `sync.RWMutex` throughout with proper lock ordering
7. **Generic type parameters** -- Clean use of Go generics for type-safe config loading
8. **Options pattern** -- Consistent functional options for DopplerProvider, Loader, and Watcher
9. **Double-check locking** -- FeatureFlags.IsEnabled uses correct read-lock/write-lock pattern

---

## RECOMMENDED PRIORITY ORDER

1. **BUG-2** -- Fix stale test (tests currently FAIL)
2. **SEC-1** -- Bound success response body (OOM risk)
3. **BUG-1** -- Fix empty-string-as-missing semantics (production logic error)
4. **SEC-2** -- Add TLS warning for non-HTTPS URLs
5. **CQ-1** -- Remove committed coverage artifacts
6. **BUG-3** -- Fix or align flattenJSON comment
7. Add test coverage for FeatureFlags, MultiTenantLoader, Watcher
8. Remaining items
