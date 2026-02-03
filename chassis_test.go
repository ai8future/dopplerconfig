package dopplerconfig

import (
	"context"
	"testing"

	"github.com/ai8future/chassis-go/call"
	"github.com/ai8future/chassis-go/testkit"
)

// EnvTagConfig tests that the env tag works as a fallback for doppler tag.
type EnvTagConfig struct {
	Port     int    `env:"PORT" default:"8080"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	Name     string `doppler:"SERVICE_NAME"` // doppler tag takes priority
}

func TestLoader_EnvTagFallback(t *testing.T) {
	values := map[string]string{
		"PORT":         "9090",
		"LOG_LEVEL":    "debug",
		"SERVICE_NAME": "myservice",
	}

	loader, _ := TestLoader[EnvTagConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.LogLevel)
	}
	if cfg.Name != "myservice" {
		t.Errorf("Name = %q, want \"myservice\"", cfg.Name)
	}
}

// DualTagConfig tests structs with both doppler and env tags.
type DualTagConfig struct {
	Port int `doppler:"PORT" env:"PORT" default:"3000"`
}

func TestLoader_DualTags(t *testing.T) {
	values := map[string]string{
		"PORT": "4000",
	}

	loader, _ := TestLoader[DualTagConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 4000 {
		t.Errorf("Port = %d, want 4000 (doppler tag should take priority)", cfg.Port)
	}
}

func TestLoader_EnvTagDefaults(t *testing.T) {
	values := map[string]string{}

	loader, _ := TestLoader[EnvTagConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want default 8080", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want default \"info\"", cfg.LogLevel)
	}
}

func TestLoadBootstrapWithChassis(t *testing.T) {
	testkit.SetEnv(t, map[string]string{
		"DOPPLER_TOKEN":          "dp.test.xxx",
		"DOPPLER_PROJECT":        "myproject",
		"DOPPLER_CONFIG":         "dev",
		"DOPPLER_FALLBACK_PATH":  "/tmp/fallback.json",
		"DOPPLER_WATCH_ENABLED":  "true",
		"DOPPLER_FAILURE_POLICY": "fail",
	})

	cfg := LoadBootstrapWithChassis()

	if cfg.Token != "dp.test.xxx" {
		t.Errorf("Token = %q, want \"dp.test.xxx\"", cfg.Token)
	}
	if cfg.Project != "myproject" {
		t.Errorf("Project = %q, want \"myproject\"", cfg.Project)
	}
	if cfg.Config != "dev" {
		t.Errorf("Config = %q, want \"dev\"", cfg.Config)
	}
	if cfg.FallbackPath != "/tmp/fallback.json" {
		t.Errorf("FallbackPath = %q, want \"/tmp/fallback.json\"", cfg.FallbackPath)
	}
	if !cfg.WatchEnabled {
		t.Error("WatchEnabled = false, want true")
	}
	if cfg.FailurePolicy != FailurePolicyFail {
		t.Errorf("FailurePolicy = %d, want FailurePolicyFail", cfg.FailurePolicy)
	}
}

func TestLoadBootstrapWithChassis_Defaults(t *testing.T) {
	testkit.SetEnv(t, map[string]string{})

	cfg := LoadBootstrapWithChassis()

	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
	if cfg.FailurePolicy != FailurePolicyFallback {
		t.Errorf("FailurePolicy = %d, want FailurePolicyFallback", cfg.FailurePolicy)
	}
	if !cfg.WatchEnabled == true {
		// WatchEnabled should be false by default (empty string != "true")
	}
}

func TestValidateConfig_Bridge(t *testing.T) {
	type Config struct {
		Port  int    `validate:"port"`
		Email string `validate:"email"`
	}

	// Valid config
	cfg := Config{Port: 8080, Email: "test@example.com"}
	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("ValidateConfig returned error for valid config: %v", err)
	}

	// Invalid config
	bad := Config{Port: 99999, Email: "invalid"}
	if err := ValidateConfig(bad); err == nil {
		t.Error("ValidateConfig should return error for invalid config")
	}
}

func TestDopplerProvider_CircuitState(t *testing.T) {
	provider, err := NewDopplerProvider("test-token", "proj", "dev")
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}

	state := provider.CircuitState()
	if state != call.StateClosed {
		t.Errorf("CircuitState = %d, want StateClosed", state)
	}
}

func TestDopplerProvider_WithHTTPClient_NilBreaker(t *testing.T) {
	provider, err := NewDopplerProvider("test-token", "proj", "dev",
		WithHTTPClient(nil),
	)
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}

	// With a custom HTTP client, CircuitState should return StateClosed (no breaker)
	state := provider.CircuitState()
	if state != call.StateClosed {
		t.Errorf("CircuitState = %d, want StateClosed (no breaker)", state)
	}
}

func TestDopplerProvider_WithLogger(t *testing.T) {
	logger := testkit.NewLogger(t)

	provider, err := NewDopplerProvider("test-token", "proj", "dev",
		WithProviderLogger(logger),
	)
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}
	defer provider.Close()

	// Provider should have accepted the logger without error
	if provider.logger == nil {
		t.Error("logger should not be nil after WithProviderLogger")
	}
}

func TestLoader_WithLogger(t *testing.T) {
	logger := testkit.NewLogger(t)

	values := map[string]string{
		"DATABASE_URL": "postgres://localhost/test",
	}

	mock := NewMockProvider(values)
	loader := NewLoaderWithProvider[TestConfig](mock, nil,
		WithLoaderLogger[TestConfig](logger),
	)

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Database.URL != "postgres://localhost/test" {
		t.Errorf("Database.URL = %q, want \"postgres://localhost/test\"", cfg.Database.URL)
	}
}

func TestCircuitStateConstants(t *testing.T) {
	// Verify re-exported constants match chassis-go values
	if CircuitStateClosed != call.StateClosed {
		t.Error("CircuitStateClosed mismatch")
	}
	if CircuitStateOpen != call.StateOpen {
		t.Error("CircuitStateOpen mismatch")
	}
	if CircuitStateHalfOpen != call.StateHalfOpen {
		t.Error("CircuitStateHalfOpen mismatch")
	}
}
