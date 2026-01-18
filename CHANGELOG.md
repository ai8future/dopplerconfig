# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
