package dopplerconfig

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Loader provides typed configuration loading with automatic struct mapping.
// It supports the single-config pattern used by Airborne.
type Loader[T any] interface {
	// Load fetches and parses configuration into the typed struct.
	Load(ctx context.Context) (*T, error)

	// Reload refreshes the configuration from the source.
	Reload(ctx context.Context) (*T, error)

	// Current returns the currently loaded configuration.
	// Returns nil if Load has not been called.
	Current() *T

	// OnChange registers a callback for configuration changes.
	// The callback receives the old and new configurations.
	OnChange(fn func(old, new *T))

	// Metadata returns information about the loaded configuration.
	Metadata() ConfigMetadata

	// Close releases resources used by the loader.
	Close() error
}

// loader implements Loader[T].
type loader[T any] struct {
	provider Provider
	fallback Provider
	bootstrap BootstrapConfig

	mu        sync.RWMutex
	current   *T
	metadata  ConfigMetadata
	callbacks []func(old, new *T)
}

// NewLoader creates a new typed configuration loader.
// It initializes the appropriate providers based on the bootstrap config.
func NewLoader[T any](bootstrap BootstrapConfig) (Loader[T], error) {
	l := &loader[T]{
		bootstrap: bootstrap,
	}

	// Initialize primary provider (Doppler)
	if bootstrap.IsEnabled() {
		provider, err := NewDopplerProvider(bootstrap.Token, bootstrap.Project, bootstrap.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to create doppler provider: %w", err)
		}
		l.provider = provider
	}

	// Initialize fallback provider (file)
	if bootstrap.HasFallback() {
		l.fallback = NewFileProvider(bootstrap.FallbackPath)
	}

	// Ensure we have at least one provider
	if l.provider == nil && l.fallback == nil {
		return nil, fmt.Errorf("no configuration source: set DOPPLER_TOKEN or DOPPLER_FALLBACK_PATH")
	}

	return l, nil
}

// NewLoaderWithProvider creates a loader with a custom provider.
// Useful for testing or custom provider implementations.
func NewLoaderWithProvider[T any](provider Provider, fallback Provider) Loader[T] {
	return &loader[T]{
		provider: provider,
		fallback: fallback,
	}
}

// Load implements Loader.Load.
func (l *loader[T]) Load(ctx context.Context) (*T, error) {
	return l.loadFromProvider(ctx, false)
}

// Reload implements Loader.Reload.
func (l *loader[T]) Reload(ctx context.Context) (*T, error) {
	return l.loadFromProvider(ctx, true)
}

func (l *loader[T]) loadFromProvider(ctx context.Context, isReload bool) (*T, error) {
	var values map[string]string
	var source string
	var err error

	// Try primary provider first
	if l.provider != nil {
		values, err = l.provider.Fetch(ctx)
		if err == nil {
			source = l.provider.Name()
		}
	}

	// Fall back if primary failed or wasn't available
	if values == nil && l.fallback != nil {
		values, err = l.fallback.Fetch(ctx)
		if err == nil {
			source = l.fallback.Name()
		}
	}

	// Handle failure based on policy
	if values == nil {
		switch l.bootstrap.FailurePolicy {
		case FailurePolicyFail:
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		case FailurePolicyWarn:
			// Create empty config with defaults only
			values = make(map[string]string)
			source = "defaults"
		default:
			if err != nil {
				return nil, fmt.Errorf("failed to load configuration: %w", err)
			}
			return nil, fmt.Errorf("no configuration available")
		}
	}

	// Parse values into struct
	cfg := new(T)
	warnings, parseErr := unmarshalConfig(values, cfg)
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", parseErr)
	}

	// Update state
	l.mu.Lock()
	old := l.current
	l.current = cfg
	l.metadata = ConfigMetadata{
		Source:   source,
		LoadedAt: time.Now(),
		Project:  l.bootstrap.Project,
		Config:   l.bootstrap.Config,
		KeyCount: len(values),
		Warnings: warnings,
	}
	callbacks := l.callbacks
	l.mu.Unlock()

	// Notify callbacks if this is a reload
	if isReload && old != nil {
		for _, cb := range callbacks {
			cb(old, cfg)
		}
	}

	return cfg, nil
}

