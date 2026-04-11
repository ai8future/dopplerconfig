Date Created: 2026-02-16 21:35:20 UTC
TOTAL_SCORE: 88/100

# dopplerconfig Code Analysis & Fix Report

## Executive Summary

The dopplerconfig package is a well-structured Go configuration management library with clean interfaces, proper concurrency primitives, and good test coverage. The codebase builds cleanly with no `go vet` warnings. Issues found are mostly minor logic concerns and code smells rather than critical bugs. The overall quality is high for a production-ready library.

---

## Grading Breakdown

| Category | Score | Max | Notes |
|---|---|---|---|
| Correctness | 22 | 25 | One logic bug in ReloadProjects, one subtle edge case in unmarshal |
| Security | 24 | 25 | Strong: secret redaction, secval validation, limited error body reads |
| Code Quality | 22 | 25 | Clean interfaces, good naming, minor dead code and smells |
| Testing | 20 | 25 | Good coverage but missing edge cases around reload errors and watcher |

---

## Issues Found

### ISSUE 1: ReloadProjects error-tracking uses wrong condition (Medium Severity)

**File:** `multitenant.go:234-239`

The error-tracking loop after `work.Map` uses `r.cfg != nil` for the success path, but for the failure path it checks `mapErr != nil` (the aggregate error from `work.Map`). This means if *any* project fails, `mapErr` is non-nil, and *all* projects whose `r.cfg` is nil get recorded as failures — even those that might have had zero-value results for other reasons. The condition should directly check if the result was empty, not rely on the aggregate error.

```go
// Current code (lines 234-239):
for i, r := range results {
    if r.cfg != nil {
        newProjects[r.code] = r.cfg
    } else if mapErr != nil {
        reloadErrors = append(reloadErrors, codes[i])
    }
}
```

The `work.Map` returns results for ALL items regardless of success/failure. A failed item returns its zero value (`reloadResult{}` with empty `code` and nil `cfg`). The `r.code` field is empty string for failures, so `r.cfg != nil` is the correct success check. However, using `codes[i]` to identify the failed project assumes that `results` and `codes` are in the same order. This is safe only if `work.Map` guarantees order preservation. If it does, the logic is acceptable but fragile. If it doesn't, this is a real bug.

Additionally, the `else if mapErr != nil` check is redundant — if `r.cfg == nil` and `mapErr == nil`, it means the project returned a zero-value result without error, which would silently drop the project from `newProjects`. This should likely be `else` without the `mapErr` condition.

**Suggested Fix:**
```diff
--- a/multitenant.go
+++ b/multitenant.go
@@ -231,9 +231,9 @@
 	// Collect successful reloads (work.Map returns results for all items, including failed ones).
 	newProjects := make(map[string]*P, len(results))
 	var reloadErrors []string
 	for i, r := range results {
 		if r.cfg != nil {
 			newProjects[r.code] = r.cfg
-		} else if mapErr != nil {
+		} else {
 			reloadErrors = append(reloadErrors, codes[i])
 		}
 	}
```

---

### ISSUE 2: Default failure policy switch has unreachable dead code (Low Severity)

**File:** `loader.go:158-163`

The `FailurePolicy` type is an `int` with three constants: `FailurePolicyFail` (0), `FailurePolicyFallback` (1), `FailurePolicyWarn` (2). The default case in `LoadBootstrapFromEnv()` always sets `FailurePolicyFallback`, and `LoadBootstrapWithChassis()` only overrides for "fail" and "warn". So the default case in `loadFromProvider` can only be reached with `FailurePolicyFallback`, but `FailurePolicyFallback` is not in the switch — it falls through to `default`. This means `FailurePolicyFallback` actually triggers the error return path, which is incorrect behavior.

```go
// Current code (lines 150-163):
switch l.bootstrap.FailurePolicy {
case FailurePolicyFail:
    return nil, fmt.Errorf("failed to load configuration: %w", err)
case FailurePolicyWarn:
    l.logger.Warn("all providers failed, using defaults only", "error", err)
    values = make(map[string]string)
    source = "defaults"
default:
    if err != nil {
        return nil, fmt.Errorf("failed to load configuration: %w", err)
    }
    return nil, fmt.Errorf("no configuration available")
}
```

