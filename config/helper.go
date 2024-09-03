package config

import (
	"fmt"
	"strconv"
	"strings"
)

// FlexibleBool is a custom bool flag that can be set to true, false, or empty
type FlexibleBool struct {
	value *bool
}

// NewFlexibleBool creates a new FlexibleBool with the given value
func NewFlexibleBool(val bool) *FlexibleBool {
	return &FlexibleBool{value: &val}
}

// String returns the string representation of the FlexibleBool
func (fb *FlexibleBool) String() string {
	if fb.value == nil {
		return "false"
	}
	return fmt.Sprintf("%v", *fb.value)
}

// Set sets the FlexibleBool based on the input string
func (fb *FlexibleBool) Set(value string) error {
	if value == "" {
		*fb.value = true
		return nil
	}

	v, err := strconv.ParseBool(strings.ToLower(value))
	if err != nil {
		return fmt.Errorf("invalid boolean value: %s", value)
	}

	*fb.value = v
	return nil
}

// Type returns the type of the flag
func (fb *FlexibleBool) Type() string {
	return "bool"
}

// Value returns the underlying boolean value
func (fb *FlexibleBool) Value() bool {
	if fb.value == nil {
		return false
	}
	return *fb.value
}
