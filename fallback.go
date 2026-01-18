package dopplerconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileProvider reads configuration from a local JSON file.
// This is used as a fallback when Doppler is unavailable or for local development.
type FileProvider struct {
	path  string
	mu    sync.RWMutex
	cache map[string]string
}

// NewFileProvider creates a new file-based provider.
func NewFileProvider(path string) *FileProvider {
	return &FileProvider{
		path: path,
	}
}

// Fetch reads the JSON file and returns all key-value pairs.
func (p *FileProvider) Fetch(ctx context.Context) (map[string]string, error) {
	return p.FetchProject(ctx, "", "")
}

// FetchProject reads the JSON file. The project/config parameters are ignored
// since file providers don't support multi-project configurations.
func (p *FileProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("fallback file not found: %s", p.path)
		}
		return nil, fmt.Errorf("failed to read fallback file: %w", err)
	}

	// Parse as flat key-value JSON
	var values map[string]interface{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse fallback file: %w", err)
	}

	// Convert to string map (flattening nested structures)
	result := make(map[string]string, len(values))
	flattenJSON("", values, result)

	// Update cache
	p.mu.Lock()
	p.cache = result
	p.mu.Unlock()

	return result, nil
}

// flattenJSON recursively flattens a nested map into a single-level map.
// Nested keys are joined with underscores (e.g., {"server": {"port": 8080}} -> {"SERVER_PORT": "8080"}).
func flattenJSON(prefix string, data map[string]interface{}, result map[string]string) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "_" + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			flattenJSON(fullKey, v, result)
		case string:
			result[fullKey] = v
		case float64:
			// JSON numbers are float64
			if v == float64(int64(v)) {
				result[fullKey] = fmt.Sprintf("%d", int64(v))
			} else {
				result[fullKey] = fmt.Sprintf("%g", v)
			}
		case bool:
			if v {
				result[fullKey] = "true"
			} else {
				result[fullKey] = "false"
			}
		case nil:
			result[fullKey] = ""
		case []interface{}:
			// Convert arrays to comma-separated strings
			parts := make([]string, len(v))
			for i, item := range v {
				parts[i] = fmt.Sprintf("%v", item)
			}
			result[fullKey] = joinStrings(parts, ",")
		default:
			result[fullKey] = fmt.Sprintf("%v", v)
		}
	}
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}

// Name returns the provider name.
func (p *FileProvider) Name() string {
	return "file:" + p.path
}

// Close is a no-op for file providers.
func (p *FileProvider) Close() error {
	return nil
}

// WriteFallbackFile creates or updates a fallback file with the given values.
// This is useful for creating local development files or caching Doppler values.
func WriteFallbackFile(path string, values map[string]string) error {
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write fallback file: %w", err)
	}

	return nil
}

// EnvProvider reads configuration from environment variables.
// This is an alternative to file-based fallback.
type EnvProvider struct {
	prefix string
}

// NewEnvProvider creates a new environment-based provider.
// If prefix is set, only variables with that prefix are included.
func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{prefix: prefix}
}

// Fetch reads all environment variables, optionally filtering by prefix.
func (p *EnvProvider) Fetch(ctx context.Context) (map[string]string, error) {
	return p.FetchProject(ctx, "", "")
}

// FetchProject reads environment variables. Project/config are ignored.
func (p *EnvProvider) FetchProject(ctx context.Context, project, config string) (map[string]string, error) {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		key, value := splitEnv(env)
		if p.prefix == "" || hasPrefix(key, p.prefix) {
			result[key] = value
		}
	}

	return result, nil
}

func splitEnv(env string) (string, string) {
	for i := 0; i < len(env); i++ {
		if env[i] == '=' {
			return env[:i], env[i+1:]
		}
	}
	return env, ""
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Name returns the provider name.
func (p *EnvProvider) Name() string {
	if p.prefix != "" {
		return "env:" + p.prefix + "*"
	}
	return "env"
}

// Close is a no-op for env providers.
func (p *EnvProvider) Close() error {
	return nil
}
