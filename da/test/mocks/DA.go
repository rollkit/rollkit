// Code generated by mockery v2.46.0. DO NOT EDIT.

package mocks

import (
	context "context"

	da "github.com/rollkit/rollkit/da"
	mock "github.com/stretchr/testify/mock"
)

// MockDA is an autogenerated mock type for the DA type
type MockDA struct {
	mock.Mock
}

type MockDA_Expecter struct {
	mock *mock.Mock
}

func (_m *MockDA) EXPECT() *MockDA_Expecter {
	return &MockDA_Expecter{mock: &_m.Mock}
}

// Commit provides a mock function with given fields: ctx, blobs, namespace
func (_m *MockDA) Commit(ctx context.Context, blobs [][]byte, namespace []byte) ([][]byte, error) {
	ret := _m.Called(ctx, blobs, namespace)

	if len(ret) == 0 {
		panic("no return value specified for Commit")
	}

	var r0 [][]byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) ([][]byte, error)); ok {
		return rf(ctx, blobs, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) [][]byte); ok {
		r0 = rf(ctx, blobs, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, []byte) error); ok {
		r1 = rf(ctx, blobs, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_Commit_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Commit'
type MockDA_Commit_Call struct {
	*mock.Call
}

// Commit is a helper method to define mock.On call
//   - ctx context.Context
//   - blobs [][]byte
//   - namespace []byte
func (_e *MockDA_Expecter) Commit(ctx interface{}, blobs interface{}, namespace interface{}) *MockDA_Commit_Call {
	return &MockDA_Commit_Call{Call: _e.mock.On("Commit", ctx, blobs, namespace)}
}

func (_c *MockDA_Commit_Call) Run(run func(ctx context.Context, blobs [][]byte, namespace []byte)) *MockDA_Commit_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].([]byte))
	})
	return _c
}

func (_c *MockDA_Commit_Call) Return(_a0 [][]byte, _a1 error) *MockDA_Commit_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_Commit_Call) RunAndReturn(run func(context.Context, [][]byte, []byte) ([][]byte, error)) *MockDA_Commit_Call {
	_c.Call.Return(run)
	return _c
}

// Get provides a mock function with given fields: ctx, ids, namespace
func (_m *MockDA) Get(ctx context.Context, ids [][]byte, namespace []byte) ([][]byte, error) {
	ret := _m.Called(ctx, ids, namespace)

	if len(ret) == 0 {
		panic("no return value specified for Get")
	}

	var r0 [][]byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) ([][]byte, error)); ok {
		return rf(ctx, ids, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) [][]byte); ok {
		r0 = rf(ctx, ids, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, []byte) error); ok {
		r1 = rf(ctx, ids, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_Get_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Get'
type MockDA_Get_Call struct {
	*mock.Call
}

// Get is a helper method to define mock.On call
//   - ctx context.Context
//   - ids [][]byte
//   - namespace []byte
func (_e *MockDA_Expecter) Get(ctx interface{}, ids interface{}, namespace interface{}) *MockDA_Get_Call {
	return &MockDA_Get_Call{Call: _e.mock.On("Get", ctx, ids, namespace)}
}

func (_c *MockDA_Get_Call) Run(run func(ctx context.Context, ids [][]byte, namespace []byte)) *MockDA_Get_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].([]byte))
	})
	return _c
}

func (_c *MockDA_Get_Call) Return(_a0 [][]byte, _a1 error) *MockDA_Get_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_Get_Call) RunAndReturn(run func(context.Context, [][]byte, []byte) ([][]byte, error)) *MockDA_Get_Call {
	_c.Call.Return(run)
	return _c
}

// GetIDs provides a mock function with given fields: ctx, height, namespace
func (_m *MockDA) GetIDs(ctx context.Context, height uint64, namespace []byte) (*da.GetIDsResult, error) {
	ret := _m.Called(ctx, height, namespace)

	if len(ret) == 0 {
		panic("no return value specified for GetIDs")
	}

	var r0 *da.GetIDsResult
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, uint64, []byte) (*da.GetIDsResult, error)); ok {
		return rf(ctx, height, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, uint64, []byte) *da.GetIDsResult); ok {
		r0 = rf(ctx, height, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*da.GetIDsResult)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, uint64, []byte) error); ok {
		r1 = rf(ctx, height, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_GetIDs_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetIDs'
type MockDA_GetIDs_Call struct {
	*mock.Call
}

// GetIDs is a helper method to define mock.On call
//   - ctx context.Context
//   - height uint64
//   - namespace []byte
func (_e *MockDA_Expecter) GetIDs(ctx interface{}, height interface{}, namespace interface{}) *MockDA_GetIDs_Call {
	return &MockDA_GetIDs_Call{Call: _e.mock.On("GetIDs", ctx, height, namespace)}
}

func (_c *MockDA_GetIDs_Call) Run(run func(ctx context.Context, height uint64, namespace []byte)) *MockDA_GetIDs_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(uint64), args[2].([]byte))
	})
	return _c
}