When `FailurePolicyFallback` is set (the default), this code falls to `default` which returns an error. This means the "fallback" policy behaves identically to "fail" — it returns an error when all providers fail. This is arguably correct since both providers (including the file fallback) already failed at this point, but the policy name is misleading. The `FailurePolicyFallback` case should be explicitly handled even if the behavior is the same, for clarity.

**Suggested Fix:**
```diff
--- a/loader.go
+++ b/loader.go
@@ -149,12 +149,13 @@
 	// Handle failure based on policy
 	if values == nil {
 		switch l.bootstrap.FailurePolicy {
-		case FailurePolicyFail:
+		case FailurePolicyFail, FailurePolicyFallback:
+			// FailurePolicyFallback also fails here because the fallback
+			// provider was already tried above and failed.
 			return nil, fmt.Errorf("failed to load configuration: %w", err)
 		case FailurePolicyWarn:
 			l.logger.Warn("all providers failed, using defaults only", "error", err)
 			values = make(map[string]string)
 			source = "defaults"
 		default:
 			if err != nil {
```

---

### ISSUE 3: unmarshalConfig treats empty string same as missing key (Low Severity)

**File:** `loader.go:298-304`

When a key exists in the values map but has an empty string value, the code treats it the same as a missing key and falls back to the default value. This may be intentional, but it means there's no way to explicitly set a field to empty string via Doppler — an empty value always becomes the default.

```go
// Current code (lines 298-304):
if !exists || rawValue == "" {
    defaultValue := field.Tag.Get(TagDefault)
    if defaultValue != "" {
        rawValue = defaultValue
        exists = true
    }
}
```

This is a design decision rather than a bug, but it's worth noting since it differs from how most config libraries work. An explicit empty string in Doppler would be silently replaced by the default, which could cause confusion.

**No fix recommended** — this is a design choice, but should be documented.

---

### ISSUE 4: `buildKey` normalizes name but doesn't normalize prefix (Low Severity)

**File:** `feature_flags.go:158-171`

The `buildKey` function upper-cases and normalizes the `name` parameter but compares against `f.prefix` without normalizing the prefix. If the prefix was set with mixed case (e.g., `"Feature_"`), the `HasPrefix` check would fail because `name` is upper-cased but `f.prefix` is not.

```go
// Current code (lines 158-171):
func (f *FeatureFlags) buildKey(name string) string {
    if f.prefix == "" {
        return name
    }
    // Normalize the name
    name = strings.ToUpper(name)
    name = strings.ReplaceAll(name, "-", "_")
    name = strings.ReplaceAll(name, " ", "_")

    if strings.HasPrefix(name, strings.ToUpper(f.prefix)) {
        return name
    }
    return f.prefix + name
}
```

The `HasPrefix` check does use `strings.ToUpper(f.prefix)`, but the returned key concatenates the original `f.prefix` (not uppercased) with the uppercased `name`. If `prefix` is "Feature_" and name is "rag_enabled", the key becomes `"Feature_RAG_ENABLED"` — mixed case, which won't match any map key if all keys are uppercase.

**Suggested Fix:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -158,6 +158,7 @@
 func (f *FeatureFlags) buildKey(name string) string {
 	if f.prefix == "" {
 		return name
 	}
 	// Normalize the name
 	name = strings.ToUpper(name)
@@ -167,5 +168,5 @@
 	if strings.HasPrefix(name, strings.ToUpper(f.prefix)) {
 		return name
 	}
-	return f.prefix + name
+	return strings.ToUpper(f.prefix) + name
 }
