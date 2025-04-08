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
	StateFn    func() raft.RaftState                        // Optional func override for State
	LeaderFn   func() raft.ServerAddress                    // Optional func override for Leader
	ApplyFn    func([]byte, time.Duration) raft.ApplyFuture // Optional func override for Apply
	MockState  raft.RaftState                               // Direct state value for simple cases
	MockLeader raft.ServerAddress                           // Direct leader value for simple cases
	ApplyErr   error                                        // Error to be returned by ApplyFuture
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
	m.Called(cmd, timeout)
	if m.ApplyFn != nil {
		return m.ApplyFn(cmd, timeout)
	}
	future := new(MockApplyFuture)
	future.Err = m.ApplyErr
	return future
}

// MockApplyFuture is a mock implementation of raft.ApplyFuture.
// Needs to be exported.
type MockApplyFuture struct {
	Err error // The error this future will return
}

func (m *MockApplyFuture) Error() error {
	return m.Err
}

func (m *MockApplyFuture) Response() interface{} {
	return nil // Default mock response
}

func (m *MockApplyFuture) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *MockApplyFuture) Index() uint64 {
	return 0 // Default mock index
}
