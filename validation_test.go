package dopplerconfig

import (
	"testing"
)

type ValidationConfig struct {
	MinVal    int    `validate:"min=10"`
	MaxVal    int    `validate:"max=100"`
	Port      int    `validate:"port"`
	URL       string `validate:"url"`
	Email     string `validate:"email"`
	OneOf     string `validate:"oneof=a|b|c"`
	Regex     string `validate:"regex=^[a-z]+$"`
	Host      string `validate:"host"`
	Required  string `required:"true"`
	Optional  string // No validation
}

func TestValidate_Min(t *testing.T) {
	c := ValidationConfig{MinVal: 5, Required: "x"} // Too small
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for MinVal=5")
	}
	c.MinVal = 10
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for MinVal=10: %v", err)
	}
}

func TestValidate_Max(t *testing.T) {
	c := ValidationConfig{MaxVal: 101, Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for MaxVal=101")
	}
	c.MaxVal = 100
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for MaxVal=100: %v", err)
	}
}

func TestValidate_Port(t *testing.T) {
	c := ValidationConfig{Port: 70000, Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for Port=70000")
	}
	c.Port = 8080
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for Port=8080: %v", err)
	}
}

func TestValidate_URL(t *testing.T) {
	c := ValidationConfig{URL: "not-a-url", Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for URL='not-a-url'")
	}
	c.URL = "https://example.com"
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for URL='https://example.com': %v", err)
	}
}

func TestValidate_Email(t *testing.T) {
	c := ValidationConfig{Email: "invalid", Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for Email='invalid'")
	}
	c.Email = "test@example.com"
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for Email='test@example.com': %v", err)
	}
}

func TestValidate_OneOf(t *testing.T) {
	c := ValidationConfig{OneOf: "d", Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for OneOf='d'")
	}
	c.OneOf = "b"
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for OneOf='b': %v", err)
	}
}

func TestValidate_Regex(t *testing.T) {
	c := ValidationConfig{Regex: "123", Required: "x", MinVal: 10}
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for Regex='123'")
	}
	c.Regex = "abc"
	if err := Validate(c); err != nil {
		t.Errorf("Unexpected error for Regex='abc': %v", err)
	}
}

func TestValidate_Required(t *testing.T) {
	c := ValidationConfig{MinVal: 10} // Required field empty
	err := Validate(c)
	if err == nil {
		t.Error("Expected error for missing Required field")
	}
}

func TestValidate_Host(t *testing.T) {
	tests := []struct {
		host  string
		valid bool
	}{
		{"localhost", true},
		{"example.com", true},
		{"redis", true}, // k8s service
		{"127.0.0.1", true},
		{"invalid host", false},
		{"-start.com", false},
	}

	for _, tt := range tests {
		c := ValidationConfig{Host: tt.host, Required: "x", MinVal: 10}
		err := Validate(c)
		if (err == nil) != tt.valid {
			t.Errorf("Host %q: valid=%v, got error=%v", tt.host, tt.valid, err)
		}
	}
}
