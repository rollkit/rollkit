package mocks

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
	// We might need to import the grpc package if we want the compile-time check
	// grpcServer "github.com/rollkit/rollkit/attester/internal/grpc"
)

// MockRaftNode is a mock implementation for the grpc.RaftNode interface.
// Needs to be exported to be used by other packages.
type MockRaftNode struct {
	mock.Mock
	// Allow configuration directly on the mock instance in tests
	StateFn              func() raft.RaftState                        // Optional func override for State
	LeaderFn             func() raft.ServerAddress                    // Optional func override for Leader
	ApplyFn              func([]byte, time.Duration) raft.ApplyFuture // Optional func override for Apply
	GetConfigurationFn   func() raft.ConfigurationFuture              // Optional func override for GetConfiguration
	MockState            raft.RaftState                               // Direct state value for simple cases
	MockLeader           raft.ServerAddress                           // Direct leader value for simple cases
	ApplyErr             error                                        // Error to be returned by ApplyFuture
	ConfigurationErr     error                                        // Error to be returned by ConfigurationFuture
	MockConfigurationVal raft.Configuration                           // Direct configuration value
}

// Compile-time check (optional, requires importing grpcServer)
// var _ grpcServer.RaftNode = (*MockRaftNode)(nil)

func (m *MockRaftNode) State() raft.RaftState {
	// Execute the mock setup for tracking/assertions
	args := m.Called()
	// Allow overriding via function field
	if m.StateFn != nil {
		return m.StateFn()
	}
	// Use direct value if set
	if m.MockState != 0 {
		return m.MockState
	}
	// Fallback to testify mock return value or default
	if state, ok := args.Get(0).(raft.RaftState); ok {
		return state
	}
	return raft.Follower // Default
}

func (m *MockRaftNode) Leader() raft.ServerAddress {
	args := m.Called()
	if m.LeaderFn != nil {
		return m.LeaderFn()
	}
	if m.MockLeader != "" {
		return m.MockLeader
	}
	if leader, ok := args.Get(0).(raft.ServerAddress); ok {
		return leader
	}
	return "" // Default
}

func (m *MockRaftNode) Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture {
	m.Called(cmd, timeout) // Track the call
	if m.ApplyFn != nil {
		return m.ApplyFn(cmd, timeout)
	}
	future := new(MockApplyFuture)
	future.Err = m.ApplyErr
	return future
}

// GetConfiguration implements the RaftNode interface.
func (m *MockRaftNode) GetConfiguration() raft.ConfigurationFuture {
	m.Called() // Track the call
	if m.GetConfigurationFn != nil {
		return m.GetConfigurationFn()
	}
	future := new(MockConfigurationFuture)
	future.Err = m.ConfigurationErr
	future.Config = m.MockConfigurationVal
	return future
}

// --- Mock Futures ---

// MockApplyFuture is a mock implementation of raft.ApplyFuture.
// Needs to be exported.
type MockApplyFuture struct {
	Err error       // The error this future will return
	Res interface{} // The response this future will return
}

func (m *MockApplyFuture) Error() error {
	return m.Err
}

func (m *MockApplyFuture) Response() interface{} {
	return m.Res
}

func (m *MockApplyFuture) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *MockApplyFuture) Index() uint64 {
	return 0 // Default mock index
}

// MockConfigurationFuture is a mock implementation of raft.ConfigurationFuture.
type MockConfigurationFuture struct {
	Err    error              // The error this future will return
	Config raft.Configuration // The configuration this future will return
}

func (m *MockConfigurationFuture) Error() error {
	return m.Err
}

func (m *MockConfigurationFuture) Configuration() raft.Configuration {
	return m.Config
}

// Index implements the raft.ConfigurationFuture interface.
func (m *MockConfigurationFuture) Index() uint64 {
	// For mock purposes, returning 0 is often sufficient.
	// Adjust if specific index tracking is needed for tests.
	return 0
}
