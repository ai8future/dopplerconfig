package dopplerconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileProvider_Fetch(t *testing.T) {
	// Create a temp file with valid JSON
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{"SERVER_PORT": "8080", "DB_HOST": "localhost"}`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	fp := NewFileProvider(path)
	values, err := fp.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if values["SERVER_PORT"] != "8080" {
		t.Errorf("SERVER_PORT = %q, want %q", values["SERVER_PORT"], "8080")
	}
	if values["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST = %q, want %q", values["DB_HOST"], "localhost")
	}
}

func TestFileProvider_FetchProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	err := os.WriteFile(path, []byte(`{"KEY": "value"}`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	fp := NewFileProvider(path)
	// project/config params are ignored for FileProvider
	values, err := fp.FetchProject(context.Background(), "any-project", "any-config")
	if err != nil {
		t.Fatalf("FetchProject failed: %v", err)
	}
	if values["KEY"] != "value" {
		t.Errorf("KEY = %q, want %q", values["KEY"], "value")
	}
}

func TestFileProvider_FileNotFound(t *testing.T) {
	fp := NewFileProvider("/nonexistent/path/config.json")
	_, err := fp.Fetch(context.Background())
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFileProvider_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	err := os.WriteFile(path, []byte(`not json at all`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	fp := NewFileProvider(path)
	_, err = fp.Fetch(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFileProvider_Name(t *testing.T) {
	fp := NewFileProvider("/tmp/test.json")
	if got := fp.Name(); got != "file:/tmp/test.json" {
		t.Errorf("Name() = %q, want %q", got, "file:/tmp/test.json")
	}
}

func TestFlattenJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]string
	}{
		{
			name:     "flat string map",
			input:    map[string]interface{}{"KEY": "value"},
			expected: map[string]string{"KEY": "value"},
		},
		{
			name: "nested object",
			input: map[string]interface{}{
				"server": map[string]interface{}{
					"port": float64(8080),
					"host": "localhost",
				},
			},
			expected: map[string]string{
				"server_port": "8080",
				"server_host": "localhost",
			},
		},
		{
			name:     "integer float",
			input:    map[string]interface{}{"COUNT": float64(42)},
			expected: map[string]string{"COUNT": "42"},
		},
		{
			name:     "fractional float",
			input:    map[string]interface{}{"RATE": float64(3.14)},
			expected: map[string]string{"RATE": "3.14"},
		},
		{
			name:     "boolean true",
			input:    map[string]interface{}{"ENABLED": true},
			expected: map[string]string{"ENABLED": "true"},
		},
		{
			name:     "boolean false",
			input:    map[string]interface{}{"ENABLED": false},
			expected: map[string]string{"ENABLED": "false"},
		},
		{
			name:     "null value",
			input:    map[string]interface{}{"EMPTY": nil},
			expected: map[string]string{"EMPTY": ""},
		},
		{
			name:     "array value",
			input:    map[string]interface{}{"TAGS": []interface{}{"a", "b", "c"}},
			expected: map[string]string{"TAGS": "a,b,c"},
		},
		{
			name: "deeply nested",
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "deep",
					},
				},
			},
			expected: map[string]string{"a_b_c": "deep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			flattenJSON("", tt.input, result)
			for k, want := range tt.expected {
				if got, ok := result[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if got != want {
					t.Errorf("result[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestWriteFallbackFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fallback.json")

	values := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}

	if err := WriteFallbackFile(path, values); err != nil {
		t.Fatalf("WriteFallbackFile failed: %v", err)
	}

	// Verify the file can be read back by FileProvider
	fp := NewFileProvider(path)
	got, err := fp.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Read back failed: %v", err)
	}

	if got["KEY1"] != "value1" {
		t.Errorf("KEY1 = %q, want %q", got["KEY1"], "value1")
	}
	if got["KEY2"] != "value2" {
		t.Errorf("KEY2 = %q, want %q", got["KEY2"], "value2")
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestEnvProvider_Fetch(t *testing.T) {
	// Set test env vars
	t.Setenv("DOPPLERTEST_KEY1", "val1")
	t.Setenv("DOPPLERTEST_KEY2", "val2")

	ep := NewEnvProvider("DOPPLERTEST_")
	values, err := ep.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if values["DOPPLERTEST_KEY1"] != "val1" {
		t.Errorf("KEY1 = %q, want %q", values["DOPPLERTEST_KEY1"], "val1")
	}
	if values["DOPPLERTEST_KEY2"] != "val2" {
		t.Errorf("KEY2 = %q, want %q", values["DOPPLERTEST_KEY2"], "val2")
	}
}

func TestEnvProvider_FetchNoPrefix(t *testing.T) {
	t.Setenv("DOPPLERTEST_NOPREFIX", "hello")

	ep := NewEnvProvider("")
	values, err := ep.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if values["DOPPLERTEST_NOPREFIX"] != "hello" {
		t.Error("expected DOPPLERTEST_NOPREFIX in unprefixed results")
	}
}

func TestEnvProvider_Name(t *testing.T) {
	ep1 := NewEnvProvider("APP_")
	if got := ep1.Name(); got != "env:APP_*" {
		t.Errorf("Name() = %q, want %q", got, "env:APP_*")
	}

	ep2 := NewEnvProvider("")
	if got := ep2.Name(); got != "env" {
		t.Errorf("Name() = %q, want %q", got, "env")
	}
}

func TestSplitEnv(t *testing.T) {
	tests := []struct {
		input   string
		wantKey string
		wantVal string
	}{
		{"KEY=value", "KEY", "value"},
		{"KEY=", "KEY", ""},
		{"KEY=val=ue", "KEY", "val=ue"},
		{"NOEQUALS", "NOEQUALS", ""},
	}

	for _, tt := range tests {
		k, v := splitEnv(tt.input)
		if k != tt.wantKey || v != tt.wantVal {
			t.Errorf("splitEnv(%q) = (%q, %q), want (%q, %q)", tt.input, k, v, tt.wantKey, tt.wantVal)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	if !hasPrefix("APP_KEY", "APP_") {
		t.Error("hasPrefix(APP_KEY, APP_) should be true")
	}
	if hasPrefix("OTHER_KEY", "APP_") {
		t.Error("hasPrefix(OTHER_KEY, APP_) should be false")
	}
	if hasPrefix("AP", "APP_") {
		t.Error("hasPrefix(AP, APP_) should be false (shorter than prefix)")
	}
}