// Current implements Loader.Current.
func (l *loader[T]) Current() *T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.current
}

// OnChange implements Loader.OnChange.
func (l *loader[T]) OnChange(fn func(old, new *T)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.callbacks = append(l.callbacks, fn)
}

// Metadata implements Loader.Metadata.
func (l *loader[T]) Metadata() ConfigMetadata {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.metadata
}

// Close implements Loader.Close.
func (l *loader[T]) Close() error {
	var errs []error
	if l.provider != nil {
		if err := l.provider.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if l.fallback != nil {
		if err := l.fallback.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// unmarshalConfig populates a struct from a map using reflection.
// Returns warnings for non-fatal issues.
func unmarshalConfig(values map[string]string, target any) ([]string, error) {
	var warnings []string

	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, fmt.Errorf("target must be a non-nil pointer")
	}
	v = v.Elem()

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("target must be a pointer to struct")
	}

	return unmarshalStruct(values, v, "", &warnings)
}

func unmarshalStruct(values map[string]string, v reflect.Value, prefix string, warnings *[]string) ([]string, error) {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip unexported fields
		if !fieldValue.CanSet() {
			continue
		}

		// Handle embedded/nested structs
		if field.Type.Kind() == reflect.Struct && field.Anonymous {
			if _, err := unmarshalStruct(values, fieldValue, prefix, warnings); err != nil {
				return *warnings, err
			}
			continue
		}

		// Handle nested structs (non-anonymous)
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Time{}) && field.Type != reflect.TypeOf(SecretValue{}) {
			newPrefix := prefix + field.Name + "."
			if _, err := unmarshalStruct(values, fieldValue, newPrefix, warnings); err != nil {
				return *warnings, err
			}
			continue
		}

		// Get the doppler tag
		dopplerKey := field.Tag.Get(TagDoppler)
		if dopplerKey == "" {
			// Use field name if no tag
			dopplerKey = prefix + field.Name
		}

		// Get the value
		rawValue, exists := values[dopplerKey]

		// Use default if not found
		if !exists || rawValue == "" {
			defaultValue := field.Tag.Get(TagDefault)
			if defaultValue != "" {
				rawValue = defaultValue
				exists = true
			}
		}

		// Check required
		if field.Tag.Get(TagRequired) == "true" && !exists {
			return *warnings, fmt.Errorf("required field %s (key: %s) not found", field.Name, dopplerKey)
		}

		// Skip if no value
		if !exists || rawValue == "" {
			continue
		}

		// Set the value
		if err := setFieldValue(fieldValue, rawValue); err != nil {
			*warnings = append(*warnings, fmt.Sprintf("failed to set %s: %v", field.Name, err))
		}
	}

	return *warnings, nil
}

func setFieldValue(v reflect.Value, s string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Handle time.Duration specially
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(s)
			if err != nil {
				// Try parsing as seconds
				secs, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					return fmt.Errorf("invalid duration: %s", s)
				}
				d = time.Duration(secs) * time.Second
			}
			v.SetInt(int64(d))
			return nil
		}

		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer: %s", s)
		}
		v.SetInt(i)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer: %s", s)
		}
		v.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("invalid float: %s", s)
		}
		v.SetFloat(f)

	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			// Accept common variations
			switch strings.ToLower(s) {
			case "yes", "y", "on", "enabled", "1":
				b = true
			case "no", "n", "off", "disabled", "0":
				b = false
			default:
				return fmt.Errorf("invalid boolean: %s", s)
			}
		}
		v.SetBool(b)

	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			// Split comma-separated values
			parts := strings.Split(s, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			v.Set(reflect.ValueOf(parts))
		} else {
			return fmt.Errorf("unsupported slice type: %v", v.Type())
		}

	case reflect.Struct:
		// Handle SecretValue
		if v.Type() == reflect.TypeOf(SecretValue{}) {
			v.Set(reflect.ValueOf(NewSecretValue(s)))
			return nil
		}
		return fmt.Errorf("unsupported struct type: %v", v.Type())

	default:
		return fmt.Errorf("unsupported type: %v", v.Kind())
	}

	return nil
}
