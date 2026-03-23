Date Created: 2026-03-21 02:10:42 CET
TOTAL_SCORE: 79/100

# dopplerconfig Audit Report

**Auditor**: Claude Code (Claude:Opus 4.6)
**Package**: `github.com/ai8future/dopplerconfig`
**Version**: 1.1.6
**Go Version**: 1.25.5
**Primary Dependency**: chassis-go v9.0.0 (local replace)
**Files Audited**: 14 source files (~3,700 LOC), 5 test files (~735 LOC)

---

## Score Breakdown

| Category | Weight | Score | Notes |
|---|---|---|---|
| Security | 25 | 19/25 | Good foundations, several medium-severity gaps |
| Correctness | 25 | 19/25 | One failing test, minor logic issues |
| Code Quality | 20 | 18/20 | Clean, idiomatic Go; good patterns |
| Test Coverage | 15 | 11/15 | Core paths covered, gaps in edge cases |
| Architecture | 10 | 9/10 | Clean separation, good extensibility |
| Documentation | 5 | 3/5 | Good README, stale inline comments |

---

## CRITICAL: Failing Test

### BUG-01: `TestChassisVersion` hardcoded to wrong major version [SEVERITY: HIGH]

**File**: `chassis_test.go:247-249`
**Status**: TEST FAILS on `go test ./...`

The test asserts `ChassisVersion` starts with `'7'`, but chassis-go v9 reports `"9.0.2"`.

```
--- FAIL: TestChassisVersion (0.00s)
    chassis_test.go:248: ChassisVersion = "9.0.2", want major version 7
```

**Root Cause**: Test was not updated during chassis-go v7 → v8 → v9 upgrades.

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

## Security Findings

### SEC-01: Unbounded response body read on success path [SEVERITY: HIGH]

**File**: `doppler.go:315`

Error responses are properly limited to 1KB via `io.LimitReader` (line 302), but successful responses use unbounded `io.ReadAll`. A compromised or misbehaving Doppler endpoint could return an arbitrarily large payload causing OOM.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,9 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config stri
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit response body to 10MB to prevent memory exhaustion
+	const maxResponseSize = 10 * 1024 * 1024
+	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

### SEC-02: No HTTPS enforcement on API URL [SEVERITY: MEDIUM]

**File**: `doppler.go:142-146`

`WithAPIURL()` accepts any URL including plain HTTP. A misconfiguration or malicious override could cause tokens and secrets to transit in cleartext.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -141,6 +141,9 @@ type DopplerProviderOption func(*DopplerProvider)
 // WithAPIURL sets a custom Doppler API URL.
 func WithAPIURL(url string) DopplerProviderOption {
 	return func(p *DopplerProvider) {
+		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://localhost") && !strings.HasPrefix(url, "http://127.0.0.1") {
+			p.logger.Warn("doppler API URL is not HTTPS — secrets may transit in cleartext", "url", url)
+		}
 		p.apiURL = url
 	}
 }
```

Note: This needs `"strings"` added to imports.

### SEC-03: FileProvider leaks filesystem path in Name() [SEVERITY: LOW]

**File**: `fallback.go:103`

`Name()` returns `"file:" + p.path`, which may expose internal filesystem structure when logged.

```diff
--- a/fallback.go
+++ b/fallback.go
@@ -100,7 +100,8 @@ func (p *FileProvider) Fetch(ctx context.Context) (map[string]string, error) {

 // Name returns the provider name.
 func (p *FileProvider) Name() string {
-	return "file:" + p.path
+	// Use only the base filename to avoid leaking full filesystem paths in logs
+	return "file"
 }
```

### SEC-04: Regex cache unbounded — potential memory leak [SEVERITY: LOW]

**File**: `validation.go:425-441`

`regexCache` is a `sync.Map` with no eviction policy. If user-controlled regex patterns are validated (e.g., from config struct tags built dynamically), the cache grows without bound.

In practice, struct tags are compile-time constants, so this is low risk. But if dopplerconfig is ever used with dynamically-generated structs, this becomes a DoS vector.

```diff
--- a/validation.go
+++ b/validation.go
@@ -424,6 +424,9 @@ func validateOneOf(value reflect.Value, param string, fieldName string) *Validat
 // regexCache caches compiled regular expressions for validation.
 var regexCache sync.Map

+// maxRegexCacheSize is the maximum number of cached regex patterns.
+const maxRegexCacheSize = 1000
+
 // getCompiledRegex returns a cached compiled regex, or compiles and caches it.
 func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
 	if cached, ok := regexCache.Load(pattern); ok {
@@ -435,7 +438,14 @@ func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
 		return nil, err
 	}

-	// Store in cache (may race with another goroutine, but that's fine)
+	// Approximate size check — sync.Map doesn't expose length,
+	// so count entries. This is O(n) but only runs on cache miss.
+	var count int
+	regexCache.Range(func(_, _ any) bool { count++; return count < maxRegexCacheSize })
+	if count >= maxRegexCacheSize {
+		return re, nil // Don't cache, but still return
+	}
+
 	regexCache.Store(pattern, re)
 	return re, nil
 }
