# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2026-02-03

### Breaking
- **Go 1.25.5 required** (bumped from 1.22.0) due to chassis-go dependency
- `chassis-go` is now a compile-time dependency

### Added
- **chassis-go integration** for resilient Doppler API calls
  - `DopplerProvider` now uses `call.Client` with automatic retries (3 attempts, exponential backoff) and circuit breaking (opens after 5 consecutive failures)
  - `WithCallOptions()` for custom call.Client configuration
  - `WithProviderLogger()` and `WithLoaderLogger()` for explicit `*slog.Logger` injection
  - `CircuitState()` method on `DopplerProvider` for health check integration
- `env` struct tag as fallback for `doppler` tag, enabling single structs to work with both `dopplerconfig` and `chassis-go/config.MustLoad`
- `LoadBootstrapWithChassis()` to load bootstrap config via `config.MustLoad`
- `ValidateConfig()` bridge function for validating chassis-go-loaded structs
- Re-exported circuit breaker constants (`CircuitStateClosed`, `CircuitStateOpen`, `CircuitStateHalfOpen`) and `ErrCircuitOpen`
- `LoaderOption[T]` type for configuring loaders with options like `WithLoaderLogger`
- New `chassis.go` file consolidating all chassis-go bridge functions
- Comprehensive test coverage for all new features (`chassis_test.go`)

### Changed
- `DopplerProvider.Close()` no longer calls `CloseIdleConnections()` (managed by `call.Client`)
- `NewLoader` now accepts variadic `LoaderOption[T]` parameters
- `NewLoaderWithProvider` now accepts variadic `LoaderOption[T]` parameters
- Error body read limited to 1KB in Doppler API error responses

## [1.0.0] - 2026-01-18

### Changed
- Promoted to version 1.0.0 stable release
- Added AGENTS.md for multi-agent coordination
- Updated .gitignore with comprehensive defaults

## [0.1.2] - 2026-01-18

### Added
- Package documentation for Doppler Secret Notes best practices
  - When to add notes (format requirements, pairings, permissions)
  - When not to add notes (obvious information)
  - CLI command example for setting notes

## [0.1.1] - 2026-01-18

### Fixed
- Race conditions in `FeatureFlags` - all methods now properly acquire locks
- Use `errors.As` in `IsDopplerError` for proper error chain unwrapping
- Return `DopplerError` type from API errors (not plain error)
- Remove unused cache from `FileProvider`
- Replace custom `joinStrings` with `strings.Join`
- Add error logging to `MultiTenantWatcher.run()`
- Set go.mod to Go 1.22.0 for broader compatibility

## [0.1.0] - 2026-01-18

### Added
- Initial release of dopplerconfig shared module
- Core `Provider` interface for config sources (Doppler, File, Mock)
- `DopplerProvider` for direct Doppler API integration
- `FileProvider` for local fallback support
- `MockProvider` for testing
- Generic `Loader[T]` for single-config pattern (Airborne)
- `MultiTenantLoader[E, P]` for env+project pattern (Solstice)
- Struct tag mapping: `doppler`, `default`, `secret`
- Hot reload support via `Watcher`
- Feature flag helpers
- Validation utilities
