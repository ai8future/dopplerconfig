Date Created: 2026-03-21T02:19:47-07:00
TOTAL_SCORE: 80/100

# dopplerconfig Code Audit & Fix Report

**Agent:** Claude:Opus 4.6
**Files Analyzed:** 14 Go source files (10 production, 4 test)
**Module:** github.com/ai8future/dopplerconfig
**Chassis Dependency:** chassis-go v9.0.0

---

## Executive Summary

The dopplerconfig package is a well-architected configuration management library with clean interfaces, proper use of Go generics, and solid concurrency patterns. The codebase demonstrates strong engineering practices including functional options, ETag caching, security validation, and comprehensive test coverage. However, the audit identified **2 high-severity bugs**, **3 medium-severity bugs**, and **4 code smells** that should be addressed.

---

## Findings

### BUG-1: Cache/Return Map Aliasing in DopplerProvider.FetchProject [HIGH]

**File:** `doppler.go:335-348`
**Severity:** High
**Category:** Data Corruption Bug

The `FetchProject` method stores the `result` map in the provider's cache AND returns the same map reference to the caller. If the caller mutates the returned map (adding, removing, or changing keys), the internal cache is silently corrupted. Subsequent ETag cache hits would return corrupted data.

Note: The cache-hit code path (lines 290-296) correctly creates a copy, making this inconsistency more dangerous since developers might assume all paths are safe.

**Current code:**
```go
// Extract raw values
result := make(map[string]string, len(dopplerResp.Secrets))
for k, v := range dopplerResp.Secrets {
    result[k] = v.Raw
}

// Update cache with new ETag
p.mu.Lock()
p.cache = result          // <-- cache points to same map
if etag := resp.Header.Get("ETag"); etag != "" {
    p.etag = etag
}
p.mu.Unlock()

return result, nil         // <-- caller gets same map
```

**Patch-ready diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -338,8 +338,15 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config strin
 	}

 	// Update cache with new ETag
+	cached := make(map[string]string, len(result))
+	for k, v := range result {
+		cached[k] = v
+	}
+
 	p.mu.Lock()
-	p.cache = result
+	p.cache = cached
 	if etag := resp.Header.Get("ETag"); etag != "" {
 		p.etag = etag
 	}
