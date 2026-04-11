Date Created: 2026-02-16T21:25:41-06:00
TOTAL_SCORE: 80/100

# dopplerconfig Code Audit Report

**Module:** `github.com/ai8future/dopplerconfig`
**Version:** 1.1.0 (CHANGELOG says 2.0.0)
**Go Version:** 1.25.5
**Key Dependency:** chassis-go v5.0.0
**Total Source Lines:** ~2,950 (excluding tests)
**Test Lines:** ~735
**Test Coverage:** 33.9%
**Tests Passing:** 34/34 (all pass, race-clean)

---

## Scoring Breakdown

| Category | Weight | Score | Notes |
|---|---|---|---|
| Code Quality & Idioms | 20 | 17 | Clean Go, good generics usage, minor issues |
| Security | 20 | 16 | secval integration good; some gaps identified |
| Correctness & Bugs | 20 | 16 | Subtle concurrency + logic bugs found |
| Test Coverage | 15 | 7 | 33.9% is low; multitenant/watcher/feature_flags untested |
| API Design | 15 | 14 | Clean provider abstraction, good generics |
| Documentation | 10 | 10 | Excellent godoc, CHANGELOG, AGENTS.md |
| **TOTAL** | **100** | **80** | |

---

## Critical Issues (Must Fix)

### C1. Unbounded Response Body Read (Security) — `doppler.go:315`

**Severity:** HIGH | **Category:** Security / DoS

The success path reads the entire Doppler API response body without any size limit. The error path correctly limits to 1KB (`doppler.go:301-303`), but the happy path at line 315 does not:

```go
body, err := io.ReadAll(resp.Body) // No size limit!
```

A compromised or misconfigured Doppler API could return an arbitrarily large JSON response, causing OOM.

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,9 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit response body to 10MB to prevent OOM from unexpected large responses
+	const maxResponseSize = 10 * 1024 * 1024
+	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
```

### C2. ETag Cache Shared Across Projects (Bug) — `doppler.go:267-271, 341-346`

**Severity:** HIGH | **Category:** Correctness

`DopplerProvider` stores a single `etag` and `cache` per provider instance. When `FetchProject` is called with different `(project, config)` combinations (multi-tenant use), the ETag from one project will be sent for a completely different project, causing either stale data or unnecessary 200 responses:

```go
// Line 267-270: sends ETag regardless of which project/config is being fetched
p.mu.RLock()
if p.etag != "" {
    req.Header.Set("If-None-Match", p.etag)
}
p.mu.RUnlock()

// Line 341-346: caches result regardless of project/config
p.mu.Lock()
p.cache = result  // Overwrites previous project's cache!
```

```diff
--- a/doppler.go
+++ b/doppler.go
@@ -131,8 +131,8 @@ type DopplerProvider struct {
 	client  httpDoer
 	breaker *call.CircuitBreaker
 	logger  *slog.Logger
 	mu      sync.RWMutex
-	cache   map[string]string
-	etag    string
+	cache   map[string]map[string]string // keyed by "project/config"
+	etags   map[string]string            // keyed by "project/config"
 }

 // In NewDopplerProvider, initialize:
