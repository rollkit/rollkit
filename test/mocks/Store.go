// Code generated by mockery v2.53.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"

	types "github.com/rollkit/rollkit/types"
)

// Store is an autogenerated mock type for the Store type
type Store struct {
	mock.Mock
}

// Close provides a mock function with no fields
func (_m *Store) Close() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Close")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetBlockByHash provides a mock function with given fields: ctx, hash
func (_m *Store) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	ret := _m.Called(ctx, hash)

	if len(ret) == 0 {
		panic("no return value specified for GetBlockByHash")
	}

	var r0 *types.SignedHeader
	var r1 *types.Data
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, []byte) (*types.SignedHeader, *types.Data, error)); ok {
		return rf(ctx, hash)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []byte) *types.SignedHeader); ok {
		r0 = rf(ctx, hash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.SignedHeader)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []byte) *types.Data); ok {
		r1 = rf(ctx, hash)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*types.Data)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, []byte) error); ok {
		r2 = rf(ctx, hash)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// GetBlockData provides a mock function with given fields: ctx, height
func (_m *Store) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	ret := _m.Called(ctx, height)

	if len(ret) == 0 {
		panic("no return value specified for GetBlockData")
	}

	var r0 *types.SignedHeader
	var r1 *types.Data
	var r2 error
	if rf, ok := ret.Get(0).(func(context.Context, uint64) (*types.SignedHeader, *types.Data, error)); ok {
		return rf(ctx, height)
	}
	if rf, ok := ret.Get(0).(func(context.Context, uint64) *types.SignedHeader); ok {
		r0 = rf(ctx, height)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.SignedHeader)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, uint64) *types.Data); ok {
		r1 = rf(ctx, height)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*types.Data)
		}
	}

	if rf, ok := ret.Get(2).(func(context.Context, uint64) error); ok {
		r2 = rf(ctx, height)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// GetMetadata provides a mock function with given fields: ctx, key
func (_m *Store) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	ret := _m.Called(ctx, key)

	if len(ret) == 0 {
		panic("no return value specified for GetMetadata")
	}

	var r0 []byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) ([]byte, error)); ok {
		return rf(ctx, key)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) []byte); ok {
		r0 = rf(ctx, key)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSignature provides a mock function with given fields: ctx, height
func (_m *Store) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	ret := _m.Called(ctx, height)

	if len(ret) == 0 {
		panic("no return value specified for GetSignature")
	}

	var r0 *types.Signature
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, uint64) (*types.Signature, error)); ok {
		return rf(ctx, height)
	}
	if rf, ok := ret.Get(0).(func(context.Context, uint64) *types.Signature); ok {
		r0 = rf(ctx, height)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.Signature)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, uint64) error); ok {
		r1 = rf(ctx, height)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetSignatureByHash provides a mock function with given fields: ctx, hash
func (_m *Store) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	ret := _m.Called(ctx, hash)

	if len(ret) == 0 {
		panic("no return value specified for GetSignatureByHash")
	}

	var r0 *types.Signature
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, []byte) (*types.Signature, error)); ok {
		return rf(ctx, hash)
	}
	if rf, ok := ret.Get(0).(func(context.Context, []byte) *types.Signature); ok {
		r0 = rf(ctx, hash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.Signature)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, []byte) error); ok {
		r1 = rf(ctx, hash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetState provides a mock function with given fields: ctx
func (_m *Store) GetState(ctx context.Context) (types.State, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for GetState")
	}

	var r0 types.State
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (types.State, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) types.State); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(types.State)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Height provides a mock function with given fields: ctx
func (_m *Store) Height(ctx context.Context) (uint64, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for Height")
	}

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context) (uint64, error)); ok {
		return rf(ctx)
	}
	if rf, ok := ret.Get(0).(func(context.Context) uint64); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SaveBlockData provides a mock function with given fields: ctx, header, data, signature
func (_m *Store) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	ret := _m.Called(ctx, header, data, signature)

	if len(ret) == 0 {
		panic("no return value specified for SaveBlockData")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *types.SignedHeader, *types.Data, *types.Signature) error); ok {
		r0 = rf(ctx, header, data, signature)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetHeight provides a mock function with given fields: ctx, height
func (_m *Store) SetHeight(ctx context.Context, height uint64) error {
	ret := _m.Called(ctx, height)

	if len(ret) == 0 {
		panic("no return value specified for SetHeight")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uint64) error); ok {
		r0 = rf(ctx, height)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetMetadata provides a mock function with given fields: ctx, key, value
func (_m *Store) SetMetadata(ctx context.Context, key string, value []byte) error {
	ret := _m.Called(ctx, key, value)

	if len(ret) == 0 {
		panic("no return value specified for SetMetadata")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, []byte) error); ok {
		r0 = rf(ctx, key, value)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateState provides a mock function with given fields: ctx, state
func (_m *Store) UpdateState(ctx context.Context, state types.State) error {
	ret := _m.Called(ctx, state)

	if len(ret) == 0 {
		panic("no return value specified for UpdateState")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, types.State) error); ok {
		r0 = rf(ctx, state)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewStore creates a new instance of Store. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewStore(t interface {
	mock.TestingT
	Cleanup(func())
}) *Store {
	mock := &Store{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