```

### SEC-05: DopplerError.Raw may contain sensitive data [SEVERITY: LOW]

**File**: `doppler.go:303-312`

The raw error body from Doppler is stored in `DopplerError.Raw` and could be logged, potentially leaking sensitive information from error responses.

No patch recommended — just document the risk. The 1KB limit already mitigates the worst case.

### SEC-06: Missing `SecretValue.UnmarshalJSON` [SEVERITY: LOW]

**File**: `config.go:167-169`

`MarshalJSON` redacts to `"[REDACTED]"`, but there's no `UnmarshalJSON`. If serialized and deserialized via JSON, the value becomes literally `"[REDACTED]"`. This is by design for output safety, but could cause confusion if someone marshals a config to JSON and tries to load it back.

```diff
--- a/config.go
+++ b/config.go
@@ -169,3 +169,11 @@ func (s SecretValue) String() string {
 func (s SecretValue) MarshalJSON() ([]byte, error) {
 	return []byte(`"[REDACTED]"`), nil
 }
+
+// UnmarshalJSON is intentionally NOT implemented.
+// SecretValue is designed for one-way output redaction. If you need to
+// round-trip secret values through JSON, use the underlying string type
+// and convert with NewSecretValue() after deserialization.
+//
+// Attempting to json.Unmarshal into a SecretValue will set the value
+// field to the empty string (zero value), which is the safe default.
```

---

## Correctness Findings

### COR-01: `flattenJSON` preserves original casing — may cause key mismatch [SEVERITY: MEDIUM]

**File**: `fallback.go:61-98`

When flattening nested JSON from fallback files, keys preserve their original casing (e.g., `{"server": {"port": 8080}}` → `"server_port"`). But struct tags typically use UPPER_CASE (`doppler:"SERVER_PORT"`). This means nested fallback JSON won't match struct tags unless the JSON also uses uppercase keys.

```diff
--- a/fallback.go
+++ b/fallback.go
@@ -60,7 +60,7 @@ func flattenJSON(prefix string, data map[string]interface{}, result map[string]s
 func flattenJSON(prefix string, data map[string]interface{}, result map[string]string) {
 	for key, value := range data {
-		fullKey := key
+		fullKey := strings.ToUpper(key)
 		if prefix != "" {
-			fullKey = prefix + "_" + key
+			fullKey = prefix + "_" + strings.ToUpper(key)
 		}
```

Note: This is a behavioral change. If existing fallback files rely on lowercase keys matching lowercase struct tags, this would break them. Consider making it optional.

### COR-02: `isZero` treats `false` as zero value — impacts `required` validation [SEVERITY: LOW]

**File**: `validation.go:482`

`isZero` returns `true` for `bool` fields that are `false`. This means `required:"true"` on a `bool` field set to `false` will trigger a "required field is missing" error, even though `false` is a valid value. This is a known Go convention issue (zero value vs. absent value) but could surprise users.

No patch — document this as a known limitation:

> **Note**: For `bool` fields, `required:"true"` checks that the value is `true` (not just present). Use a `string` field with `validate:"oneof=true|false"` if you need explicit presence checking.

### COR-03: Watcher `Stop()` calls itself from `poll()` — potential goroutine leak [SEVERITY: LOW]

**File**: `watcher.go:147`

When `maxFailures` is reached, `poll()` calls `go w.Stop()` in a new goroutine. `Stop()` closes `stopCh` and then blocks on `<-w.doneCh`. But `poll()` is called from `run()` which is the goroutine that closes `doneCh`. This works because `Stop()` is called in a *new* goroutine (`go w.Stop()`), so `run()` can proceed to return and close `doneCh`. However, there's a race: if `Stop()` is called externally at the same time, double-closing `stopCh` would panic.

```diff
--- a/watcher.go
+++ b/watcher.go
@@ -144,7 +144,13 @@ func (w *Watcher[T]) poll(ctx context.Context) {
 		if maxFail > 0 && failures >= maxFail {
 			w.logger.Error("max failures reached, stopping watcher",
 				"max_failures", maxFail,
 			)
-			go w.Stop()
+			// Signal stop without calling Stop() to avoid double-close race on stopCh.
+			w.mu.Lock()
+			if w.running {
+				close(w.stopCh)
+			}
+			w.mu.Unlock()
 		}
 		return
 	}
```

---

## Code Quality Findings

### QUA-01: Comment says "major version 7" but code uses v9 [SEVERITY: LOW]

**File**: `chassis_test.go:247`

Stale comment from before the v7→v9 upgrade. Already covered by BUG-01.

### QUA-02: `FeatureFlagsFromValues` function name doesn't match its doc comment [SEVERITY: TRIVIAL]

**File**: `feature_flags.go:187-191`

Doc comment says `FeatureFlagsFromLoader` but function is named `FeatureFlagsFromValues`.

```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -187,7 +187,7 @@ func parseBool(s string) bool {
 	}
 }

-// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
+// FeatureFlagsFromValues creates a FeatureFlags instance from a config values map.
 // This is a convenience function for extracting feature flags from a loaded config.
 func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

### QUA-03: `buildKey` normalizes to uppercase but prefix may not be uppercase [SEVERITY: TRIVIAL]

**File**: `feature_flags.go:158-171`

`buildKey()` uppercases the name but not the prefix. If someone passes a lowercase prefix like `"feature_"`, the check `strings.HasPrefix(name, strings.ToUpper(f.prefix))` will uppercase it for comparison, but the concatenation `f.prefix + name` will use the original case.

No patch — very unlikely edge case given the documented convention of uppercase prefixes.

---

## Test Coverage Gaps

| Area | Status | Notes |
|---|---|---|
| Loader (single config) | Covered | Load, Reload, OnChange, defaults, required fields |
| Validation (all 8 rules) | Covered | Min, max, port, URL, email, oneof, regex, host |
| Feature flags | NOT TESTED | No test file for feature_flags.go |
| Watcher | NOT TESTED | No test file for watcher.go |
| MultiTenantLoader | NOT TESTED | No test file for multitenant.go |
| FileProvider | PARTIAL | Tested via secval dangerous keys only |
| EnvProvider | NOT TESTED | No test coverage |
| DopplerProvider (API calls) | PARTIAL | Health check and secval tested; normal fetch not integration-tested |
| Error handling paths | PARTIAL | Missing tests for fallback, FailurePolicyWarn |
| Slice parsing edge cases | Covered | Int, bool, string slices tested |

**Missing test files** (recommend adding):
- `feature_flags_test.go`
- `watcher_test.go`
- `multitenant_test.go`
- `fallback_test.go` (beyond secval)

---

## Architecture Assessment

**Strengths:**
- Clean Provider interface enables testing and extensibility
- Generic `Loader[T]` pattern with type safety
- Consistent options pattern (`With*` functions)
- Proper mutex usage throughout (RWMutex for read-heavy paths, double-check locking in feature flags)
- Good error handling with `%w` wrapping
- SecretValue type prevents accidental secret leakage in logs/JSON
- JSON security validation via chassis-go's secval
- ETag caching reduces API load
- Circuit breaker + retry for resilience

**Concerns:**
- Heavy coupling to chassis-go (local replace directive means this can't be used independently)
- No CI/CD pipeline configured
- No Makefile or build scripts
- `go.mod` uses local `replace` directive — must be removed before publishing

---

## Summary of All Findings

| ID | Severity | Category | Description |
|---|---|---|---|
| BUG-01 | HIGH | Correctness | `TestChassisVersion` fails — hardcoded to v7, should be v9 |
| SEC-01 | HIGH | Security | Unbounded `io.ReadAll` on success response in `doppler.go:315` |
| SEC-02 | MEDIUM | Security | No HTTPS enforcement on `WithAPIURL()` |
| COR-01 | MEDIUM | Correctness | `flattenJSON` preserves casing — may mismatch uppercase struct tags |
| SEC-03 | LOW | Security | FileProvider `Name()` leaks full filesystem path |
| SEC-04 | LOW | Security | Regex cache unbounded — potential memory leak |
| SEC-05 | LOW | Security | `DopplerError.Raw` may contain sensitive data |
| SEC-06 | LOW | Security | Missing `SecretValue.UnmarshalJSON` documentation |
| COR-02 | LOW | Correctness | `isZero` treats `false` as zero — impacts `required` on bools |
| COR-03 | LOW | Correctness | Watcher `Stop()` double-close race on `stopCh` |
| QUA-01 | LOW | Quality | Stale comment references v7 |
| QUA-02 | TRIVIAL | Quality | Doc comment says `FeatureFlagsFromLoader`, function is `FeatureFlagsFromValues` |
| QUA-03 | TRIVIAL | Quality | `buildKey` prefix case handling inconsistency |

---

## Final Grade: 79/100

**Good, production-usable library with notable gaps.** The code is clean, idiomatic Go with solid architecture. The security fundamentals are in place (secret redaction, JSON validation, circuit breaking). However, the failing test, unbounded response body, missing test coverage for 3 major components (feature flags, watcher, multitenant), and lack of CI/CD hold it back from an excellent score.

**Top 3 actions to improve the score:**
1. Fix the failing test and add response body size limits (+6 points)
2. Add test files for feature flags, watcher, and multitenant (+8 points)
3. Set up CI/CD with `go test`, `go vet`, and `staticcheck` (+4 points)