func (_c *MockDA_GetIDs_Call) Return(_a0 *da.GetIDsResult, _a1 error) *MockDA_GetIDs_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_GetIDs_Call) RunAndReturn(run func(context.Context, uint64, []byte) (*da.GetIDsResult, error)) *MockDA_GetIDs_Call {
	_c.Call.Return(run)
	return _c
}

// GetProofs provides a mock function with given fields: ctx, ids, namespace
func (_m *MockDA) GetProofs(ctx context.Context, ids [][]byte, namespace []byte) ([][]byte, error) {
	ret := _m.Called(ctx, ids, namespace)

	if len(ret) == 0 {
		panic("no return value specified for GetProofs")
	}

	var r0 [][]byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) ([][]byte, error)); ok {
		return rf(ctx, ids, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, []byte) [][]byte); ok {
		r0 = rf(ctx, ids, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, []byte) error); ok {
		r1 = rf(ctx, ids, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_GetProofs_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetProofs'
type MockDA_GetProofs_Call struct {
	*mock.Call
}

// GetProofs is a helper method to define mock.On call
//   - ctx context.Context
//   - ids [][]byte
//   - namespace []byte
func (_e *MockDA_Expecter) GetProofs(ctx interface{}, ids interface{}, namespace interface{}) *MockDA_GetProofs_Call {
	return &MockDA_GetProofs_Call{Call: _e.mock.On("GetProofs", ctx, ids, namespace)}
}

func (_c *MockDA_GetProofs_Call) Run(run func(ctx context.Context, ids [][]byte, namespace []byte)) *MockDA_GetProofs_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].([]byte))
	})
	return _c
}

func (_c *MockDA_GetProofs_Call) Return(_a0 [][]byte, _a1 error) *MockDA_GetProofs_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_GetProofs_Call) RunAndReturn(run func(context.Context, [][]byte, []byte) ([][]byte, error)) *MockDA_GetProofs_Call {
	_c.Call.Return(run)
	return _c
}

// MaxBlobSize provides a mock function with given fields: ctx
func (_m *MockDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for MaxBlobSize")
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

// MockDA_MaxBlobSize_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'MaxBlobSize'
type MockDA_MaxBlobSize_Call struct {
	*mock.Call
}

// MaxBlobSize is a helper method to define mock.On call
//   - ctx context.Context
func (_e *MockDA_Expecter) MaxBlobSize(ctx interface{}) *MockDA_MaxBlobSize_Call {
	return &MockDA_MaxBlobSize_Call{Call: _e.mock.On("MaxBlobSize", ctx)}
}

func (_c *MockDA_MaxBlobSize_Call) Run(run func(ctx context.Context)) *MockDA_MaxBlobSize_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context))
	})
	return _c
}

func (_c *MockDA_MaxBlobSize_Call) Return(_a0 uint64, _a1 error) *MockDA_MaxBlobSize_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_MaxBlobSize_Call) RunAndReturn(run func(context.Context) (uint64, error)) *MockDA_MaxBlobSize_Call {
	_c.Call.Return(run)
	return _c
}

// Submit provides a mock function with given fields: ctx, blobs, gasPrice, namespace
func (_m *MockDA) Submit(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte) ([][]byte, error) {
	ret := _m.Called(ctx, blobs, gasPrice, namespace)

	if len(ret) == 0 {
		panic("no return value specified for Submit")
	}

	var r0 [][]byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, float64, []byte) ([][]byte, error)); ok {
		return rf(ctx, blobs, gasPrice, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, float64, []byte) [][]byte); ok {
		r0 = rf(ctx, blobs, gasPrice, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, float64, []byte) error); ok {
		r1 = rf(ctx, blobs, gasPrice, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_Submit_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Submit'
type MockDA_Submit_Call struct {
	*mock.Call
}

// Submit is a helper method to define mock.On call
//   - ctx context.Context
//   - blobs [][]byte
//   - gasPrice float64
//   - namespace []byte
func (_e *MockDA_Expecter) Submit(ctx interface{}, blobs interface{}, gasPrice interface{}, namespace interface{}) *MockDA_Submit_Call {
	return &MockDA_Submit_Call{Call: _e.mock.On("Submit", ctx, blobs, gasPrice, namespace)}
}

func (_c *MockDA_Submit_Call) Run(run func(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte)) *MockDA_Submit_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].(float64), args[3].([]byte))
	})
	return _c
}

