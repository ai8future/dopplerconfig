package dopplerconfig

import (
	"encoding/json"
	"testing"
)

func TestLoadBootstrapFromEnv(t *testing.T) {
	t.Setenv("DOPPLER_TOKEN", "dp.st.test-token")
	t.Setenv("DOPPLER_PROJECT", "myproject")
	t.Setenv("DOPPLER_CONFIG", "dev")
	t.Setenv("DOPPLER_FALLBACK_PATH", "/tmp/fallback.json")
	t.Setenv("DOPPLER_WATCH_ENABLED", "true")
	t.Setenv("DOPPLER_FAILURE_POLICY", "fail")

	cfg := LoadBootstrapFromEnv()

	if cfg.Token != "dp.st.test-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "dp.st.test-token")
	}
	if cfg.Project != "myproject" {
		t.Errorf("Project = %q, want %q", cfg.Project, "myproject")
	}
	if cfg.Config != "dev" {
		t.Errorf("Config = %q, want %q", cfg.Config, "dev")
	}
	if cfg.FallbackPath != "/tmp/fallback.json" {
		t.Errorf("FallbackPath = %q, want %q", cfg.FallbackPath, "/tmp/fallback.json")
	}
	if !cfg.WatchEnabled {
		t.Error("WatchEnabled = false, want true")
	}
	if cfg.FailurePolicy != FailurePolicyFail {
		t.Errorf("FailurePolicy = %d, want FailurePolicyFail (%d)", cfg.FailurePolicy, FailurePolicyFail)
	}
}

func TestLoadBootstrapFromEnv_FailurePolicies(t *testing.T) {
	tests := []struct {
		envVal   string
		expected FailurePolicy
	}{
		{"fail", FailurePolicyFail},
		{"warn", FailurePolicyWarn},
		{"fallback", FailurePolicyFallback},
		{"", FailurePolicyFallback},        // default
		{"unknown", FailurePolicyFallback}, // unrecognized -> fallback
	}

	for _, tt := range tests {
		t.Run("policy_"+tt.envVal, func(t *testing.T) {
			t.Setenv("DOPPLER_TOKEN", "")
			t.Setenv("DOPPLER_FAILURE_POLICY", tt.envVal)
			cfg := LoadBootstrapFromEnv()
			if cfg.FailurePolicy != tt.expected {
				t.Errorf("FailurePolicy = %d, want %d for env %q", cfg.FailurePolicy, tt.expected, tt.envVal)
			}
		})
	}
}

func TestBootstrapConfig_IsEnabled(t *testing.T) {
	cfg := BootstrapConfig{Token: "some-token"}
	if !cfg.IsEnabled() {
		t.Error("IsEnabled() = false, want true when token is set")
	}

	cfg2 := BootstrapConfig{}
	if cfg2.IsEnabled() {
		t.Error("IsEnabled() = true, want false when token is empty")
	}
}

func TestBootstrapConfig_HasFallback(t *testing.T) {
	cfg := BootstrapConfig{FallbackPath: "/tmp/fallback.json"}
	if !cfg.HasFallback() {
		t.Error("HasFallback() = false, want true when path is set")
	}

	cfg2 := BootstrapConfig{}
	if cfg2.HasFallback() {
		t.Error("HasFallback() = true, want false when path is empty")
	}
}

func TestSecretValue_EmptyString(t *testing.T) {
	sv := SecretValue{}
	if sv.String() != "[empty]" {
		t.Errorf("String() = %q, want %q", sv.String(), "[empty]")
	}
}

func TestSecretValue_MarshalJSON(t *testing.T) {
	sv := NewSecretValue("super-secret-key")
	data, err := json.Marshal(sv)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if string(data) != `"[REDACTED]"` {
		t.Errorf("MarshalJSON = %s, want %q", data, `"[REDACTED]"`)
	}
}

func TestNewSecretValue(t *testing.T) {
	sv := NewSecretValue("my-secret")
	if sv.Value() != "my-secret" {
		t.Errorf("Value() = %q, want %q", sv.Value(), "my-secret")
	}
}
