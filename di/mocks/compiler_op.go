// Code generated by mockery v2.44.1. DO NOT EDIT.

package mocks

import (
	di "github.com/michalkurzeja/godi/v2/di"
	mock "github.com/stretchr/testify/mock"
)

// CompilerOp is an autogenerated mock type for the CompilerOp type
type CompilerOp struct {
	mock.Mock
}

type CompilerOp_Expecter struct {
	mock *mock.Mock
}

func (_m *CompilerOp) EXPECT() *CompilerOp_Expecter {
	return &CompilerOp_Expecter{mock: &_m.Mock}
}

// Run provides a mock function with given fields: builder
func (_m *CompilerOp) Run(builder *di.ContainerBuilder) error {
	ret := _m.Called(builder)

	if len(ret) == 0 {
		panic("no return value specified for Run")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(*di.ContainerBuilder) error); ok {
		r0 = rf(builder)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CompilerOp_Run_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Run'
type CompilerOp_Run_Call struct {
	*mock.Call
}

// Run is a helper method to define mock.On call
//   - builder *di.ContainerBuilder
func (_e *CompilerOp_Expecter) Run(builder interface{}) *CompilerOp_Run_Call {
	return &CompilerOp_Run_Call{Call: _e.mock.On("Run", builder)}
}

func (_c *CompilerOp_Run_Call) Run(run func(builder *di.ContainerBuilder)) *CompilerOp_Run_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*di.ContainerBuilder))
	})
	return _c
}

func (_c *CompilerOp_Run_Call) Return(_a0 error) *CompilerOp_Run_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *CompilerOp_Run_Call) RunAndReturn(run func(*di.ContainerBuilder) error) *CompilerOp_Run_Call {
	_c.Call.Return(run)
	return _c
}

// NewCompilerOp creates a new instance of CompilerOp. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewCompilerOp(t interface {
	mock.TestingT
	Cleanup(func())
}) *CompilerOp {
	mock := &CompilerOp{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
