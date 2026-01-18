package dopplerconfig

import (
	"strconv"
	"strings"
	"sync"
)

// FeatureFlags provides a simple feature flag interface backed by config values.
// Flags are expected to be stored in Doppler with a consistent naming convention.
type FeatureFlags struct {
	values  map[string]string
	prefix  string
	mu      sync.RWMutex
	cache   map[string]bool
}

// NewFeatureFlags creates a new feature flags helper.
// The prefix is prepended to all flag names (e.g., "FEATURE_" -> "FEATURE_RAG_ENABLED").
func NewFeatureFlags(values map[string]string, prefix string) *FeatureFlags {
	return &FeatureFlags{
		values: values,
		prefix: prefix,
		cache:  make(map[string]bool),
	}
}

// IsEnabled checks if a feature flag is enabled.
// Flag names are case-insensitive and the prefix is automatically added.
func (f *FeatureFlags) IsEnabled(name string) bool {
	f.mu.RLock()
	if cached, ok := f.cache[name]; ok {
		f.mu.RUnlock()
		return cached
	}
	f.mu.RUnlock()

	// Build the full key name
	key := f.buildKey(name)
	value, exists := f.values[key]
	if !exists {
		// Try alternate casings
		for k, v := range f.values {
			if strings.EqualFold(k, key) {
				value = v
				exists = true
				break
			}
		}
	}

	result := parseBool(value)

	// Cache the result
	f.mu.Lock()
	f.cache[name] = result
	f.mu.Unlock()

	return result
}

// IsDisabled checks if a feature flag is disabled.
func (f *FeatureFlags) IsDisabled(name string) bool {
	return !f.IsEnabled(name)
}

// GetInt returns an integer value for a flag.
// Returns defaultVal if the flag doesn't exist or isn't a valid integer.
func (f *FeatureFlags) GetInt(name string, defaultVal int) int {
	key := f.buildKey(name)
	value, exists := f.values[key]
	if !exists {
		return defaultVal
	}

	i, err := strconv.Atoi(value)
	if err != nil {
		return defaultVal
	}
	return i
}

// GetFloat returns a float value for a flag.
func (f *FeatureFlags) GetFloat(name string, defaultVal float64) float64 {
	key := f.buildKey(name)
	value, exists := f.values[key]
	if !exists {
		return defaultVal
	}

	fl, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultVal
	}
	return fl
}

// GetString returns a string value for a flag.
func (f *FeatureFlags) GetString(name string, defaultVal string) string {
	key := f.buildKey(name)
	value, exists := f.values[key]
	if !exists {
		return defaultVal
	}
	return value
}

// GetStringSlice returns a string slice value for a flag.
// Values are expected to be comma-separated.
func (f *FeatureFlags) GetStringSlice(name string, defaultVal []string) []string {
	key := f.buildKey(name)
	value, exists := f.values[key]
	if !exists || value == "" {
		return defaultVal
	}

	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// Update replaces the underlying values map.
// This is used when config is reloaded.
func (f *FeatureFlags) Update(values map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values = values
	f.cache = make(map[string]bool) // Clear cache
}

func (f *FeatureFlags) buildKey(name string) string {
	if f.prefix == "" {
		return name
	}
	// Normalize the name
	name = strings.ToUpper(name)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")

	if strings.HasPrefix(name, strings.ToUpper(f.prefix)) {
		return name
	}
	return f.prefix + name
}

func parseBool(s string) bool {
	if s == "" {
		return false
	}

	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "1", "yes", "on", "enabled", "enable":
		return true
	default:
		return false
	}
}

// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
// This is a convenience function for extracting feature flags from a loaded config.
func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
	return NewFeatureFlags(values, "FEATURE_")
}

// CommonFeatureFlags defines common feature flag patterns.
type CommonFeatureFlags struct {
	// DopplerEnabled indicates if Doppler integration is active.
	// Used for gradual migration from other config systems.
	DopplerEnabled bool `doppler:"FEATURE_DOPPLER_ENABLED" default:"false"`

	// MaintenanceMode puts the service in maintenance mode.
	MaintenanceMode bool `doppler:"FEATURE_MAINTENANCE_MODE" default:"false"`

	// DebugLogging enables verbose debug logging.
	DebugLogging bool `doppler:"FEATURE_DEBUG_LOGGING" default:"false"`

	// RateLimitBypass disables rate limiting (for testing).
	RateLimitBypass bool `doppler:"FEATURE_RATE_LIMIT_BYPASS" default:"false"`
}

// RolloutConfig supports percentage-based feature rollouts.
type RolloutConfig struct {
	// Percentage of users/requests that should see the feature (0-100).
	Percentage int `doppler:"ROLLOUT_PERCENTAGE" default:"0"`

	// AllowedUsers is a comma-separated list of user IDs that always get the feature.
	AllowedUsers []string `doppler:"ROLLOUT_ALLOWED_USERS"`

	// BlockedUsers is a comma-separated list of user IDs that never get the feature.
	BlockedUsers []string `doppler:"ROLLOUT_BLOCKED_USERS"`
}

// ShouldEnable checks if a feature should be enabled for a given user.
// If userID is in AllowedUsers, returns true.
// If userID is in BlockedUsers, returns false.
// Otherwise, uses the percentage-based rollout.
func (r *RolloutConfig) ShouldEnable(userID string, hashFunc func(string) uint32) bool {
	// Check allow list
	for _, u := range r.AllowedUsers {
		if u == userID {
			return true
		}
	}

	// Check block list
	for _, u := range r.BlockedUsers {
		if u == userID {
			return false
		}
	}

	// Use hash for consistent rollout
	if r.Percentage <= 0 {
		return false
	}
	if r.Percentage >= 100 {
		return true
	}

	hash := hashFunc(userID)
	return (hash % 100) < uint32(r.Percentage)
}
