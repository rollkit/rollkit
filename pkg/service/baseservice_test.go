// Package service_test contains tests for the BaseService implementation.
package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rollkit/rollkit/pkg/log"
)

// dummyService is a simple implementation of the Service interface for testing purposes.
type dummyService struct {
	*BaseService
	runCalled bool
	runError  error
}

// newDummyService creates a new dummy service instance and initializes BaseService.
func newDummyService(name string, runError error) *dummyService {
	d := &dummyService{
		runError: runError,
	}
	d.BaseService = NewBaseService(log.NewNopLogger(), name, d)
	return d
}

// Run implements the Service interface for the dummy service.
func (d *dummyService) Run(ctx context.Context) error {
	d.runCalled = true
	if d.runError != nil {
		return d.runError
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestBaseService_Run(t *testing.T) {
	tests := []struct {
		name        string
		setupImpl   bool
		runError    error
		expectError bool
	}{
		{
			name:        "Default implementation (no impl)",
			setupImpl:   false,
			expectError: true, // Context cancellation error
		},
		{
			name:        "Custom implementation - success",
			setupImpl:   true,
			runError:    nil,
			expectError: true, // Context cancellation error
		},
		{
			name:        "Custom implementation - error",
			setupImpl:   true,
			runError:    errors.New("run error"),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var bs *BaseService
			var ds *dummyService

			if tc.setupImpl {
				ds = newDummyService("dummy", tc.runError)
				bs = ds.BaseService
			} else {
				bs = NewBaseService(log.NewNopLogger(), "dummy", nil)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			err := bs.Run(ctx)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil")
				}
				if tc.runError != nil && !errors.Is(err, tc.runError) {
					t.Errorf("expected error %v, got %v", tc.runError, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}

			if tc.setupImpl && !ds.runCalled {
				t.Error("expected Run to be called on implementation")
			}
		})
	}
}

func TestBaseService_String(t *testing.T) {
	serviceName := "test-service"
	bs := NewBaseService(log.NewNopLogger(), serviceName, nil)

	if bs.String() != serviceName {
		t.Errorf("expected service name %s, got %s", serviceName, bs.String())
	}
}

func TestBaseService_SetLogger(t *testing.T) {
	bs := NewBaseService(log.NewNopLogger(), "test", nil)
	newLogger := log.NewNopLogger()

	bs.SetLogger(newLogger)

	if bs.Logger != newLogger {
		t.Error("expected logger to be updated")
	}
}
