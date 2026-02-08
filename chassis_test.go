package dopplerconfig

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	chassis "github.com/ai8future/chassis-go/v5"
	"github.com/ai8future/chassis-go/v5/call"
	"github.com/ai8future/chassis-go/v5/testkit"
)

func TestMain(m *testing.M) {
	chassis.RequireMajor(5)
	os.Exit(m.Run())
}

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
	if cfg.WatchEnabled {
		t.Error("WatchEnabled should be false by default")
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

func TestRequireChassisVersion(t *testing.T) {
	// RequireChassisVersion should not panic — TestMain already called RequireMajor(5),
	// and calling it again is safe (idempotent).
	RequireChassisVersion()
}

func TestChassisVersion(t *testing.T) {
	if ChassisVersion == "" {
		t.Error("ChassisVersion should not be empty")
	}
	// Should be a semver starting with "5."
	if ChassisVersion[0] != '5' {
		t.Errorf("ChassisVersion = %q, want major version 5", ChassisVersion)
	}
}

func TestHealthCheck_Healthy(t *testing.T) {
	// Use a mock HTTP server that returns valid Doppler JSON
	srv := newTestDopplerServer(t, `{"secrets":{"KEY":{"raw":"value"}}}`, 200)
	defer srv.Close()

	provider, err := NewDopplerProvider("test-token", "proj", "dev",
		WithAPIURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}

	check := HealthCheck(provider)
	if err := check(context.Background()); err != nil {
		t.Errorf("HealthCheck returned error for healthy provider: %v", err)
	}
}

func TestHealthCheck_CircuitOpen(t *testing.T) {
	provider, err := NewDopplerProvider("test-token", "proj", "dev")
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}

	// Force circuit open by tripping the breaker
	for i := 0; i < DefaultBreakerThreshold+1; i++ {
		provider.breaker.Record(false)
	}

	check := HealthCheck(provider)
	err = check(context.Background())
	if err == nil {
		t.Error("HealthCheck should return error when circuit is open")
	}
	if err != call.ErrCircuitOpen {
		t.Errorf("HealthCheck error = %v, want ErrCircuitOpen", err)
	}
}

func TestDopplerError_ServiceError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantHTTP   int
	}{
		{"unauthorized 401", 401, 401},
		{"forbidden 403", 403, 401}, // maps to UnauthorizedError (401)
		{"not found 404", 404, 404},
		{"rate limit 429", 429, 429},
		{"server error 500", 500, 503}, // maps to DependencyError (503)
		{"server error 502", 502, 503},
		{"unknown 418", 418, 500}, // maps to InternalError (500)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			de := &DopplerError{
				StatusCode: tt.statusCode,
				Message:    "test error",
			}
			se := de.ServiceError()
			if se.HTTPCode != tt.wantHTTP {
				t.Errorf("ServiceError().HTTPCode = %d, want %d", se.HTTPCode, tt.wantHTTP)
			}
			if se.Details["doppler_status"] != fmt.Sprintf("%d", tt.statusCode) {
				t.Errorf("ServiceError().Details[doppler_status] = %q, want %q",
					se.Details["doppler_status"], fmt.Sprintf("%d", tt.statusCode))
			}
			// Verify cause chain
			if se.Unwrap() != de {
				t.Error("ServiceError().Unwrap() should return original DopplerError")
			}
		})
	}
}

func TestSecvalRejectsDangerousKeys_DopplerResponse(t *testing.T) {
	// Mock server returns JSON with a dangerous key
	srv := newTestDopplerServer(t, `{"secrets":{"__proto__":{"raw":"evil"}}}`, 200)
	defer srv.Close()

	provider, err := NewDopplerProvider("test-token", "proj", "dev",
		WithAPIURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewDopplerProvider failed: %v", err)
	}

	_, err = provider.Fetch(context.Background())
	if err == nil {
		t.Error("Fetch should reject response with dangerous key __proto__")
	}
}

func TestSecvalRejectsDangerousKeys_FallbackFile(t *testing.T) {
	// Write a fallback file with a dangerous key
	tmpFile := t.TempDir() + "/dangerous.json"
	if err := os.WriteFile(tmpFile, []byte(`{"__proto__": "evil"}`), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	fp := NewFileProvider(tmpFile)
	_, err := fp.Fetch(context.Background())
	if err == nil {
		t.Error("FileProvider.Fetch should reject file with dangerous key __proto__")
	}
}

// newTestDopplerServer creates a test HTTP server that returns the given body.
func newTestDopplerServer(t *testing.T, body string, statusCode int) *httpTestServer {
	t.Helper()
	return newHTTPTestServer(body, statusCode)
}

// httpTestServer wraps net/http/httptest for test convenience.
type httpTestServer struct {
	*httptest.Server
}

func newHTTPTestServer(body string, statusCode int) *httpTestServer {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	}))
	return &httpTestServer{srv}
}
