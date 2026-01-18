package dopplerconfig

import (
	"context"
	"testing"
)

// TestConfig is a sample config struct for testing.
type TestConfig struct {
	Server struct {
		Port int    `doppler:"SERVER_PORT" default:"8080"`
		Host string `doppler:"SERVER_HOST" default:"localhost"`
	}
	Database struct {
		URL      string `doppler:"DATABASE_URL" required:"true"`
		MaxConns int    `doppler:"DATABASE_MAX_CONNS" default:"10"`
	}
	Features struct {
		Enabled  bool     `doppler:"FEATURE_ENABLED" default:"false"`
		AllowedUsers []string `doppler:"FEATURE_ALLOWED_USERS"`
	}
	Secret SecretValue `doppler:"API_SECRET"`
}

func TestLoader_Load(t *testing.T) {
	values := map[string]string{
		"SERVER_PORT":        "9090",
		"SERVER_HOST":        "0.0.0.0",
		"DATABASE_URL":       "postgres://localhost/test",
		"DATABASE_MAX_CONNS": "20",
		"FEATURE_ENABLED":    "true",
		"API_SECRET":         "super-secret",
	}

	loader, _ := TestLoader[TestConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, want \"0.0.0.0\"", cfg.Server.Host)
	}
	if cfg.Database.URL != "postgres://localhost/test" {
		t.Errorf("Database.URL = %q, want \"postgres://localhost/test\"", cfg.Database.URL)
	}
	if cfg.Database.MaxConns != 20 {
		t.Errorf("Database.MaxConns = %d, want 20", cfg.Database.MaxConns)
	}
	if !cfg.Features.Enabled {
		t.Error("Features.Enabled = false, want true")
	}
	if cfg.Secret.Value() != "super-secret" {
		t.Errorf("Secret.Value() = %q, want \"super-secret\"", cfg.Secret.Value())
	}
	if cfg.Secret.String() != "[REDACTED]" {
		t.Errorf("Secret.String() = %q, want \"[REDACTED]\"", cfg.Secret.String())
	}
}

func TestLoader_Defaults(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL": "postgres://localhost/test",
	}

	loader, _ := TestLoader[TestConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want default 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("Server.Host = %q, want default \"localhost\"", cfg.Server.Host)
	}
	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want default 10", cfg.Database.MaxConns)
	}
}

func TestLoader_Required(t *testing.T) {
	values := map[string]string{
		"SERVER_PORT": "8080",
		// DATABASE_URL is required but missing
	}

	loader, _ := TestLoader[TestConfig](values)
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Error("Load should fail when required field is missing")
	}
}

func TestLoader_Reload(t *testing.T) {
	values := map[string]string{
		"SERVER_PORT":  "8080",
		"DATABASE_URL": "postgres://localhost/test",
	}

	loader, mock := TestLoader[TestConfig](values)
	cfg1, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg1.Server.Port != 8080 {
		t.Errorf("Initial Server.Port = %d, want 8080", cfg1.Server.Port)
	}

	// Update values
	mock.SetValue("SERVER_PORT", "9000")

	// Reload
	cfg2, err := loader.Reload(context.Background())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if cfg2.Server.Port != 9000 {
		t.Errorf("Reloaded Server.Port = %d, want 9000", cfg2.Server.Port)
	}
}

func TestLoader_OnChange(t *testing.T) {
	values := map[string]string{
		"SERVER_PORT":  "8080",
		"DATABASE_URL": "postgres://localhost/test",
	}

	loader, mock := TestLoader[TestConfig](values)
	_, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Track changes
	var oldPort, newPort int
	loader.OnChange(func(old, new *TestConfig) {
		oldPort = old.Server.Port
		newPort = new.Server.Port
	})

	// Update and reload
	mock.SetValue("SERVER_PORT", "9000")
	loader.Reload(context.Background())

	if oldPort != 8080 {
		t.Errorf("OnChange oldPort = %d, want 8080", oldPort)
	}
	if newPort != 9000 {
		t.Errorf("OnChange newPort = %d, want 9000", newPort)
	}
}

func TestLoader_StringSlice(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":         "postgres://localhost/test",
		"FEATURE_ALLOWED_USERS": "user1, user2, user3",
	}

	loader, _ := TestLoader[TestConfig](values)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	expected := []string{"user1", "user2", "user3"}
	if len(cfg.Features.AllowedUsers) != len(expected) {
		t.Fatalf("AllowedUsers length = %d, want %d", len(cfg.Features.AllowedUsers), len(expected))
	}
	for i, v := range expected {
		if cfg.Features.AllowedUsers[i] != v {
			t.Errorf("AllowedUsers[%d] = %q, want %q", i, cfg.Features.AllowedUsers[i], v)
		}
	}
}