```

---

### BUG-2: Test Asserts Wrong Chassis Version [HIGH]

**File:** `chassis_test.go:242-250`
**Severity:** High
**Category:** Incorrect Test Assertion

`TestChassisVersion` checks that `ChassisVersion` starts with `'7'`, but the codebase uses chassis-go v9. The comment even says "Should be a semver starting with 7." This test will either fail (correctly revealing the bug) or pass incorrectly if the chassis-go v9 package still reports version "7.x" internally. Either way, the test intent is wrong.

**Current code:**
```go
func TestChassisVersion(t *testing.T) {
    if ChassisVersion == "" {
        t.Error("ChassisVersion should not be empty")
    }
    // Should be a semver starting with "7."
    if ChassisVersion[0] != '7' {
        t.Errorf("ChassisVersion = %q, want major version 7", ChassisVersion)
    }
}
```

**Patch-ready diff:**
```diff
--- a/chassis_test.go
+++ b/chassis_test.go
@@ -244,8 +244,8 @@ func TestChassisVersion(t *testing.T) {
 	if ChassisVersion == "" {
 		t.Error("ChassisVersion should not be empty")
 	}
-	// Should be a semver starting with "7."
-	if ChassisVersion[0] != '7' {
-		t.Errorf("ChassisVersion = %q, want major version 7", ChassisVersion)
+	// Should be a semver starting with "9."
+	if ChassisVersion[0] != '9' {
+		t.Errorf("ChassisVersion = %q, want major version 9", ChassisVersion)
 	}
 }
```

---

### BUG-3: Empty String Values Conflated With Missing Keys [MEDIUM]

**File:** `loader.go:296-314`
**Severity:** Medium
**Category:** Logic Bug

In `unmarshalStruct`, the check `if !exists || rawValue == ""` treats an explicitly-set empty string value from Doppler the same as a missing key. This means:
1. A Doppler key set to `""` gets replaced by its default value
2. A required field set to `""` in Doppler triggers a "required field missing" error

This violates the principle of least surprise. If a user explicitly sets `DATABASE_URL=""` in Doppler, the system should honor that rather than substituting a default.

**Current code:**
```go
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
```

**Patch-ready diff:**
```diff
--- a/loader.go
+++ b/loader.go
@@ -295,7 +295,7 @@ func unmarshalStruct(values map[string]string, v reflect.Value, prefix string, w
 		rawValue, exists := values[dopplerKey]

 		// Use default if not found
-		if !exists || rawValue == "" {
+		if !exists {
 			defaultValue := field.Tag.Get(TagDefault)
 			if defaultValue != "" {
 				rawValue = defaultValue
@@ -309,7 +309,7 @@ func unmarshalStruct(values map[string]string, v reflect.Value, prefix string, w
 		}

 		// Skip if no value
-		if !exists || rawValue == "" {
+		if !exists {
 			continue
 		}
```

---

### BUG-4: Doc Comment / Function Name Mismatch [MEDIUM]

**File:** `feature_flags.go:187-191`
**Severity:** Medium
**Category:** Documentation Bug

The doc comment says `FeatureFlagsFromLoader` but the function is named `FeatureFlagsFromValues`. This is likely a leftover from a rename. It will confuse developers using GoDoc.

**Current code:**
```go
// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
// This is a convenience function for extracting feature flags from a loaded config.
func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

**Patch-ready diff:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -187,7 +187,7 @@ func parseBool(s string) bool {
 	}
 }

-// FeatureFlagsFromLoader creates a FeatureFlags instance from a loader's current values.
+// FeatureFlagsFromValues creates a FeatureFlags instance from a map of config values.
 // This is a convenience function for extracting feature flags from a loaded config.
 func FeatureFlagsFromValues(values map[string]string) *FeatureFlags {
```

---

### BUG-5: uint64-to-int64 Overflow in Min/Max Validation [MEDIUM]

**File:** `validation.go:210-213` and `validation.go:242-245`
**Severity:** Medium
**Category:** Numeric Overflow Bug

Both `validateMin` and `validateMax` cast `value.Uint()` to `int64`. For `uint64` values greater than `math.MaxInt64` (9,223,372,036,854,775,807), this produces a negative number, causing min validation to incorrectly fail and max validation to incorrectly pass.

**Current code (validateMin):**
```go
case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
    val = int64(value.Uint())
```

**Patch-ready diff:**
```diff
--- a/validation.go
+++ b/validation.go
@@ -209,7 +209,13 @@ func validateMin(value reflect.Value, param string, fieldName string) *Validatio
 	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
 		val = value.Int()
 	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
-		val = int64(value.Uint())
+		uval := value.Uint()
+		if uval > uint64(math.MaxInt64) {
+			// Value exceeds int64 range, always >= any reasonable min
+			return nil
+		}
+		val = int64(uval)
 	case reflect.String:
 		val = int64(len(value.String()))
 	default:
@@ -241,7 +247,13 @@ func validateMax(value reflect.Value, param string, fieldName string) *Validatio
 	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
 		val = value.Int()
 	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
-		val = int64(value.Uint())
+		uval := value.Uint()
+		if uval > uint64(max) {
+			return &ValidationError{
+				Field:   fieldName,
+				Value:   uval,
+				Message: fmt.Sprintf("must be at most %d", max),
+			}
+		}
+		val = int64(uval)
 	case reflect.String:
 		val = int64(len(value.String()))
 	default:
```

Note: Requires adding `"math"` to the import block.

---

### SMELL-1: No Size Limit on Successful Doppler Response Body [LOW]

**File:** `doppler.go:314`
**Severity:** Low
**Category:** Resource Exhaustion Risk

Error responses are correctly limited to 1KB via `io.LimitReader`, but successful responses use unbounded `io.ReadAll(resp.Body)`. A compromised or malicious API endpoint could return an extremely large response body, causing an out-of-memory condition.

**Patch-ready diff:**
```diff
--- a/doppler.go
+++ b/doppler.go
@@ -312,7 +312,10 @@ func (p *DopplerProvider) FetchProject(ctx context.Context, project, config stri
 		}
 	}

-	body, err := io.ReadAll(resp.Body)
+	// Limit response body to 10MB to prevent OOM from malicious/corrupted responses
+	const maxResponseSize = 10 * 1024 * 1024
+	limitedBody := io.LimitReader(resp.Body, maxResponseSize+1)
+	body, err := io.ReadAll(limitedBody)
 	if err != nil {
 		return nil, fmt.Errorf("failed to read doppler response: %w", err)
 	}
+	if int64(len(body)) > maxResponseSize {
+		return nil, fmt.Errorf("doppler response exceeds maximum size of %d bytes", maxResponseSize)
+	}
```

---

### SMELL-2: buildKey Prefix Case Inconsistency [LOW]

**File:** `feature_flags.go:158-171`
**Severity:** Low
**Category:** Logic Inconsistency

`buildKey` uppercases the `name` but uses the `prefix` as-is. The `HasPrefix` check compares the uppercased name against `strings.ToUpper(f.prefix)`, but the concatenation at line 170 uses the original (possibly lowercase) prefix. If a user passes `"feature_"` as the prefix, the built key would be `"feature_MY_FLAG"` instead of `"FEATURE_MY_FLAG"`.

**Patch-ready diff:**
```diff
--- a/feature_flags.go
+++ b/feature_flags.go
@@ -161,10 +161,11 @@ func (f *FeatureFlags) buildKey(name string) string {
 	// Normalize the name
 	name = strings.ToUpper(name)
 	name = strings.ReplaceAll(name, "-", "_")
 	name = strings.ReplaceAll(name, " ", "_")

-	if strings.HasPrefix(name, strings.ToUpper(f.prefix)) {
+	upperPrefix := strings.ToUpper(f.prefix)
+	if strings.HasPrefix(name, upperPrefix) {
 		return name
 	}
-	return f.prefix + name
+	return upperPrefix + name
 }
```

---

### SMELL-3: Deprecated os.IsNotExist Usage [LOW]

**File:** `fallback.go:36`
**Severity:** Low
**Category:** Deprecated API

`os.IsNotExist(err)` is the legacy pattern. Modern Go (1.16+) recommends `errors.Is(err, fs.ErrNotExist)` for proper error chain unwrapping.

**Patch-ready diff:**
```diff
--- a/fallback.go
+++ b/fallback.go
@@ -2,8 +2,10 @@ package dopplerconfig

 import (
 	"context"
 	"encoding/json"
+	"errors"
 	"fmt"
+	"io/fs"
 	"os"
 	"strings"

@@ -33,7 +35,7 @@ func (p *FileProvider) FetchProject(ctx context.Context, project, config string)
 	data, err := os.ReadFile(p.path)
 	if err != nil {
-		if os.IsNotExist(err) {
-			return nil, fmt.Errorf("fallback file not found: %s", p.path)
+		if errors.Is(err, fs.ErrNotExist) {
+			return nil, fmt.Errorf("fallback file not found: %s: %w", p.path, err)
 		}
 		return nil, fmt.Errorf("failed to read fallback file: %w", err)
 	}
```

---

### SMELL-4: Error Not Wrapped in File-Not-Found Path [LOW]

**File:** `fallback.go:37`
**Severity:** Low
**Category:** Error Handling

The "file not found" error path does not wrap the original error with `%w`, breaking error chain inspection with `errors.Is`/`errors.As`. (This is addressed in the SMELL-3 diff above.)

---

## Positive Observations

| Aspect | Assessment |
|--------|-----------|
| **Architecture** | Clean provider interface with pluggable backends. Good separation between bootstrap, loading, watching, and validation. |
| **Generics** | Excellent use of Go generics in `Loader[T]`, `MultiTenantLoader[E, P]`, and `Watcher[T]`. |
| **Concurrency** | Proper RWMutex usage throughout. Double-check locking in `FeatureFlags.IsEnabled`. Channels for watcher lifecycle. |
| **Security** | JSON security validation via secval, secret redaction in `SecretValue`, limited error body reads. |
| **Testing** | Good test coverage with mock/recording providers, table-driven tests, test helpers. |
| **API Design** | Functional options pattern (`WithAPIURL`, `WithHTTPClient`, etc.) is idiomatic and extensible. |
| **Error Handling** | Structured errors (`DopplerError`, `ValidationErrors`), proper error wrapping with `%w` (mostly). |
| **Documentation** | Comprehensive package-level doc with usage examples, well-commented code. |
| **Resilience** | ETag caching, circuit breaker, configurable failure policies, fallback chains. |

---

## Score Breakdown

| Category | Points | Deductions | Notes |
|----------|--------|------------|-------|
| Correctness | 25/30 | -5 | Cache aliasing bug, empty-string conflation, uint overflow |
| Test Quality | 18/20 | -2 | Wrong version assertion in chassis test |
| Security | 18/20 | -2 | Unbounded success response body |
| Code Quality | 12/15 | -3 | Doc mismatch, deprecated API, prefix inconsistency |
| Architecture | 15/15 | 0 | Excellent design patterns and separation of concerns |

**TOTAL: 80/100**
