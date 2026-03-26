# dopplerconfig -- Product Overview

## What This Product Is

dopplerconfig is an internal Go library that serves as the centralized configuration management layer for all backend services in the organization. It bridges the gap between Doppler (a cloud-hosted secrets and configuration management SaaS) and the organization's Go microservices, providing a type-safe, resilient, and operationally mature way to load, validate, watch, and distribute configuration and secrets at runtime.

This is not a standalone application. It is a shared infrastructure library -- part of the `infra_suite` family of internal tooling -- consumed by multiple production services.

---

## Why This Product Exists

### The Core Problem

Microservices need configuration: database URLs, API keys, feature flags, port numbers, service endpoints, and secrets. In a production environment, managing these values across multiple services, environments (dev/staging/production), and tenants introduces several business-critical challenges:

1. **Secrets sprawl and leakage risk.** Secrets stored in environment variables, config files, or code repositories are difficult to rotate, audit, and protect. A centralized secrets manager (Doppler) solves storage, but each service still needs a reliable, secure way to consume those secrets.

2. **Configuration drift across environments.** Without a structured loading mechanism, services may silently run with wrong or stale configurations. Bad config in production causes outages.

3. **Multi-tenant configuration complexity.** The organization operates multi-tenant systems (referred to internally as the "Solstice pattern") where a shared environment-level config coexists with per-project/per-tenant configs. Managing this two-tier hierarchy manually is error-prone.

4. **Resilience during infrastructure failures.** If the Doppler API goes down, services must not crash on startup or fail silently. They need fallback mechanisms and circuit breaking to maintain availability.

5. **Runtime configuration changes.** Some configuration changes (feature flags, rate limits, tenant onboarding) should take effect without redeploying services. Hot-reload support is essential for operational agility.

6. **Developer productivity.** Without a shared library, each service team would build its own config loading, validation, and fallback logic -- duplicating effort and introducing inconsistencies.

### The Business Decision

Rather than letting each service implement its own Doppler integration, the organization invested in a single shared library that encapsulates all configuration management concerns. This ensures consistent behavior, security posture, and operational patterns across the entire service fleet.

---

## Business Goals

### 1. Eliminate Secret Exposure in Logs, APIs, and Error Messages

The `SecretValue` type is a first-class concept. Any field tagged `secret:"true"` is automatically wrapped so that:
- `fmt.Println(cfg.Password)` prints `[REDACTED]`, not the actual password.
- JSON serialization (e.g., in API responses or structured logs) emits `"[REDACTED]"`.
- The actual value is only accessible via an explicit `.Value()` method call.

This is a deliberate business control: it makes accidental secret exposure in logs, debug output, and error responses structurally impossible rather than relying on developer discipline.

### 2. Guarantee Configuration Correctness at Startup

The validation engine runs 8 built-in validators (`min`, `max`, `port`, `url`, `email`, `host`, `oneof`, `regex`) against configuration values before a service begins handling traffic. This catches:
- Ports outside the valid range (1-65535)
- Malformed database URLs that would cause runtime connection failures
- Invalid email addresses in notification configs
- Log levels or mode strings that don't match expected values

The `required:"true"` tag further ensures that critical fields like `DATABASE_URL` cannot be silently empty. Services fail fast at startup with clear error messages rather than experiencing mysterious runtime failures.

### 3. Ensure Zero-Downtime During Doppler Outages

The fallback chain provides three levels of resilience:

- **Primary:** Live Doppler API with retries (3 attempts, exponential backoff) and circuit breaking (opens after 5 consecutive failures, stays open 30 seconds).
- **Secondary:** Local JSON fallback file that can be pre-seeded or cached from a previous successful Doppler fetch.
- **Tertiary:** OS environment variables (via `EnvProvider`) or struct-level `default` tags.

The `FailurePolicy` setting gives operations teams control over how aggressively to enforce Doppler availability:
- `fail` -- strict mode for production, where stale config is worse than no service.
- `fallback` -- default mode, gracefully degrades to local config.
- `warn` -- lenient mode for development, logs a warning and proceeds with defaults only.

### 4. Support Multi-Tenant Configuration Without Complexity

The `MultiTenantLoader[E, P]` directly addresses the organization's multi-tenant architecture (the "Solstice pattern"). It provides:

- **Environment config (E):** Shared settings like connection pool sizes, global rate limits, and infrastructure endpoints. Loaded once.
- **Project/tenant config (P):** Per-tenant API keys, endpoints, and feature settings. Loaded per tenant, in parallel (bounded to 5 concurrent workers to avoid overwhelming Doppler).

The `ReloadDiff` mechanism tracks which tenants were added, removed, or unchanged during a reload cycle, enabling services to respond intelligently to tenant onboarding/offboarding events at runtime.

This pattern allows the organization to onboard new tenants by adding Doppler configs without redeploying services.

### 5. Enable Feature Flags and Gradual Rollouts

The `FeatureFlags` system and `RolloutConfig` provide lightweight feature management directly from Doppler config values:

- **Boolean flags:** `FEATURE_DARK_MODE=true` enables a feature globally.
- **Percentage rollouts:** A feature can be enabled for a specific percentage of users (e.g., 25%) with consistent hashing so the same user always gets the same experience.
- **Allow/block lists:** Specific users can be force-included or force-excluded from a feature, regardless of the rollout percentage.
- **Common operational flags:** Pre-defined patterns for `MAINTENANCE_MODE`, `DEBUG_LOGGING`, `RATE_LIMIT_BYPASS`, and `DOPPLER_ENABLED` (for gradual migration).

