package dopplerconfig

import (
	"os"
	"time"
)

// BootstrapConfig contains the minimal configuration needed to connect to Doppler.
// These values come from environment variables and are used to bootstrap the full config.
type BootstrapConfig struct {
	// Token is the Doppler service or personal token (DOPPLER_TOKEN).
	// Required unless FallbackPath is set.
	Token string

	// Project is the Doppler project name (DOPPLER_PROJECT).
	// Optional when using service tokens (which encode the project).
	Project string

	// Config is the Doppler config name (DOPPLER_CONFIG), e.g., "dev", "stg", "prd".
	// Optional when using service tokens (which encode the config).
	Config string

	// FallbackPath is the path to a local JSON file for fallback (DOPPLER_FALLBACK_PATH).
	// If Doppler is unavailable and this is set, the file will be used.
	FallbackPath string

	// WatchEnabled enables hot reload of configuration (DOPPLER_WATCH_ENABLED).
	WatchEnabled bool

	// WatchInterval is how often to poll for changes when watching.
	// Defaults to 30 seconds if not set.
	WatchInterval time.Duration

	// FailurePolicy controls behavior when Doppler is unavailable.
	FailurePolicy FailurePolicy
}

// FailurePolicy defines how to handle Doppler unavailability.
type FailurePolicy int

const (
	// FailurePolicyFail causes startup to fail if Doppler is unavailable.
	FailurePolicyFail FailurePolicy = iota

	// FailurePolicyFallback uses the fallback file if Doppler is unavailable.
	FailurePolicyFallback

	// FailurePolicyWarn logs a warning and uses fallback or defaults.
	FailurePolicyWarn
)

// LoadBootstrapFromEnv creates a BootstrapConfig from environment variables.
func LoadBootstrapFromEnv() BootstrapConfig {
	cfg := BootstrapConfig{
		Token:         os.Getenv("DOPPLER_TOKEN"),
		Project:       os.Getenv("DOPPLER_PROJECT"),
		Config:        os.Getenv("DOPPLER_CONFIG"),
		FallbackPath:  os.Getenv("DOPPLER_FALLBACK_PATH"),
		WatchEnabled:  os.Getenv("DOPPLER_WATCH_ENABLED") == "true",
		WatchInterval: 30 * time.Second,
		FailurePolicy: FailurePolicyFallback,
	}

	// Parse failure policy
	switch os.Getenv("DOPPLER_FAILURE_POLICY") {
	case "fail":
		cfg.FailurePolicy = FailurePolicyFail
	case "warn":
		cfg.FailurePolicy = FailurePolicyWarn
	default:
		cfg.FailurePolicy = FailurePolicyFallback
	}

	return cfg
}

// IsEnabled returns true if Doppler integration is enabled (token is set).
func (b BootstrapConfig) IsEnabled() bool {
	return b.Token != ""
}

// HasFallback returns true if a fallback path is configured.
func (b BootstrapConfig) HasFallback() bool {
	return b.FallbackPath != ""
}

// Struct tag constants for config mapping.
const (
	// TagDoppler is the struct tag for mapping to Doppler key names.
	// Example: `doppler:"REDIS_PASSWORD"`
	TagDoppler = "doppler"

	// TagDefault is the struct tag for default values.
	// Example: `default:"50051"`
	TagDefault = "default"

	// TagSecret marks a field as containing sensitive data.
	// When true, the value will be redacted in logs.
	// Example: `secret:"true"`
	TagSecret = "secret"

	// TagRequired marks a field as required.
	// The loader will return an error if the value is missing.
	// Example: `required:"true"`
	TagRequired = "required"

	// TagDescription provides documentation for the field.
	// Example: `description:"gRPC server port"`
	TagDescription = "description"
)

// ConfigMetadata contains information about a loaded configuration.
type ConfigMetadata struct {
	// Source indicates where the config was loaded from.
	Source string

	// LoadedAt is when the config was loaded.
	LoadedAt time.Time

	// Project is the Doppler project name (if applicable).
	Project string

	// Config is the Doppler config name (if applicable).
	Config string

	// ETag is the version identifier from Doppler (for caching).
	ETag string

	// KeyCount is the number of keys loaded.
	KeyCount int

	// Warnings contains any non-fatal issues encountered during loading.
	Warnings []string
}

// SecretValue wraps a string value that should be redacted in logs.
type SecretValue struct {
	value string
}

// NewSecretValue creates a new SecretValue.
func NewSecretValue(v string) SecretValue {
	return SecretValue{value: v}
}

// Value returns the underlying secret value.
func (s SecretValue) Value() string {
	return s.value
}

// String returns a redacted representation for logging.
func (s SecretValue) String() string {
	if s.value == "" {
		return "[empty]"
	}
	return "[REDACTED]"
}

// MarshalJSON redacts the value in JSON output.
func (s SecretValue) MarshalJSON() ([]byte, error) {
	return []byte(`"[REDACTED]"`), nil
}
