Date Created: 2026-02-16 21:38:24 CET
TOTAL_SCORE: 78/100

# Refactor Report: dopplerconfig

## Executive Summary

dopplerconfig is a well-structured Go configuration library (v1.1.0) with strong type safety via generics, good resilience patterns through chassis-go v5, and clean separation of concerns across 10 core modules (~3,700 lines). The codebase earns a solid 78/100 — it's production-quality with room for improvement in test coverage, minor duplication, and a few structural refinements.

---

## Scoring Breakdown

| Category                    | Score | Max | Notes                                          |
|-----------------------------|-------|-----|-------------------------------------------------|
| Code Organization           | 16    | 20  | Clean module split; some files getting large    |
| Duplication                 | 12    | 15  | Minor duplication in type conversion and parsing|
| Error Handling              | 11    | 15  | Good wrapping; some silent fallthrough paths    |
| Test Coverage               | 7     | 15  | 33.9% is below target; key paths untested       |
| API Design & Consistency    | 14    | 15  | Strong generics usage; consistent option pattern|
| Maintainability             | 10    | 10  | Well-documented; clear conventions              |
| Security                    | 8     | 10  | SecretValue, secval, 0600 perms — solid         |
| **TOTAL**                   | **78**| **100** |                                             |

---

## Detailed Findings

### 1. Code Organization (16/20)

**Strengths:**
- Clean Provider interface pattern with DopplerProvider, FileProvider, EnvProvider, and MockProvider
- Generics well-applied: `Loader[T]`, `MultiTenantLoader[E, P]`, `Watcher[T]`
- Chassis-go bridge isolated to `chassis.go` — easy to evolve independently

**Opportunities:**
- **loader.go (438 lines)** handles both the Loader struct/methods and the `unmarshalConfig()` reflection engine. The unmarshaling logic (lines ~180-440) is a distinct concern that could live in its own `unmarshal.go` file. This would improve readability and make the reflection code independently testable.
- **validation.go (500 lines)** is the largest file. The regex caching, hostname validation, and email validation are self-contained utilities that could be extracted into a `validators.go` or similar. Not urgent, but would improve navigability.
- **multitenant.go (489 lines)** packs the MultiTenantLoader, its options, the ReloadDiff type, and the MultiTenantWatcher all into one file. The watcher portion could be split out since it's a distinct runtime concern.

### 2. Duplication (12/15)

**Boolean Parsing Duplication:**
- `feature_flags.go` lines 173-184 implement boolean parsing with extended values ("yes", "on", "enabled", "enable", "1")
- `loader.go` uses `strconv.ParseBool()` for bool fields which only accepts "true"/"false"/"1"/"0"/etc.
- These two code paths parse booleans differently. A user could set `FEATURE_X=yes` in feature flags (works) but the same value as a `bool` struct field would fail. A shared `parseBool()` utility would eliminate this inconsistency.

**Type Conversion Patterns:**
- `fallback.go` has type conversion in `flattenJSON()` (numbers, bools, nested objects)
- `loader.go` has type conversion in `unmarshalConfig()` (strings to int/float/bool/duration)
- While these serve different purposes (JSON normalization vs struct population), the string-to-number conversion pattern appears in both. Minor, but a shared `convertValue()` helper could reduce surface area.

**Slice Parsing:**
- `loader.go` lines 325-395 has three nearly identical blocks for `[]string`, `[]int`, and `[]bool` slices — split string, trim, convert each element. This is a classic case for a small generic helper like `parseSlice[T](raw string, converter func(string) (T, error)) ([]T, error)`.

### 3. Error Handling (11/15)

**Strengths:**
- Consistent use of `fmt.Errorf` with `%w` for error wrapping
- `DopplerError` properly maps HTTP status codes to chassis-go `ServiceError` types
- `ValidationErrors` aggregates multiple failures cleanly
- Body size capped at 1KB to prevent memory issues from error responses

**Opportunities:**
- **Silent type fallthrough in unmarshalConfig():** When a struct field has an unsupported type, the code silently skips it (no error, no warning). This can mask misconfiguration. At minimum, a warning in metadata would help.
- **flattenJSON error suppression:** In `fallback.go`, when JSON values can't be converted during flattening, they're silently dropped. A debug-level log would help operators troubleshoot missing config values.
- **Error context in parsing failures:** Messages like `"failed to parse int field"` don't include the field name or the raw value that failed. Adding these would significantly improve debuggability. Example: `"failed to parse int field %s: value %q: %w"`.
- **ReloadProjects partial failure:** `multitenant.go` collects errors from parallel project reloads but returns a single joined error. There's no way for the caller to distinguish which projects failed vs succeeded. A structured error type (or populating ReloadDiff with failure details) would be more useful.

### 4. Test Coverage (7/15)

**Current state: 33.9% statement coverage**

This is the biggest area for improvement. Key untested paths:

