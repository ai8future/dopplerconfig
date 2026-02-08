package dopplerconfig

import (
	"context"
	"time"

	chassis "github.com/ai8future/chassis-go/v5"
	"github.com/ai8future/chassis-go/v5/call"
	"github.com/ai8future/chassis-go/v5/config"
)

// bootstrapEnv is the struct used by LoadBootstrapWithChassis to load
// bootstrap configuration via chassis-go's config.MustLoad.
type bootstrapEnv struct {
	Token         string `env:"DOPPLER_TOKEN" required:"false"`
	Project       string `env:"DOPPLER_PROJECT" required:"false"`
	Config        string `env:"DOPPLER_CONFIG" required:"false"`
	FallbackPath  string `env:"DOPPLER_FALLBACK_PATH" required:"false"`
	WatchEnabled  string `env:"DOPPLER_WATCH_ENABLED" required:"false"`
	FailurePolicy string `env:"DOPPLER_FAILURE_POLICY" default:"fallback" required:"false"`
}

// LoadBootstrapWithChassis loads BootstrapConfig using chassis-go's
// config.MustLoad. This provides a unified env-loading pattern for teams
// already using chassis-go for their application config.
//
// Unlike LoadBootstrapFromEnv, this uses chassis-go's struct tag conventions
// (env tag, required tag, default tag) for consistency with other
// chassis-go-loaded configs.
func LoadBootstrapWithChassis() BootstrapConfig {
	raw := config.MustLoad[bootstrapEnv]()

	cfg := BootstrapConfig{
		Token:         raw.Token,
		Project:       raw.Project,
		Config:        raw.Config,
		FallbackPath:  raw.FallbackPath,
		WatchEnabled:  raw.WatchEnabled == "true",
		WatchInterval: 30 * time.Second,
		FailurePolicy: FailurePolicyFallback,
	}

	switch raw.FailurePolicy {
	case "fail":
		cfg.FailurePolicy = FailurePolicyFail
	case "warn":
		cfg.FailurePolicy = FailurePolicyWarn
	}

	return cfg
}

// ValidateConfig validates a struct populated by either dopplerconfig or
// chassis-go's config.MustLoad. It checks validate struct tags and calls
// custom Validate() methods if present.
//
// This bridges dopplerconfig's rich validation (min, max, port, url, host,
// email, oneof, regex) to structs loaded by any mechanism:
//
//	cfg := config.MustLoad[AppConfig]()  // chassis-go
//	if err := dopplerconfig.ValidateConfig(&cfg); err != nil {
//	    log.Fatal(err)
//	}
func ValidateConfig(cfg any) error {
	return Validate(cfg)
}

// Re-export chassis-go call types for convenience, so consumers don't need
// to import the call package directly for common operations like checking
// circuit breaker state.

// CircuitStateClosed indicates the circuit breaker is allowing all requests.
const CircuitStateClosed = call.StateClosed

// CircuitStateOpen indicates the circuit breaker is rejecting all requests.
const CircuitStateOpen = call.StateOpen

// CircuitStateHalfOpen indicates the circuit breaker is allowing a single probe request.
const CircuitStateHalfOpen = call.StateHalfOpen

// ErrCircuitOpen is returned when the Doppler API circuit breaker is open.
var ErrCircuitOpen = call.ErrCircuitOpen

// ChassisVersion is the version of the chassis-go toolkit in use.
// Useful for diagnostic logging or health endpoints.
var ChassisVersion = chassis.Version

// RequireChassisVersion calls chassis.RequireMajor(5) so that consuming
// services can satisfy the version gate without importing chassis-go directly.
// Must be called before any chassis-go API (config.MustLoad, call.New, work.Map, etc.).
func RequireChassisVersion() {
	chassis.RequireMajor(5)
}

// HealthCheck returns a health check function compatible with chassis-go's
// health.Check type. It checks the DopplerProvider's circuit breaker state
// (fast path) and, if the circuit is closed or half-open, attempts a Fetch
// (slow path) to verify end-to-end connectivity.
func HealthCheck(provider *DopplerProvider) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if provider.CircuitState() == call.StateOpen {
			return call.ErrCircuitOpen
		}
		_, err := provider.Fetch(ctx)
		return err
	}
}
