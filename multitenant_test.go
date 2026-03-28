package dopplerconfig

import (
	"context"
	"fmt"
	"testing"
)

type MTEnvConfig struct {
	Region   string `doppler:"REGION" default:"us-east-1"`
	LogLevel string `doppler:"LOG_LEVEL" default:"info"`
}

type MTProjectConfig struct {
	Name     string `doppler:"PROJECT_NAME" default:"unnamed"`
	MaxConns int    `doppler:"MAX_CONNS" default:"10"`
}

func TestMultiTenantLoader_LoadEnv(t *testing.T) {
	mock := NewMockProvider(map[string]string{
		"REGION":    "eu-west-1",
		"LOG_LEVEL": "debug",
	})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	env, err := loader.LoadEnv(context.Background())
	if err != nil {
		t.Fatalf("LoadEnv failed: %v", err)
	}

	if env.Region != "eu-west-1" {
		t.Errorf("Region = %q, want %q", env.Region, "eu-west-1")
	}
	if env.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", env.LogLevel, "debug")
	}

	// Verify Env() returns the loaded config
	if loader.Env() == nil {
		t.Fatal("Env() returned nil after LoadEnv")
	}
	if loader.Env().Region != "eu-west-1" {
		t.Errorf("Env().Region = %q, want %q", loader.Env().Region, "eu-west-1")
	}
}

func TestMultiTenantLoader_LoadProject(t *testing.T) {
	mock := NewMockProvider(nil)
	mock.SetProjectValues("", "proj-a", map[string]string{
		"PROJECT_NAME": "Project Alpha",
		"MAX_CONNS":    "25",
	})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	cfg, err := loader.LoadProject(context.Background(), "proj-a")
	if err != nil {
		t.Fatalf("LoadProject failed: %v", err)
	}

	if cfg.Name != "Project Alpha" {
		t.Errorf("Name = %q, want %q", cfg.Name, "Project Alpha")
	}
	if cfg.MaxConns != 25 {
		t.Errorf("MaxConns = %d, want 25", cfg.MaxConns)
	}

	// Verify Project() returns the loaded config
	cached, ok := loader.Project("proj-a")
	if !ok {
		t.Fatal("Project(proj-a) not found")
	}
	if cached.Name != "Project Alpha" {
		t.Errorf("cached Name = %q, want %q", cached.Name, "Project Alpha")
	}
}

func TestMultiTenantLoader_ProjectCodes(t *testing.T) {
	mock := NewMockProvider(nil)
	mock.SetProjectValues("", "b-proj", map[string]string{"PROJECT_NAME": "B"})
	mock.SetProjectValues("", "a-proj", map[string]string{"PROJECT_NAME": "A"})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	loader.LoadProject(context.Background(), "b-proj")
	loader.LoadProject(context.Background(), "a-proj")

	codes := loader.ProjectCodes()
	if len(codes) != 2 {
		t.Fatalf("ProjectCodes length = %d, want 2", len(codes))
	}
	// Should be sorted
	if codes[0] != "a-proj" || codes[1] != "b-proj" {
		t.Errorf("ProjectCodes = %v, want [a-proj, b-proj]", codes)
	}
}

func TestMultiTenantLoader_Projects(t *testing.T) {
	mock := NewMockProvider(nil)
	mock.SetProjectValues("", "proj-x", map[string]string{"PROJECT_NAME": "X"})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)
	loader.LoadProject(context.Background(), "proj-x")

	projects := loader.Projects()
	if len(projects) != 1 {
		t.Fatalf("Projects length = %d, want 1", len(projects))
	}
	if projects["proj-x"].Name != "X" {
		t.Errorf("project name = %q, want %q", projects["proj-x"].Name, "X")
	}
}

func TestMultiTenantLoader_ProjectNotFound(t *testing.T) {
	mock := NewMockProvider(map[string]string{})
	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	_, ok := loader.Project("nonexistent")
	if ok {
		t.Error("expected Project() to return false for unloaded project")
	}
}

func TestMultiTenantLoader_OnEnvChange(t *testing.T) {
	mock := NewMockProvider(map[string]string{
		"REGION":    "us-east-1",
		"LOG_LEVEL": "info",
	})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	// Load initial env
	loader.LoadEnv(context.Background())

	var oldRegion, newRegion string
	loader.OnEnvChange(func(old, new *MTEnvConfig) {
		oldRegion = old.Region
		newRegion = new.Region
	})

	// Update and reload
	mock.SetValues(map[string]string{
		"REGION":    "ap-south-1",
		"LOG_LEVEL": "warn",
	})
	loader.LoadEnv(context.Background())

	if oldRegion != "us-east-1" {
		t.Errorf("old region = %q, want %q", oldRegion, "us-east-1")
	}
	if newRegion != "ap-south-1" {
		t.Errorf("new region = %q, want %q", newRegion, "ap-south-1")
	}
}

func TestMultiTenantLoader_OnProjectChange(t *testing.T) {
	mock := NewMockProvider(nil)
	mock.SetProjectValues("", "proj-a", map[string]string{"PROJECT_NAME": "A"})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)
	loader.LoadProject(context.Background(), "proj-a")

	var receivedDiff *ReloadDiff
	loader.OnProjectChange(func(diff *ReloadDiff) {
		receivedDiff = diff
	})

	// Reload projects
	diff, err := loader.ReloadProjects(context.Background())
	if err != nil {
		t.Fatalf("ReloadProjects failed: %v", err)
	}

	if len(diff.Unchanged) != 1 || diff.Unchanged[0] != "proj-a" {
		t.Errorf("diff.Unchanged = %v, want [proj-a]", diff.Unchanged)
	}

	if receivedDiff == nil {
		t.Fatal("OnProjectChange callback was not called")
	}
}

func TestMultiTenantLoader_Close(t *testing.T) {
	mock := NewMockProvider(map[string]string{})
	fallback := NewMockProvider(map[string]string{})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, fallback)
	err := loader.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestMultiTenantLoader_FetchWithFallback(t *testing.T) {
	// Primary fails, fallback succeeds
	primary := NewMockProviderWithError(fmt.Errorf("primary down"))
	fallback := NewMockProvider(map[string]string{
		"REGION":    "fallback-region",
		"LOG_LEVEL": "warn",
	})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](primary, fallback)

	env, err := loader.LoadEnv(context.Background())
	if err != nil {
		t.Fatalf("LoadEnv with fallback failed: %v", err)
	}
	if env.Region != "fallback-region" {
		t.Errorf("Region = %q, want %q (from fallback)", env.Region, "fallback-region")
	}
}

func TestMultiTenantLoader_EnvBeforeLoad(t *testing.T) {
	mock := NewMockProvider(map[string]string{})
	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	// Env() before LoadEnv should return nil
	if loader.Env() != nil {
		t.Error("Env() should be nil before LoadEnv")
	}
}

func TestMultiTenantLoader_OnEnvChange_NoCallbackOnFirstLoad(t *testing.T) {
	mock := NewMockProvider(map[string]string{
		"REGION":    "us-east-1",
		"LOG_LEVEL": "info",
	})

	loader := NewMultiTenantLoaderWithProvider[MTEnvConfig, MTProjectConfig](mock, nil)

	callbackCalled := false
	loader.OnEnvChange(func(old, new *MTEnvConfig) {
		callbackCalled = true
	})

	// First load should NOT fire callback (no old config)
	loader.LoadEnv(context.Background())

	if callbackCalled {
		t.Error("OnEnvChange should not fire on first load")
	}
}
