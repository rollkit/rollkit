package mocks

import (
	"github.com/stretchr/testify/mock"
)

// MockAggregator is a mock implementation for fsm.AggregatorService.
// It needs to be exported (starts with uppercase) to be used by other packages.
type MockAggregator struct {
	mock.Mock
}

// SetBlockData mock implementation.
func (m *MockAggregator) SetBlockData(blockHash []byte, dataToSign []byte) {
	m.Called(blockHash, dataToSign)
}

// AddSignature mock implementation.
func (m *MockAggregator) AddSignature(blockHeight uint64, blockHash []byte, attesterID string, signature []byte) (bool, error) {
	args := m.Called(blockHeight, blockHash, attesterID, signature)
	return args.Bool(0), args.Error(1)
}

// GetAggregatedSignatures mock implementation.
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

// Compile-time check could be added if the fsm package was imported here,
// but typically mocks don't import the packages they are mocking directly.
// var _ fsm.AggregatorService = (*MockAggregator)(nil)
