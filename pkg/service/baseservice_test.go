// Package service_test contains tests for the BaseService implementation.
package service

import (
	"context"
	"errors"
	"testing"

	"cosmossdk.io/log"
)

// dummyService is a simple implementation of the Service interface for testing purposes.
type dummyService struct {
	*BaseService
	// flags to indicate function calls
	started     bool
	stopped     bool
	resetCalled bool
}

// newDummyService creates a new dummy service instance and initializes BaseService.
func newDummyService(name string) *dummyService {
	d := &dummyService{}
	d.BaseService = NewBaseService(log.NewNopLogger(), name, d)
	return d
}

// OnStart sets the started flag and returns nil.
func (d *dummyService) OnStart(ctx context.Context) error {
	d.started = true
	return nil
}

// OnStop sets the stopped flag.
func (d *dummyService) OnStop(ctx context.Context) {
	d.stopped = true
}

// OnReset sets the resetCalled flag and returns nil.
func (d *dummyService) OnReset(ctx context.Context) error {
	d.resetCalled = true
	return nil
}

// Reset implements the Reset method of Service. It simply calls BaseService.Reset.
func (d *dummyService) Reset(ctx context.Context) error {
	return d.BaseService.Reset(ctx)
}

// Start and Stop are already implemented by BaseService.
// IsRunning, Quit, String, and SetLogger are also inherited from BaseService.

func TestBaseService_Start(t *testing.T) {
	tests := []struct {
		name         string
		preStart     func(d *dummyService)
		expectError  bool
		errorContent string
	}{
		{
			name:        "Successful start",
			preStart:    nil,
			expectError: false,
		},
		{
			name: "Start already started service",
			preStart: func(d *dummyService) {
				// Start the service beforehand
				err := d.BaseService.Start(context.Background())
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			},
			expectError:  true,
			errorContent: "already started",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDummyService("dummy")
			if tc.preStart != nil {
				tc.preStart(d)
			}

			err := d.BaseService.Start(context.Background())
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if err != nil && !errors.Is(err, ErrAlreadyStarted) && !contains(err.Error(), tc.errorContent) {
					t.Errorf("expected error containing '%s', got '%s'", tc.errorContent, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				if !d.started {
					t.Error("expected OnStart to be called")
				}
				if !d.IsRunning() {
					t.Error("expected service to be running")
				}
			}
		})
	}
}

func TestBaseService_Stop(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(d *dummyService)
		expectError  bool
		errorContent string
	}{
		{
			name: "Stop without starting",
			setup: func(d *dummyService) {
				// do nothing
			},
			expectError:  true,
			errorContent: "not started",
		},
		{
			name: "Start then stop service",
			setup: func(d *dummyService) {
				err := d.BaseService.Start(context.Background())
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDummyService("dummy")
			tc.setup(d)
			err := d.BaseService.Stop(context.Background())
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if err != nil && !contains(err.Error(), tc.errorContent) {
					t.Errorf("expected error containing '%s', got '%s'", tc.errorContent, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error on stopping, got %v", err)
				}
				if !d.stopped {
					t.Error("expected OnStop to be called")
				}
				if d.IsRunning() {
					t.Error("expected service not to be running")
				}
			}
		})
	}
}

func TestBaseService_Reset(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(d *dummyService)
		expectError  bool
		errorContent string
	}{
		{
			name: "Reset without starting (should succeed)",
			setup: func(d *dummyService) {
				// service not started
			},
			expectError: false,
		},
		{
			name: "Reset while running (should error)",
			setup: func(d *dummyService) {
				// Start the service so that it is running
				err := d.BaseService.Start(context.Background())
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			},
			expectError:  true,
			errorContent: "cannot reset running service",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := newDummyService("dummy")
			tc.setup(d)
			err := d.Reset(context.Background())
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if err != nil && !contains(err.Error(), tc.errorContent) {
					t.Errorf("expected error containing '%s', got '%s'", tc.errorContent, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error on reset, got %v", err)
				}
				if !d.resetCalled {
					t.Error("expected OnReset to be called")
				}
			}
		})
	}
}

// contains is a helper function to check if a substring is present in a string.
func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

// To avoid importing strings package only for Index, we implement our own simple index calculation.
func indexOf(s, substr string) int {
	slen := len(s)
	subLen := len(substr)
	for i := 0; i <= slen-subLen; i++ {
		if s[i:i+subLen] == substr {
			return i
		}
	}
	return -1
}
