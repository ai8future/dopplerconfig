# dopplerconfig

A Go library for type-safe configuration management using [Doppler](https://www.doppler.com/) as the primary secrets and config source. Provides automatic struct mapping, hot-reload, fallback providers, validation, feature flags, and multi-tenant config patterns — all with production-grade resilience via [chassis-go](https://github.com/ai8future/chassis-go).

## Features

- **Generic, type-safe loaders** — `Loader[T]` maps Doppler secrets directly into typed Go structs via struct tags
- **Multi-tenant support** — `MultiTenantLoader[E, P]` handles two-tier config (shared environment + per-project)
- **Hot-reload** — `Watcher` polls Doppler for changes with ETag-based caching and fires callbacks on config updates
- **Resilient API calls** — retries with exponential backoff and circuit breaking via chassis-go's `call.Client`
- **Fallback chain** — `FileProvider` (local JSON) and `EnvProvider` (OS environment) as fallbacks when Doppler is unavailable
- **Validation engine** — 8 built-in validators (`min`, `max`, `port`, `url`, `email`, `host`, `oneof`, `regex`) via struct tags
- **Feature flags** — cached flag evaluation with percentage-based rollouts and allow/block lists
- **Secret redaction** — `SecretValue` type that returns `[REDACTED]` in logs and JSON serialization
- **Security validation** — all JSON payloads screened for prototype pollution and excessive nesting via `secval`
- **Test utilities** — `MockProvider`, `RecordingProvider`, `TestLoader[T]` for easy unit testing

## Installation

```bash
go get github.com/ai8future/dopplerconfig
```

Requires **Go 1.25.5+** and chassis-go v9.

## Quick Start

### Define a config struct

```go
type AppConfig struct {
    Port     int           `doppler:"SERVER_PORT" default:"8080" validate:"port"`
    DBUrl    string        `doppler:"DATABASE_URL" required:"true" validate:"url"`
    Password SecretValue   `doppler:"DB_PASSWORD" secret:"true"`
    LogLevel string        `doppler:"LOG_LEVEL" default:"info" validate:"oneof=debug|info|warn|error"`
    Hosts    []string      `doppler:"ALLOWED_HOSTS"`
    Timeout  time.Duration `doppler:"REQUEST_TIMEOUT" default:"30s"`
}
```

### Load configuration

```go
bootstrap := dopplerconfig.LoadBootstrapFromEnv()

loader, err := dopplerconfig.NewLoader[AppConfig](bootstrap)
if err != nil {
    log.Fatal(err)
}
defer loader.Close()

cfg, err := loader.Load(context.Background())
if err != nil {
    log.Fatal(err)
}

fmt.Println("Port:", cfg.Port)
fmt.Println("Password:", cfg.Password) // prints "[REDACTED]"
```

### Watch for changes

```go
loader.OnChange(func(old, new *AppConfig) {
    log.Println("Config changed, new port:", new.Port)
})

stop := dopplerconfig.Watch(ctx, loader,
    dopplerconfig.WithWatchInterval[AppConfig](30 * time.Second),
)
defer stop()
```

### Multi-tenant configuration

```go
type EnvConfig struct {
    MaxPoolSize int `doppler:"MAX_POOL_SIZE" default:"10"`
}

type ProjectConfig struct {
    APIKey   string `doppler:"API_KEY" required:"true"`
    Endpoint string `doppler:"ENDPOINT" validate:"url"`
}

mtLoader, err := dopplerconfig.NewMultiTenantLoader[EnvConfig, ProjectConfig](bootstrap)
if err != nil {
    log.Fatal(err)
}

env, _ := mtLoader.LoadEnv(ctx)
projects, _ := mtLoader.LoadAllProjects(ctx, []string{"proj-a", "proj-b"})
```

## Struct Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `doppler` | Doppler secret key name | `doppler:"DATABASE_URL"` |
| `env` | Fallback key name (chassis-go compat) | `env:"DATABASE_URL"` |
| `default` | Default value if key is absent | `default:"8080"` |
| `required` | Fail if key is missing or empty | `required:"true"` |
| `secret` | Marks sensitive fields | `secret:"true"` |
| `validate` | Validation rules (comma-separated) | `validate:"port,min=1000"` |
| `description` | Documentation for the field | `description:"gRPC port"` |

**Tag priority:** `doppler` > `env` > field name.

## Validation Rules

| Rule | Syntax | Description |
|------|--------|-------------|
| `min` | `validate:"min=10"` | Minimum value (int) or length (string) |
| `max` | `validate:"max=100"` | Maximum value or length |
| `port` | `validate:"port"` | Valid port number (1-65535) |
| `url` | `validate:"url"` | Parseable URI |
| `host` | `validate:"host"` | RFC 1123 hostname or IP, optional port |
| `email` | `validate:"email"` | Valid email format |
| `oneof` | `validate:"oneof=a\|b\|c"` | Must match one of the pipe-delimited values |
| `regex` | `validate:"regex=^[a-z]+$"` | Must match the regex pattern |

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `DOPPLER_TOKEN` | Doppler service or personal token | *(required if no fallback)* |
| `DOPPLER_PROJECT` | Doppler project name | *(optional with service tokens)* |
| `DOPPLER_CONFIG` | Config name (dev/stg/prd) | *(optional with service tokens)* |
| `DOPPLER_FALLBACK_PATH` | Path to local JSON fallback file | *(none)* |
| `DOPPLER_WATCH_ENABLED` | Enable hot-reload polling | `false` |
| `DOPPLER_FAILURE_POLICY` | `fail`, `fallback`, or `warn` | `fallback` |

## Failure Policies

| Policy | Behavior |
|--------|----------|
| `fail` | Return error immediately if Doppler is unavailable (strict) |
| `fallback` | Use fallback file/env if Doppler is unavailable (default) |
| `warn` | Log warning and use struct `default` tags only (lenient) |

## Provider Interface

All config sources implement the `Provider` interface:

```go
type Provider interface {
    Fetch(ctx context.Context) (map[string]string, error)
    FetchProject(ctx context.Context, project, config string) (map[string]string, error)
    Name() string
    Close() error
}
```

Built-in providers:

| Provider | Description |
|----------|-------------|
| `DopplerProvider` | Live Doppler API with retries, circuit breaking, and ETag caching |
| `FileProvider` | Local JSON file (supports nested JSON with automatic flattening) |
| `EnvProvider` | OS environment variables with optional prefix |
| `MockProvider` | In-memory provider for tests |
| `RecordingProvider` | Decorator that records all fetch calls for test assertions |

## Resilience

`DopplerProvider` uses chassis-go's `call.Client` under the hood:

- **Retries:** 3 attempts with exponential backoff (1s, 2s, 4s)
- **Circuit breaker:** Opens after 5 consecutive failures, stays open for 30 seconds
- **ETag caching:** `304 Not Modified` responses return cached values with zero JSON parsing
- **Timeout:** 30-second per-request timeout
- **Health check:** `HealthCheck(provider)` returns a function suitable for health check endpoints

```go
provider, _ := dopplerconfig.NewDopplerProvider(token, project, config,
    dopplerconfig.WithCallOptions(
        call.WithTimeout(15 * time.Second),
        call.WithRetry(5, 2 * time.Second),
    ),
)

// Check circuit state
state := provider.CircuitState() // call.StateClosed, StateOpen, or StateHalfOpen
```

## Feature Flags

```go
flags := dopplerconfig.NewFeatureFlags(secretValues, "FF_")

if flags.IsEnabled("DARK_MODE") {
    // Feature is on
}

maxRetries := flags.GetInt("MAX_RETRIES", 3)
```

Percentage-based rollouts:

```go
rollout := &dopplerconfig.RolloutConfig{
    Percentage: 25,
    AllowList:  []string{"beta-user-1"},
    BlockList:  []string{"excluded-user"},
}

if rollout.ShouldEnable(userID, hashFunc) {
    // Enabled for ~25% of users + allow list
}
```

## Testing

```go
func TestMyService(t *testing.T) {
    loader, mock := dopplerconfig.TestLoader[AppConfig](map[string]string{
        "SERVER_PORT":  "9090",
        "DATABASE_URL": "postgres://localhost/test",
        "DB_PASSWORD":  "secret",
    })

    cfg, err := loader.Load(context.Background())
    if err != nil {
        t.Fatal(err)
    }

    // Update config mid-test
    mock.SetValue("SERVER_PORT", "9091")
    cfg, _ = loader.Reload(context.Background())
}
```

## Architecture

```
Consumer Service
      │
      ▼
Loader[T] / MultiTenantLoader[E,P]     ← Generic loaders with callbacks
      │
      ▼
Provider interface                       ← Pluggable data sources
   ┌──┴──┐
   ▼     ▼
Doppler  File/Env                        ← Primary + fallback
   │
   ▼
chassis-go call.Client                   ← Retries + circuit breaker
   │
   ▼
Doppler REST API (v3)
```

## Supported Types

The reflection-based unmarshaller handles:

- Primitives: `string`, `int`, `int8`–`int64`, `uint`–`uint64`, `float32`, `float64`, `bool`
- `time.Duration` (e.g., `"30s"`, `"5m"`)
- `SecretValue` (redacted in logs/JSON)
- Slices: `[]string`, `[]int`, `[]bool` (comma-separated values)
- Nested and embedded structs

## License

Private — internal infrastructure module.