func (_c *MockDA_Submit_Call) Return(_a0 [][]byte, _a1 error) *MockDA_Submit_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_Submit_Call) RunAndReturn(run func(context.Context, [][]byte, float64, []byte) ([][]byte, error)) *MockDA_Submit_Call {
	_c.Call.Return(run)
	return _c
}

// SubmitWithOptions provides a mock function with given fields: ctx, blobs, gasPrice, namespace, options
func (_m *MockDA) SubmitWithOptions(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte, options []byte) ([][]byte, error) {
	ret := _m.Called(ctx, blobs, gasPrice, namespace, options)

	if len(ret) == 0 {
		panic("no return value specified for SubmitWithOptions")
	}

	var r0 [][]byte
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, float64, []byte, []byte) ([][]byte, error)); ok {
		return rf(ctx, blobs, gasPrice, namespace, options)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, float64, []byte, []byte) [][]byte); ok {
		r0 = rf(ctx, blobs, gasPrice, namespace, options)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([][]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, float64, []byte, []byte) error); ok {
		r1 = rf(ctx, blobs, gasPrice, namespace, options)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_SubmitWithOptions_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'SubmitWithOptions'
type MockDA_SubmitWithOptions_Call struct {
	*mock.Call
}

// SubmitWithOptions is a helper method to define mock.On call
//   - ctx context.Context
//   - blobs [][]byte
//   - gasPrice float64
//   - namespace []byte
//   - options []byte
func (_e *MockDA_Expecter) SubmitWithOptions(ctx interface{}, blobs interface{}, gasPrice interface{}, namespace interface{}, options interface{}) *MockDA_SubmitWithOptions_Call {
	return &MockDA_SubmitWithOptions_Call{Call: _e.mock.On("SubmitWithOptions", ctx, blobs, gasPrice, namespace, options)}
}

func (_c *MockDA_SubmitWithOptions_Call) Run(run func(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte, options []byte)) *MockDA_SubmitWithOptions_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].(float64), args[3].([]byte), args[4].([]byte))
	})
	return _c
}

func (_c *MockDA_SubmitWithOptions_Call) Return(_a0 [][]byte, _a1 error) *MockDA_SubmitWithOptions_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_SubmitWithOptions_Call) RunAndReturn(run func(context.Context, [][]byte, float64, []byte, []byte) ([][]byte, error)) *MockDA_SubmitWithOptions_Call {
	_c.Call.Return(run)
	return _c
}

// Validate provides a mock function with given fields: ctx, ids, proofs, namespace
func (_m *MockDA) Validate(ctx context.Context, ids [][]byte, proofs [][]byte, namespace []byte) ([]bool, error) {
	ret := _m.Called(ctx, ids, proofs, namespace)

	if len(ret) == 0 {
		panic("no return value specified for Validate")
	}

	var r0 []bool
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, [][]byte, []byte) ([]bool, error)); ok {
		return rf(ctx, ids, proofs, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, [][]byte, [][]byte, []byte) []bool); ok {
		r0 = rf(ctx, ids, proofs, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]bool)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, [][]byte, [][]byte, []byte) error); ok {
		r1 = rf(ctx, ids, proofs, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockDA_Validate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Validate'
type MockDA_Validate_Call struct {
	*mock.Call
}

// Validate is a helper method to define mock.On call
//   - ctx context.Context
//   - ids [][]byte
//   - proofs [][]byte
//   - namespace []byte
func (_e *MockDA_Expecter) Validate(ctx interface{}, ids interface{}, proofs interface{}, namespace interface{}) *MockDA_Validate_Call {
	return &MockDA_Validate_Call{Call: _e.mock.On("Validate", ctx, ids, proofs, namespace)}
}

func (_c *MockDA_Validate_Call) Run(run func(ctx context.Context, ids [][]byte, proofs [][]byte, namespace []byte)) *MockDA_Validate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].([][]byte), args[2].([][]byte), args[3].([]byte))
	})
	return _c
}

func (_c *MockDA_Validate_Call) Return(_a0 []bool, _a1 error) *MockDA_Validate_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockDA_Validate_Call) RunAndReturn(run func(context.Context, [][]byte, [][]byte, []byte) ([]bool, error)) *MockDA_Validate_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockDA creates a new instance of MockDA. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockDA(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockDA {
	mock := &MockDA{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
