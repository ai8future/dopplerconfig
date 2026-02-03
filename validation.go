package dopplerconfig

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Validator defines the interface for custom validation functions.
type Validator interface {
	Validate() error
}

// ValidationError contains details about validation failures.
type ValidationError struct {
	Field   string
	Value   any
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (value: %v)", e.Field, e.Message, e.Value)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e)))
	for i, err := range e {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// HasErrors returns true if there are any validation errors.
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Validate validates a config struct.
// It checks for common issues and calls custom Validate() methods if present.
func Validate(cfg any) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %v", v.Kind())
	}

	var errs ValidationErrors

	// Validate struct fields
	validateStruct(v, "", &errs)

	// Call custom Validate() if present
	if validator, ok := cfg.(Validator); ok {
		if err := validator.Validate(); err != nil {
			if ve, ok := err.(ValidationErrors); ok {
				errs = append(errs, ve...)
			} else {
				errs = append(errs, ValidationError{
					Field:   "custom",
					Message: err.Error(),
				})
			}
		}
	}

	if errs.HasErrors() {
		return errs
	}
	return nil
}

func validateStruct(v reflect.Value, prefix string, errs *ValidationErrors) {
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip unexported fields
		if !fieldValue.CanInterface() {
			continue
		}

		fieldName := prefix + field.Name

		// Handle nested structs
		if fieldValue.Kind() == reflect.Struct && !isSpecialType(fieldValue.Type()) {
			validateStruct(fieldValue, fieldName+".", errs)
			continue
		}

		// Check required
		if field.Tag.Get(TagRequired) == "true" {
			if isZero(fieldValue) {
				*errs = append(*errs, ValidationError{
					Field:   fieldName,
					Message: "required field is missing or empty",
				})
			}
		}

		// Run tag-based validations
		validateField(field, fieldValue, fieldName, errs)
	}
}

func validateField(field reflect.StructField, value reflect.Value, name string, errs *ValidationErrors) {
	// Skip if empty and not required (already checked above)
	if isZero(value) {
		return
	}

	// Get validation tags
	tags := parseValidationTags(field.Tag)

	for _, tag := range tags {
		if err := runValidation(tag, value, name); err != nil {
			*errs = append(*errs, *err)
		}
	}
}

type validationTag struct {
	name  string
	param string
}

func parseValidationTags(tag reflect.StructTag) []validationTag {
	validate := tag.Get("validate")
	if validate == "" {
		return nil
	}

	parts := strings.Split(validate, ",")
	tags := make([]validationTag, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Parse "name=param" format
		if idx := strings.Index(p, "="); idx > 0 {
			tags = append(tags, validationTag{
				name:  p[:idx],
				param: p[idx+1:],
			})
		} else {
			tags = append(tags, validationTag{name: p})
		}
	}

	return tags
}

func runValidation(tag validationTag, value reflect.Value, fieldName string) *ValidationError {
	switch tag.name {
	case "min":
		return validateMin(value, tag.param, fieldName)
	case "max":
		return validateMax(value, tag.param, fieldName)
	case "port":
		return validatePort(value, fieldName)
	case "url":
		return validateURL(value, fieldName)
	case "host":
		return validateHost(value, fieldName)
	case "email":
		return validateEmail(value, fieldName)
	case "oneof":
		return validateOneOf(value, tag.param, fieldName)
	case "regex":
		return validateRegex(value, tag.param, fieldName)
	}
	return nil
}

func validateMin(value reflect.Value, param string, fieldName string) *ValidationError {
	min, err := strconv.ParseInt(param, 10, 64)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Value:   param,
			Message: fmt.Sprintf("invalid min validation parameter: %q is not a valid integer", param),
		}
	}

	var val int64
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val = value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val = int64(value.Uint())
	case reflect.String:
		val = int64(len(value.String()))
	default:
		return nil
	}

	if val < min {
		return &ValidationError{
			Field:   fieldName,
			Value:   val,
			Message: fmt.Sprintf("must be at least %d", min),
		}
	}
	return nil
}

func validateMax(value reflect.Value, param string, fieldName string) *ValidationError {
	max, err := strconv.ParseInt(param, 10, 64)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Value:   param,
			Message: fmt.Sprintf("invalid max validation parameter: %q is not a valid integer", param),
		}
	}

	var val int64
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		val = value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val = int64(value.Uint())
	case reflect.String:
		val = int64(len(value.String()))
	default:
		return nil
	}

	if val > max {
		return &ValidationError{
			Field:   fieldName,
			Value:   val,
			Message: fmt.Sprintf("must be at most %d", max),
		}
	}
	return nil
}

