package config

import "testing"

// TestFlexibleBool is a test suite for FlexibleBool
func TestFlexibleBool(t *testing.T) {
	// Test NewFlexibleBool
	fb := NewFlexibleBool(true)
	if fb.Value() != true {
		t.Errorf("Expected true, got %v", fb.Value())
	}

	// Test Set, String
	// false
	err := fb.Set("false")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
	if fb.Value() != false {
		t.Errorf("Expected false, got %v", fb.Value())
	}
	if fb.String() != "false" {
		t.Errorf("Expected false, got %v", fb.String())
	}
	// true
	err = fb.Set("true")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
	if fb.Value() != true {
		t.Errorf("Expected true, got %v", fb.Value())
	}
	if fb.String() != "true" {
		t.Errorf("Expected true, got %v", fb.String())
	}
	// empty
	err = fb.Set("")
	if err != nil {
		t.Errorf("Expected nil, got %v", err)
	}
	if fb.Value() != true {
		t.Errorf("Expected true, got %v", fb.Value())
	}
	if fb.String() != "true" {
		t.Errorf("Expected true, got %v", fb.String())
	}

	// Test Type
	if fb.Type() != "bool" {
		t.Errorf("Expected bool, got %v", fb.Type())
	}
}
