package dopplerconfig

import (
	"sync"
	"testing"
)

func TestNewFeatureFlags(t *testing.T) {
	values := map[string]string{
		"FEATURE_DARK_MODE": "true",
	}
	ff := NewFeatureFlags(values, "FEATURE_")
	if ff == nil {
		t.Fatal("NewFeatureFlags returned nil")
	}
	if ff.prefix != "FEATURE_" {
		t.Errorf("prefix = %q, want %q", ff.prefix, "FEATURE_")
	}
}

func TestFeatureFlags_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]string
		prefix   string
		flag     string
		expected bool
	}{
		{
			name:     "true value",
			values:   map[string]string{"FEATURE_DARK_MODE": "true"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "1 value",
			values:   map[string]string{"FEATURE_DARK_MODE": "1"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "yes value",
			values:   map[string]string{"FEATURE_DARK_MODE": "yes"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "on value",
			values:   map[string]string{"FEATURE_DARK_MODE": "on"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "enabled value",
			values:   map[string]string{"FEATURE_DARK_MODE": "enabled"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "enable value",
			values:   map[string]string{"FEATURE_DARK_MODE": "enable"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "false value",
			values:   map[string]string{"FEATURE_DARK_MODE": "false"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: false,
		},
		{
			name:     "empty string",
			values:   map[string]string{"FEATURE_DARK_MODE": ""},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: false,
		},
		{
			name:     "missing flag",
			values:   map[string]string{},
			prefix:   "FEATURE_",
			flag:     "NONEXISTENT",
			expected: false,
		},
		{
			name:     "no prefix",
			values:   map[string]string{"DARK_MODE": "true"},
			prefix:   "",
			flag:     "DARK_MODE",
			expected: true,
		},
		{
			name:     "case-insensitive lookup",
			values:   map[string]string{"feature_dark_mode": "true"},
			prefix:   "FEATURE_",
			flag:     "DARK_MODE",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := NewFeatureFlags(tt.values, tt.prefix)
			got := ff.IsEnabled(tt.flag)
			if got != tt.expected {
				t.Errorf("IsEnabled(%q) = %v, want %v", tt.flag, got, tt.expected)
			}
		})
	}
}

func TestFeatureFlags_IsEnabled_CacheHit(t *testing.T) {
	values := map[string]string{"FEATURE_X": "true"}
	ff := NewFeatureFlags(values, "FEATURE_")

	// First call populates cache
	result1 := ff.IsEnabled("X")
	// Second call should hit cache
	result2 := ff.IsEnabled("X")

	if result1 != result2 {
		t.Errorf("cache inconsistency: first=%v, second=%v", result1, result2)
	}
	if !result1 {
		t.Error("expected true from cache")
	}
}

func TestFeatureFlags_IsDisabled(t *testing.T) {
	values := map[string]string{"FEATURE_X": "false"}
	ff := NewFeatureFlags(values, "FEATURE_")

	if !ff.IsDisabled("X") {
		t.Error("IsDisabled should return true for disabled flag")
	}

	values2 := map[string]string{"FEATURE_Y": "true"}
	ff2 := NewFeatureFlags(values2, "FEATURE_")

	if ff2.IsDisabled("Y") {
		t.Error("IsDisabled should return false for enabled flag")
	}
}

func TestFeatureFlags_GetInt(t *testing.T) {
	values := map[string]string{
		"FEATURE_MAX_RETRIES": "5",
		"FEATURE_BAD_INT":     "not-a-number",
	}
	ff := NewFeatureFlags(values, "FEATURE_")

	if got := ff.GetInt("MAX_RETRIES", 3); got != 5 {
		t.Errorf("GetInt(MAX_RETRIES) = %d, want 5", got)
	}
	if got := ff.GetInt("BAD_INT", 3); got != 3 {
		t.Errorf("GetInt(BAD_INT) = %d, want default 3", got)
	}
	if got := ff.GetInt("MISSING", 42); got != 42 {
		t.Errorf("GetInt(MISSING) = %d, want default 42", got)
	}
}

func TestFeatureFlags_GetFloat(t *testing.T) {
	values := map[string]string{
		"FEATURE_RATE":     "0.75",
		"FEATURE_BAD_RATE": "abc",
	}
	ff := NewFeatureFlags(values, "FEATURE_")

	if got := ff.GetFloat("RATE", 1.0); got != 0.75 {
		t.Errorf("GetFloat(RATE) = %f, want 0.75", got)
	}
	if got := ff.GetFloat("BAD_RATE", 1.0); got != 1.0 {
		t.Errorf("GetFloat(BAD_RATE) = %f, want default 1.0", got)
	}
	if got := ff.GetFloat("MISSING", 2.5); got != 2.5 {
		t.Errorf("GetFloat(MISSING) = %f, want default 2.5", got)
	}
}

func TestFeatureFlags_GetString(t *testing.T) {
	values := map[string]string{
		"FEATURE_REGION": "us-east-1",
	}
	ff := NewFeatureFlags(values, "FEATURE_")

	if got := ff.GetString("REGION", "us-west-2"); got != "us-east-1" {
		t.Errorf("GetString(REGION) = %q, want %q", got, "us-east-1")
	}
	if got := ff.GetString("MISSING", "fallback"); got != "fallback" {
		t.Errorf("GetString(MISSING) = %q, want %q", got, "fallback")
	}
}

func TestFeatureFlags_GetStringSlice(t *testing.T) {
	values := map[string]string{
		"FEATURE_REGIONS":   "us-east-1, eu-west-1, ap-south-1",
		"FEATURE_EMPTY_VAL": "",
	}
	ff := NewFeatureFlags(values, "FEATURE_")

	got := ff.GetStringSlice("REGIONS", nil)
	expected := []string{"us-east-1", "eu-west-1", "ap-south-1"}
	if len(got) != len(expected) {
		t.Fatalf("GetStringSlice length = %d, want %d", len(got), len(expected))
	}
	for i, v := range expected {
		if got[i] != v {
			t.Errorf("GetStringSlice[%d] = %q, want %q", i, got[i], v)
		}
	}

	// Empty value returns default
	defaultSlice := []string{"default"}
	got2 := ff.GetStringSlice("EMPTY_VAL", defaultSlice)
	if len(got2) != 1 || got2[0] != "default" {
		t.Errorf("GetStringSlice(empty) = %v, want %v", got2, defaultSlice)
	}

	// Missing key returns default
	got3 := ff.GetStringSlice("MISSING", defaultSlice)
	if len(got3) != 1 || got3[0] != "default" {
		t.Errorf("GetStringSlice(missing) = %v, want %v", got3, defaultSlice)
	}
}

func TestFeatureFlags_Update(t *testing.T) {
	values := map[string]string{"FEATURE_X": "true"}
	ff := NewFeatureFlags(values, "FEATURE_")

	// Populate cache
	if !ff.IsEnabled("X") {
		t.Fatal("expected X to be enabled initially")
	}

	// Update to disable X
	ff.Update(map[string]string{"FEATURE_X": "false"})

	// Cache should be cleared, new value should be returned
	if ff.IsEnabled("X") {
		t.Error("expected X to be disabled after Update")
	}
}

func TestFeatureFlags_BuildKey(t *testing.T) {
	ff := NewFeatureFlags(nil, "FEATURE_")

	tests := []struct {
		input    string
		expected string
	}{
		{"dark_mode", "FEATURE_DARK_MODE"},
		{"DARK_MODE", "FEATURE_DARK_MODE"},
		{"dark-mode", "FEATURE_DARK_MODE"},
		{"dark mode", "FEATURE_DARK_MODE"},
		{"FEATURE_ALREADY_PREFIXED", "FEATURE_ALREADY_PREFIXED"},
	}

	for _, tt := range tests {
		got := ff.buildKey(tt.input)
		if got != tt.expected {
			t.Errorf("buildKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFeatureFlags_BuildKey_NoPrefix(t *testing.T) {
	ff := NewFeatureFlags(nil, "")

	// Without a prefix, buildKey returns the name as-is (no uppercasing)
	got := ff.buildKey("my_flag")
	if got != "my_flag" {
		t.Errorf("buildKey with no prefix = %q, want %q", got, "my_flag")
	}
}

func TestParseBool(t *testing.T) {
	truths := []string{"true", "1", "yes", "on", "enabled", "enable", "TRUE", "Yes", " true "}
	for _, s := range truths {
		if !parseBool(s) {
			t.Errorf("parseBool(%q) = false, want true", s)
		}
	}

	falses := []string{"false", "0", "no", "off", "disabled", "", "random"}
	for _, s := range falses {
		if parseBool(s) {
			t.Errorf("parseBool(%q) = true, want false", s)
		}
	}
}

func TestFeatureFlagsFromValues(t *testing.T) {
	values := map[string]string{"FEATURE_X": "true"}
	ff := FeatureFlagsFromValues(values)

	if ff.prefix != "FEATURE_" {
		t.Errorf("prefix = %q, want %q", ff.prefix, "FEATURE_")
	}
	if !ff.IsEnabled("X") {
		t.Error("expected X to be enabled via FeatureFlagsFromValues")
	}
}

func TestRolloutConfig_ShouldEnable(t *testing.T) {
	hash := func(s string) uint32 {
		// Simple deterministic hash for testing
		var h uint32
		for _, c := range s {
			h = h*31 + uint32(c)
		}
		return h
	}

	tests := []struct {
		name     string
		config   RolloutConfig
		userID   string
		expected bool
	}{
		{
			name:     "allowed user always enabled",
			config:   RolloutConfig{Percentage: 0, AllowedUsers: []string{"admin"}},
			userID:   "admin",
			expected: true,
		},
		{
			name:     "blocked user always disabled",
			config:   RolloutConfig{Percentage: 100, BlockedUsers: []string{"banned"}},
			userID:   "banned",
			expected: false,
		},
		{
			name:     "0 percent disables all",
			config:   RolloutConfig{Percentage: 0},
			userID:   "user123",
			expected: false,
		},
		{
			name:     "100 percent enables all",
			config:   RolloutConfig{Percentage: 100},
			userID:   "user123",
			expected: true,
		},
		{
			name:     "negative percentage disables",
			config:   RolloutConfig{Percentage: -5},
			userID:   "user123",
			expected: false,
		},
		{
			name:     "allowed takes priority over blocked",
			config:   RolloutConfig{Percentage: 0, AllowedUsers: []string{"both"}, BlockedUsers: []string{"both"}},
			userID:   "both",
			expected: true, // AllowedUsers is checked first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.ShouldEnable(tt.userID, hash)
			if got != tt.expected {
				t.Errorf("ShouldEnable(%q) = %v, want %v", tt.userID, got, tt.expected)
			}
		})
	}
}

func TestFeatureFlags_ConcurrentAccess(t *testing.T) {
	values := map[string]string{
		"FEATURE_X": "true",
		"FEATURE_Y": "false",
	}
	ff := NewFeatureFlags(values, "FEATURE_")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ff.IsEnabled("X")
			ff.IsDisabled("Y")
			ff.GetInt("X", 0)
			ff.GetString("X", "")
		}()
	}
	wg.Wait()
	// No race condition = pass
}