func validatePort(value reflect.Value, fieldName string) *ValidationError {
	var port int64
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		port = value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		port = int64(value.Uint())
	case reflect.String:
		p, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil {
			return &ValidationError{
				Field:   fieldName,
				Value:   value.String(),
				Message: "invalid port number",
			}
		}
		port = p
	default:
		return nil
	}

	if port < 1 || port > 65535 {
		return &ValidationError{
			Field:   fieldName,
			Value:   port,
			Message: "port must be between 1 and 65535",
		}
	}
	return nil
}

func validateURL(value reflect.Value, fieldName string) *ValidationError {
	if value.Kind() != reflect.String {
		return nil
	}
	s := value.String()
	if s == "" {
		return nil
	}

	_, err := url.ParseRequestURI(s)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Value:   s,
			Message: "invalid URL",
		}
	}
	return nil
}

func validateHost(value reflect.Value, fieldName string) *ValidationError {
	if value.Kind() != reflect.String {
		return nil
	}
	s := value.String()
	if s == "" {
		return nil
	}

	// Check if it's a valid host:port or just host
	host := s
	if strings.Contains(s, ":") {
		var err error
		host, _, err = net.SplitHostPort(s)
		if err != nil {
			return &ValidationError{
				Field:   fieldName,
				Value:   s,
				Message: "invalid host:port format",
			}
		}
	}

	// Basic hostname validation
	// Allow: IP addresses, localhost, single-label hosts (k8s services), and FQDNs
	if net.ParseIP(host) != nil {
		return nil // Valid IP address
	}

	// Validate hostname format (RFC 1123)
	// Allow single-label hostnames for Kubernetes service names, etc.
	if !isValidHostname(host) {
		return &ValidationError{
			Field:   fieldName,
			Value:   s,
			Message: "invalid hostname",
		}
	}
	return nil
}

// isValidHostname checks if a hostname is valid per RFC 1123.
// Allows single-label hostnames (e.g., "redis", "postgres") for k8s compatibility.
func isValidHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}

	// Each label must be 1-63 chars, alphanumeric or hyphen, not starting/ending with hyphen
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func validateEmail(value reflect.Value, fieldName string) *ValidationError {
	if value.Kind() != reflect.String {
		return nil
	}
	s := value.String()
	if s == "" {
		return nil
	}

	if !emailRegex.MatchString(s) {
		return &ValidationError{
			Field:   fieldName,
			Value:   s,
			Message: "invalid email address",
		}
	}
	return nil
}

func validateOneOf(value reflect.Value, param string, fieldName string) *ValidationError {
	options := strings.Split(param, "|")

	var strVal string
	switch value.Kind() {
	case reflect.String:
		strVal = value.String()
	default:
		strVal = fmt.Sprintf("%v", value.Interface())
	}

	for _, opt := range options {
		if strVal == opt {
			return nil
		}
	}

	return &ValidationError{
		Field:   fieldName,
		Value:   strVal,
		Message: fmt.Sprintf("must be one of: %s", param),
	}
}

// regexCache caches compiled regular expressions for validation.
var regexCache sync.Map

// getCompiledRegex returns a cached compiled regex, or compiles and caches it.
func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	// Store in cache (may race with another goroutine, but that's fine)
	regexCache.Store(pattern, re)
	return re, nil
}

func validateRegex(value reflect.Value, pattern string, fieldName string) *ValidationError {
	if value.Kind() != reflect.String {
		return nil
	}
	s := value.String()
	if s == "" {
		return nil
	}

	re, err := getCompiledRegex(pattern)
	if err != nil {
		return &ValidationError{
			Field:   fieldName,
			Value:   pattern,
			Message: "invalid regex pattern",
		}
	}

	if !re.MatchString(s) {
		return &ValidationError{
			Field:   fieldName,
			Value:   s,
			Message: fmt.Sprintf("must match pattern: %s", pattern),
		}
	}
	return nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func isSpecialType(t reflect.Type) bool {
	// Types that shouldn't be recursed into
	switch t.String() {
	case "time.Time", "dopplerconfig.SecretValue":
		return true
	}
	return false
}