| Component | Coverage Gap |
|-----------|-------------|
| MultiTenantLoader | LoadProject, LoadAllProjects, ReloadProjects — the parallel loading and diff tracking logic is entirely untested |
| Watcher / MultiTenantWatcher | Start/Stop/polling loops, max failure handling, graceful shutdown |
| FeatureFlags | Caching behavior, RolloutConfig.ShouldEnable, Update() cache invalidation |
| FileProvider | Actual file I/O, nested JSON flattening, WriteFallbackFile |
| DopplerProvider | Real HTTP request flow, ETag caching, circuit breaker state transitions |
| EnvProvider | Prefix filtering |

The existing tests are well-written and cover the core loader + validation + chassis bridge thoroughly. Expanding to the above areas would bring coverage to a much healthier 65-75%.

### 5. API Design & Consistency (14/15)

**Strengths:**
- Functional options pattern used consistently (DopplerProviderOption, LoaderOption, WatcherOption, MultiTenantOption)
- Provider interface is clean and minimal (Fetch, FetchProject, Name, Close)
- Generics eliminate runtime type assertions for callers
- Tag precedence (doppler > env > default) is well-defined
- SecretValue type prevents accidental logging of sensitive data

**Minor Inconsistency:**
- `LoadBootstrapFromEnv()` returns `(BootstrapConfig, error)` — but `LoadBootstrapWithChassis()` only returns `BootstrapConfig` (panics on error via `config.MustLoad`). The mixed error-handling strategy (return error vs panic) could surprise callers. Since this is a bootstrap function, the panic-on-failure behavior is defensible, but documenting the difference more prominently would help.

### 6. Maintainability (10/10)

- CHANGELOG.md is well-maintained with clear versioning history
- AGENTS.md provides multi-agent coordination guidelines
- Struct tags are documented in config.go header comments
- Code follows Go conventions (exported types, godoc comments)
- Dependencies are minimal and well-justified

### 7. Security (8/10)

**Strengths:**
- `SecretValue` type redacts in String(), MarshalJSON(), and MarshalText()
- `secval.ValidateJSON()` blocks prototype pollution attacks (__proto__, constructor)
- Fallback files written with 0600 permissions
- Doppler API response body capped at 1KB for error responses
- HTTP status code mapping prevents information leakage

**Opportunities:**
- **No secret scrubbing in error messages:** If a config value contains a secret and fails validation, the raw value could appear in the error message. Fields tagged `secret:"true"` should have their values scrubbed in validation error output.
- **EnvProvider reads all environment variables:** When no prefix is set, `EnvProvider.Fetch()` returns the entire environment. While this is documented behavior, it's a broad surface area. Consider requiring a prefix or explicitly documenting the security implications.

---

## Refactoring Recommendations (Prioritized)

### High Impact / Low Effort

1. **Unify boolean parsing** — Create a shared `parseBoolExtended(s string) (bool, error)` used by both feature_flags.go and loader.go. Eliminates inconsistent behavior.

2. **Add field context to error messages** — In `unmarshalConfig()`, include the field name and raw value in parse errors. This is a small change with outsized debugging benefit.

3. **Warn on unsupported field types** — Instead of silently skipping unknown types in `unmarshalConfig()`, append a warning to `ConfigMetadata.Warnings`.

### Medium Impact / Medium Effort

4. **Extract unmarshal logic** — Move the reflection-based `unmarshalConfig()` and supporting functions from loader.go into `unmarshal.go`. No API changes needed, just file reorganization.

5. **Generic slice parser** — Replace the three parallel slice-parsing blocks with a single `parseSlice[T]()` generic helper. Reduces ~70 lines to ~25 and makes adding new slice types trivial.

6. **Structured reload errors** — In MultiTenantLoader.ReloadProjects(), return a type that maps project codes to their individual errors, rather than joining all errors into one string.

### Lower Priority

7. **Split multitenant.go** — Extract MultiTenantWatcher into `multitenant_watcher.go`. Keeps the file under 300 lines.

8. **Document EnvProvider security model** — Add a doc comment explaining that prefix-less usage exposes the full environment, and recommend always setting a prefix in production.

9. **Scrub secrets in validation errors** — Check if a field is tagged `secret:"true"` before including its value in validation error messages.

---

## Architecture Notes

The codebase follows a clean layered architecture:

```
Providers (doppler, file, env)
    ↓
Loader[T] / MultiTenantLoader[E,P]  (unmarshal + validate)
    ↓
Watcher[T] / MultiTenantWatcher  (hot reload)
    ↓
FeatureFlags  (runtime toggles)
```

The chassis-go v5 integration is well-isolated in chassis.go and doppler.go, making it straightforward to evolve or replace. The bridge pattern in chassis.go is particularly clean — it re-exports what's needed without leaking chassis-go types into the public API.

The Provider interface is minimal and well-designed. Adding new providers (e.g., Vault, AWS SSM) would be straightforward.

---

## Conclusion

This is a well-engineered library with thoughtful API design and solid resilience patterns. The main areas for improvement are:

1. **Test coverage (33.9% → 65%+)** — the single highest-impact improvement
2. **Minor duplication in parsing logic** — easy wins for consistency
3. **Error message quality** — small changes for big debugging improvements

The code is clean, well-documented, and follows Go conventions consistently. The score of 78/100 reflects a production-ready library that would benefit from incremental refinement rather than any major restructuring.
