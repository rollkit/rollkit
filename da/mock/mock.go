package mock

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/rollkit/go-da"
)

// MockDA is a mock for the DA interface
type MockDA struct {
	mock.Mock
}

// MaxBlobSize returns the max blob size.
func (m *MockDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	args := m.Called()
	return args.Get(0).(uint64), args.Error(1)
}

// Get returns the blob by its ID.
func (m *MockDA) Get(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Blob, error) {
	args := m.Called(ids, ns)
	return args.Get(0).([]da.Blob), args.Error(1)
}

// GetIDs returns the IDs in a specific DA height.
func (m *MockDA) GetIDs(ctx context.Context, height uint64, ns da.Namespace) ([]da.ID, error) {
	args := m.Called(height, ns)
	return args.Get(0).([]da.ID), args.Error(1)
}

// Commit returns the commitments for the given blobs.
func (m *MockDA) Commit(ctx context.Context, blobs []da.Blob, ns da.Namespace) ([]da.Commitment, error) {
	args := m.Called(blobs, ns)
	return args.Get(0).([]da.Commitment), args.Error(1)
}

// Submit submits and returns the IDs for the given blobs.
func (m *MockDA) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns da.Namespace) ([]da.ID, error) {
	args := m.Called(blobs, gasPrice, ns)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return args.Get(0).([]da.ID), args.Error(1)
	}
}

// GetProofs returns the proofs for the given ids.
func (m *MockDA) GetProofs(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Proof, error) {
	args := m.Called(ids, ns)
	return args.Get(0).([]da.Proof), args.Error(1)
}

// Validate validates the given ids and proofs and returns validation results.
func (m *MockDA) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, ns da.Namespace) ([]bool, error) {
	args := m.Called(ids, proofs, ns)
	return args.Get(0).([]bool), args.Error(1)
}