```

---

### ISSUE 5: Watcher.Stop() called from within run goroutine can deadlock (Low Severity)

**File:** `watcher.go:143-148`

When max failures are reached, `w.Stop()` is called via `go w.Stop()`. The `Stop()` method acquires `w.mu`, closes `stopCh`, then waits on `<-w.doneCh`. The `run()` goroutine's deferred cleanup also acquires `w.mu`. Since `go w.Stop()` starts a new goroutine, this avoids a direct deadlock. However, there's a subtle issue: if `Stop()` is called externally at the same time as `go w.Stop()` from the max-failures path, the `stopCh` channel would be closed twice, causing a panic.

```go
// Current code (lines 143-148):
if maxFail > 0 && failures >= maxFail {
    w.logger.Error("max failures reached, stopping watcher",
        "max_failures", maxFail,
    )
    go w.Stop()
}
```

The `go w.Stop()` goroutine and an external `Stop()` call could both try to close `stopCh`, which panics on double-close.

**Suggested Fix:**
```diff
--- a/watcher.go
+++ b/watcher.go
@@ -84,6 +84,9 @@
 func (w *Watcher[T]) Stop() {
 	w.mu.Lock()
 	if !w.running {
 		w.mu.Unlock()
 		return
 	}
+	w.running = false
 	close(w.stopCh)
 	w.mu.Unlock()
```

This prevents double-close by setting `running = false` before closing the channel, so a second caller would see `!w.running` and return early. The deferred cleanup in `run()` should also check before setting `running = false` again (which is harmless but redundant).

---

### ISSUE 6: `FeatureFlagsFromValues` function name doesn't match doc comment (Cosmetic)

**File:** `feature_flags.go:187-191`

The doc comment says `FeatureFlagsFromLoader` but the function is named `FeatureFlagsFromValues`. This is a misleading comment.

```go
// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
// This is a convenience function for extracting feature flags from a loaded config.
func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

**Suggested Fix:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -187,3 +187,3 @@
-// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
+// FeatureFlagsFromValues creates a FeatureFlags instance from a raw values map.
 // This is a convenience function for extracting feature flags from a loaded config.
 func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

---

### ISSUE 7: Error body truncation is redundant (Cosmetic)

**File:** `doppler.go:300-307`

The code reads up to 1024 bytes via `io.LimitReader`, then checks if the result is >= 1024 to truncate. Since `LimitReader` already constrains the read to 1024 bytes, the length will be at most 1024. The truncation replaces the last 3 bytes with "...", which is correct, but the `if` condition `len(rawBody) >= maxErrorBodySize` should be `==` since it can never be `>`.

```go
const maxErrorBodySize = 1024
limitedReader := io.LimitReader(resp.Body, maxErrorBodySize)
body, _ := io.ReadAll(limitedReader)
rawBody := string(body)
if len(rawBody) >= maxErrorBodySize {
    rawBody = rawBody[:maxErrorBodySize-3] + "..."
}
```

This is functionally correct but the `>=` is misleading. Not worth fixing — cosmetic only.

---

## Strengths

1. **Clean interface design** — `Provider`, `Loader[T]`, `MultiTenantLoader[E, P]` interfaces are well-separated and compose naturally.
2. **Security-conscious** — Secret value redaction via `SecretValue.String()` and `MarshalJSON()`, JSON security validation via `secval.ValidateJSON()`, limited error body reads.
3. **Proper concurrency** — Consistent use of `sync.RWMutex`, callback slice copying under lock, defensive map copying in providers.
4. **Good test infrastructure** — `MockProvider`, `RecordingProvider`, `TestLoader()`, `TestLoaderWithConfig()`, and `AssertConfigEqual()` helpers.
5. **Generic type safety** — Good use of Go generics for `Loader[T]` and `MultiTenantLoader[E, P]`.
6. **Circuit breaker integration** — Automatic retry and circuit breaking via chassis-go's `call.Client`.
7. **ETag caching** — Efficient cache-based config fetching with HTTP conditional requests.

## Testing Gaps

- No tests for `Watcher` stopping on max failures (race condition scenario)
- No tests for `MultiTenantWatcher`
- No tests for `ReloadProjects` error handling with partial failures
- No tests for `FeatureFlags.buildKey` with mixed-case prefixes
- No negative tests for `WriteFallbackFile` (permissions, invalid paths)

## Summary

This is a solid, production-quality Go package. The most impactful issue is #1 (ReloadProjects error tracking) which could mask reload failures in multi-tenant scenarios. Issue #5 (double-close panic) is a real concurrency bug but requires specific timing to trigger. The remaining issues are minor code quality improvements. No security vulnerabilities were found.
