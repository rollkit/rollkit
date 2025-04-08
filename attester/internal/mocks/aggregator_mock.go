package mocks

import (
	"github.com/stretchr/testify/mock"
)

// MockAggregator is a mock implementation for the grpc.Aggregator interface.
// It needs to be exported (starts with uppercase) to be used by other packages.
type MockAggregator struct {
	mock.Mock
}

// Compile-time check to ensure MockAggregator implements the interface
// We need to import the grpc package where the interface is defined.
// Assuming the interface Aggregator is defined in github.com/rollkit/rollkit/attester/internal/grpc
// var _ grpc.Aggregator = (*MockAggregator)(nil) // This requires importing the grpc package

// AddSignature mock implementation.
func (m *MockAggregator) AddSignature(blockHeight uint64, blockHash []byte, attesterID string, signature []byte) (bool, error) {
	args := m.Called(blockHeight, blockHash, attesterID, signature)
	return args.Bool(0), args.Error(1)
}

// GetAggregatedSignatures mock implementation. (Updated name and signature)
func (m *MockAggregator) GetAggregatedSignatures(blockHeight uint64) ([][]byte, bool) {
	args := m.Called(blockHeight)
	// Handle potential nil return for the slice
	var res [][]byte
	ret := args.Get(0)
	if ret != nil {
		res = ret.([][]byte)
	}
	return res, args.Bool(1)
}