This eliminates the need for a separate feature flag service (like LaunchDarkly) for simple use cases, keeping the operational surface area small.

### 6. Enable Hot-Reload for Operational Agility

The `Watcher` polls Doppler at configurable intervals (default 30 seconds) and fires registered callbacks when configuration changes are detected. Key behaviors:

- **ETag-based caching:** Doppler returns `304 Not Modified` when config hasn't changed, eliminating unnecessary JSON parsing and reducing API load.
- **Change callbacks:** Services can react to specific changes (e.g., update a connection pool size, toggle a feature) without full restarts.
- **Failure tolerance:** The watcher tracks consecutive failures and can auto-stop after a configurable maximum, preventing runaway error logging.
- **Multi-tenant watching:** `MultiTenantWatcher` reloads both environment and all project configs on each poll cycle.

This means operations teams can change a database connection limit, enable a feature flag, or rotate a secret in Doppler and have the change propagate to all running service instances within 30 seconds -- without any deployment.

### 7. Protect Against Malicious Configuration Payloads

All JSON payloads -- whether from Doppler API responses or local fallback files -- are screened through chassis-go's `secval` (security validation) package before being parsed. This detects:

- **Prototype pollution attacks:** Keys like `__proto__`, `constructor`, or `prototype` that could exploit JavaScript-style vulnerabilities in downstream systems.
- **Excessive nesting depth:** Deeply nested JSON structures that could cause stack overflows or excessive memory allocation.

This is a defense-in-depth measure: even if Doppler itself were compromised, or a developer accidentally created a malicious fallback file, the library would reject the payload before it enters the application.

### 8. Integrate Seamlessly with the chassis-go Ecosystem

The library is tightly integrated with `chassis-go` (the organization's internal service framework, currently at v10):

- **Resilient HTTP calls** use `chassis-go/call.Client` for retries, exponential backoff, and circuit breaking.
- **Parallel loading** uses `chassis-go/work.Map` for bounded concurrent operations.
- **Error mapping** converts Doppler HTTP errors to `chassis-go/errors.ServiceError` types with proper HTTP/gRPC status codes and RFC 9457 problem details.
- **Health checks** expose a function compatible with chassis-go's health check endpoints, checking circuit breaker state and end-to-end connectivity.
- **Dual struct tags** (`doppler` and `env`) allow a single config struct to be loaded by either `dopplerconfig` or `chassis-go/config.MustLoad`, enabling gradual migration and flexibility.
- **Bootstrap bridging** via `LoadBootstrapWithChassis()` uses chassis-go's own config loading for the bootstrap config, maintaining a unified pattern.

### 9. Reduce Developer Friction with Type-Safe Generics

The `Loader[T]` and `MultiTenantLoader[E, P]` use Go generics so that:

- Developers define their config as a plain Go struct with tags.
- The library handles all reflection, type conversion, default values, and validation.
- The returned config is a fully typed struct -- no type assertions, no `interface{}`, no `map[string]string` at the call site.
- Supported types cover real-world needs: primitives, `time.Duration`, `SecretValue`, slices (comma-separated), and nested/embedded structs.

### 10. Provide First-Class Test Support

The library includes dedicated testing utilities so that services can write config-dependent tests without mocking HTTP servers or setting up Doppler:

- `MockProvider` -- in-memory provider with `SetValue()`, `SetValues()`, `SetError()`, and `Clear()` for controlling config in tests.
- `TestLoader[T]` -- one-liner to create a loader with a mock provider.
- `TestLoaderWithConfig[T]` -- creates a loader and immediately loads config.
- `RecordingProvider` -- decorator that records all fetch calls for asserting that the loader called the provider correctly.
- `TestBootstrap()` -- sensible bootstrap defaults for test scenarios.

---

## Business Logic Summary

| Concern | How It Is Handled |
|---|---|
| Secret management | Doppler as primary source; `SecretValue` type prevents accidental exposure |
| Configuration correctness | 8 validators + `required` tag enforce constraints at load time |
| Resilience | Retry with backoff, circuit breaker, fallback chain, configurable failure policy |
| Multi-tenancy | Two-tier loader with parallel per-tenant loading and reload diffing |
| Feature flags | Boolean flags, percentage rollouts, allow/block lists from Doppler values |
| Hot-reload | Polling watcher with ETag caching, change callbacks, failure limits |
| Security | JSON payload validation against prototype pollution and nesting attacks |
| Ecosystem integration | chassis-go v10 for HTTP resilience, parallel work, error types, health checks |
| Developer experience | Go generics, struct tags, type-safe configs, comprehensive test utilities |
| Operational control | Failure policies (`fail`/`fallback`/`warn`), circuit breaker state exposure, health check endpoints |

---

## Internal Service Patterns

The codebase references two internal service architectures by name:

- **Airborne pattern:** Single-config services that use `Loader[T]` to load one configuration struct from one Doppler project/config.
- **Solstice pattern:** Multi-tenant services that use `MultiTenantLoader[E, P]` to load a shared environment config plus per-tenant project configs from multiple Doppler project/config combinations.

These are the two canonical configuration patterns in the organization, and dopplerconfig provides purpose-built support for both.

---

## Lifecycle and Maturity

- **Current version:** 1.1.8
- **First release:** January 2026 (v0.1.0)
- **Stable since:** January 2026 (v1.0.0)
- **Major upgrade:** February 2026 (v2.0.0) added chassis-go integration
- **Dependency:** chassis-go v10.0.0 (tracked in VERSION.chassis)
- **License:** Private/internal -- not published as open source