@@ -193,6 +193,8 @@ func NewDopplerProvider(token, project, config string, opts ...DopplerProviderOpt
 		breaker: breaker,
 		logger:  slog.Default(),
+		cache:   make(map[string]map[string]string),
+		etags:   make(map[string]string),
 		client: call.New(

 // In FetchProject, use composite key:
@@ -244,9 +244,10 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin

+	cacheKey := project + "/" + config
+
 	// Add ETag for caching if available
 	p.mu.RLock()
-	if p.etag != "" {
-		req.Header.Set("If-None-Match", p.etag)
+	if etag, ok := p.etags[cacheKey]; ok && etag != "" {
+		req.Header.Set("If-None-Match", etag)
 	}
 	p.mu.RUnlock()

@@ -284,9 +285,9 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 	if resp.StatusCode == http.StatusNotModified {
 		p.mu.RLock()
-		cached := make(map[string]string, len(p.cache))
-		for k, v := range p.cache {
+		src := p.cache[cacheKey]
+		cached := make(map[string]string, len(src))
+		for k, v := range src {
 			cached[k] = v
 		}
 		p.mu.RUnlock()
@@ -340,9 +341,9 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin

 	// Update cache with new ETag
 	p.mu.Lock()
-	p.cache = result
+	p.cache[cacheKey] = result
 	if etag := resp.Header.Get("ETag"); etag != "" {
-		p.etag = etag
+		p.etags[cacheKey] = etag
 	}
 	p.mu.Unlock()
```

### C3. VERSION / CHANGELOG Mismatch

**Severity:** MEDIUM | **Category:** Process

`VERSION` file says `1.1.0` but `CHANGELOG.md` says the latest release is `2.0.0`. This is confusing for consumers. Either VERSION should be `2.0.0` or the CHANGELOG heading is wrong.

---

## Significant Issues (Should Fix)

### S1. Regex DoS via Validation Tags (Security) — `validation.go:432-441`

**Severity:** MEDIUM | **Category:** Security

`getCompiledRegex` compiles user-supplied regex patterns from struct tags and caches them with `sync.Map`. While struct tags are typically developer-controlled, if a service exposes a mechanism to define validation rules dynamically, a pathological regex could cause ReDoS. The `sync.Map` cache also grows unboundedly.

```diff
--- a/validation.go
+++ b/validation.go
@@ -427,6 +427,14 @@ func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
 		return cached.(*regexp.Regexp), nil
 	}

+	// Limit pattern complexity to prevent ReDoS
+	const maxPatternLen = 1024
+	if len(pattern) > maxPatternLen {
+		return nil, fmt.Errorf("regex pattern too long (%d chars, max %d)", len(pattern), maxPatternLen)
+	}
+
 	re, err := regexp.Compile(pattern)
```

### S2. Watcher `Stop()` Self-Deadlock Risk — `watcher.go:147`

**Severity:** MEDIUM | **Category:** Concurrency Bug

In `poll()`, when max failures is reached, `Stop()` is called via `go w.Stop()`. The goroutine calling `Stop()` closes `w.stopCh` and waits on `w.doneCh`. Meanwhile `run()` is blocked in `poll()`. Since `poll()` returns after calling `go w.Stop()`, `run()` picks up the stop signal and closes `doneCh`, which unblocks `Stop()`. This works today but is fragile — if `poll()` ever blocks after dispatching `go w.Stop()`, it would deadlock. A more robust pattern is to use a non-blocking channel send:

```diff
--- a/watcher.go
+++ b/watcher.go
@@ -144,7 +144,12 @@ func (w *Watcher[T]) poll(ctx context.Context) {
 			w.logger.Error("max failures reached, stopping watcher",
 				"max_failures", maxFail,
 			)
-			go w.Stop()
+			// Non-blocking signal to stop — avoids potential deadlock
+			// since we're called from within run()
+			select {
+			case w.stopCh <- struct{}{}:
+			default:
+			}
 		}
```

Wait — `stopCh` is a `chan struct{}` that is closed to signal stop, not sent to. The `go w.Stop()` pattern works by calling `close(w.stopCh)` from a separate goroutine. This is actually safe as-is because `Stop()` acquires the lock, closes the channel, then waits on `doneCh`. Since `poll()` returns after the `go w.Stop()` launch, `run()` will see the closed `stopCh` and exit. However, if `close(w.stopCh)` is called twice (e.g., external Stop + max-failure Stop racing), it panics. A `sync.Once` or a non-blocking approach is safer:

```diff
--- a/watcher.go
+++ b/watcher.go
@@ -17,6 +17,7 @@ type Watcher[T any] struct {
 	running      bool
 	stopCh       chan struct{}
 	doneCh       chan struct{}
+	stopOnce     sync.Once
 	failureCount int
 	maxFailures  int
 }
@@ -84,8 +85,10 @@ func (w *Watcher[T]) Stop() {
 	if !w.running {
 		w.mu.Unlock()
 		return
 	}
-	close(w.stopCh)
+	w.stopOnce.Do(func() {
+		close(w.stopCh)
+	})
 	w.mu.Unlock()

 	// Wait for goroutine to finish
```

### S3. Callbacks Invoked Outside Lock Can See Stale State — `loader.go:185-193`

**Severity:** MEDIUM | **Category:** Concurrency

In `loadFromProvider`, the callback list is captured under the lock, then callbacks are invoked outside the lock:

```go
l.mu.Lock()
old := l.current
l.current = cfg
// ...
callbacks := l.callbacks  // captures slice header
l.mu.Unlock()

if isReload && old != nil {
    for _, cb := range callbacks {
        cb(old, cfg)  // old and cfg are pointers — may be mutated concurrently
    }
}
```

If another goroutine calls `Reload()` concurrently, `cfg` (a pointer) could be overwritten. The callbacks receive the right pointers, but the config structs pointed to are mutable. This is a documentation gap at minimum — callers should be warned not to mutate the config in callbacks.

### S4. `FailurePolicyFallback` Acts Like `FailurePolicyFail` — `loader.go:158-163`

**Severity:** MEDIUM | **Category:** Logic Bug

When both providers fail and the failure policy is `FailurePolicyFallback` (the default), the code hits the `default` branch which returns an error — identical behavior to `FailurePolicyFail`:

```go
switch l.bootstrap.FailurePolicy {
case FailurePolicyFail:
    return nil, fmt.Errorf("failed to load configuration: %w", err)
case FailurePolicyWarn:
    l.logger.Warn("all providers failed, using defaults only", "error", err)
    values = make(map[string]string)
    source = "defaults"
default:  // FailurePolicyFallback lands here
    if err != nil {
        return nil, fmt.Errorf("failed to load configuration: %w", err)
    }
    return nil, fmt.Errorf("no configuration available")
}
```

This means `FailurePolicyFallback` and `FailurePolicyFail` are functionally identical. The intent seems to be that `Fallback` should use the fallback file — but the fallback was already attempted above. The `default` branch should arguably also try defaults (like `Warn`) or be renamed/documented to clarify the distinction.

```diff
--- a/loader.go
+++ b/loader.go
@@ -149,11 +149,10 @@ func (l *loader[T]) loadFromProvider(ctx context.Context, isReload bool) (*T, er
 	if values == nil {
 		switch l.bootstrap.FailurePolicy {
 		case FailurePolicyFail:
 			return nil, fmt.Errorf("failed to load configuration: %w", err)
-		case FailurePolicyWarn:
+		case FailurePolicyWarn, FailurePolicyFallback:
+			// Fallback was already attempted above; use defaults as last resort
 			l.logger.Warn("all providers failed, using defaults only", "error", err)
 			values = make(map[string]string)
 			source = "defaults"
-		default:
-			if err != nil {
-				return nil, fmt.Errorf("failed to load configuration: %w", err)
-			}
-			return nil, fmt.Errorf("no configuration available")
+		default: // FailurePolicyFail is the safe default
+			return nil, fmt.Errorf("failed to load configuration: %w", err)
 		}
 	}
```

---

## Minor Issues (Nice to Fix)

### M1. `SecretValue` Not Truly Immutable — `config.go:144-164`

`SecretValue` has an unexported `value` field (good), but it is a value type (struct, not pointer). Since Go copies structs on assignment, and the field is a string (immutable in Go), this is actually safe. However, `MarshalJSON` always returns `[REDACTED]` — `UnmarshalJSON` is not implemented, meaning `json.Unmarshal` into a `SecretValue` field will fail silently (the value stays empty). If config structs are ever round-tripped through JSON (e.g., for caching), secrets will be lost.

### M2. `validateURL` Accepts Relative Paths — `validation.go:302`

`url.ParseRequestURI` accepts paths like `/foo/bar` as valid. Consider using `url.Parse` and checking for a scheme:

```diff
--- a/validation.go
+++ b/validation.go
@@ -299,7 +299,8 @@ func validateURL(value reflect.Value, fieldName string) *ValidationError {
 		return nil
 	}

-	_, err := url.ParseRequestURI(s)
+	u, err := url.ParseRequestURI(s)
-	if err != nil {
+	if err != nil || u.Scheme == "" {
 		return &ValidationError{
```

### M3. `FeatureFlags.IsEnabled` Case-Insensitive Fallback Is O(n) — `feature_flags.go:54-59`

When the exact key isn't found, `IsEnabled` iterates all values with `strings.EqualFold`. For large value maps this is slow. Consider normalizing keys at construction time.

### M4. No Test Coverage for Key Components

| Component | Coverage |
|---|---|
| `multitenant.go` (488 lines) | 0% |
| `watcher.go` (178 lines) | 0% |
| `feature_flags.go` (250 lines) | 0% |
| `fallback.go` - EnvProvider, flattenJSON | 0% |
| `config.go` - LoadBootstrapFromEnv | 0% |

These represent significant untested surface area. The 33.9% total coverage is well below the recommended 60%+ for a shared library.

### M5. `flattenJSON` Does Not Uppercase Keys — `fallback.go:61-99`

The comment says it converts `{"server": {"port": 8080}}` to `{"SERVER_PORT": "8080"}`, but the code preserves original casing. Since Doppler keys are typically UPPER_SNAKE_CASE, and the struct tags match exactly, a nested JSON file with lowercase keys won't match struct tags like `doppler:"DATABASE_URL"`.

```diff
--- a/fallback.go
+++ b/fallback.go
@@ -62,7 +62,7 @@ func flattenJSON(prefix string, data map[string]interface{}, result map[string]s
 	for key, value := range data {
-		fullKey := key
+		fullKey := strings.ToUpper(key)
 		if prefix != "" {
-			fullKey = prefix + "_" + key
+			fullKey = strings.ToUpper(prefix) + "_" + strings.ToUpper(key)
 		}
```

### M6. `multiTenantLoader.fetchWithFallback` Returns `nil` Error When Both Providers Are Nil — `multitenant.go:357-378`

If `l.provider` is nil and `l.fallback` is nil (which shouldn't happen after constructor validation, but could in `NewMultiTenantLoaderWithProvider` with nil args), `fetchWithFallback` returns `nil, nil` — no values and no error. This silent failure would be hard to debug.

### M7. `EnvProvider` Does Not Strip Prefix From Keys — `fallback.go:144-155`

When an `EnvProvider` has a prefix like `"APP_"`, it filters variables to only include those with that prefix, but the resulting keys still include the prefix (e.g., `APP_PORT`). Struct tags would need to include the prefix too (`doppler:"APP_PORT"`), which is likely not the intent.

### M8. `email` Regex Allows Long Local Parts

The email regex `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$` has no length limit. RFC 5321 limits the local part to 64 characters and the domain to 253 characters. This is minor but could allow pathological inputs.

---

## Security Summary

| Area | Status | Notes |
|---|---|---|
| Token Handling | GOOD | Bearer token in Authorization header, not in URL query params |
| Secret Redaction | GOOD | SecretValue redacts in String() and MarshalJSON() |
| JSON Validation | GOOD | secval.ValidateJSON on both API responses and fallback files |
| Error Body Limiting | PARTIAL | Error path limited to 1KB; success path unbounded (C1) |
| TLS | GOOD | Default API URL is HTTPS |
| File Permissions | GOOD | WriteFallbackFile uses 0600 |
| Dependency Pinning | GOOD | go.sum with checksums |
| Input Validation | GOOD | Comprehensive validation framework |
| Race Conditions | CLEAN | Race detector passes all tests |
| Regex DoS | LOW RISK | Struct tags are developer-controlled (S1) |

---

## Code Quality Observations

**Strengths:**
- Clean, idiomatic Go with well-designed generic abstractions
- Good use of the options pattern for configuration
- Provider interface is well-designed and easy to test with mocks
- Thread-safe design throughout (proper mutex usage, defensive copies)
- Error handling is thorough with proper wrapping
- chassis-go integration is well-documented with bridge functions
- secval integration for JSON security validation is a strong security choice

**Weaknesses:**
- Test coverage is too low for a shared library (33.9%)
- Multi-tenant loader (488 lines) has zero test coverage
- Watcher (178 lines) has zero test coverage
- Feature flags (250 lines) has zero test coverage
- VERSION / CHANGELOG are out of sync

---

## Recommendations (Priority Order)

1. **Fix C1** — Add response body size limit on success path
2. **Fix C2** — Per-project ETag/cache storage
3. **Fix C3** — Sync VERSION and CHANGELOG
4. **Fix S4** — Clarify `FailurePolicyFallback` behavior
5. **Fix S2** — Add `sync.Once` to watcher Stop
6. **Add tests** for multitenant, watcher, feature_flags (target 60%+ coverage)
7. **Fix M5** — flattenJSON key casing
8. **Fix M2** — URL validator scheme check
