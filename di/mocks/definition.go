// Code generated by mockery. DO NOT EDIT.

package mocks

import (
	di "github.com/michalkurzeja/godi/v2/di"
	mock "github.com/stretchr/testify/mock"

	reflect "reflect"
)

// Definition is an autogenerated mock type for the Definition type
type Definition struct {
	mock.Mock
}

type Definition_Expecter struct {
	mock *mock.Mock
}

func (_m *Definition) EXPECT() *Definition_Expecter {
	return &Definition_Expecter{mock: &_m.Mock}
}

// ID provides a mock function with no fields
func (_m *Definition) ID() di.ID {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for ID")
	}

	var r0 di.ID
	if rf, ok := ret.Get(0).(func() di.ID); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(di.ID)
	}

	return r0
}

// Definition_ID_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'ID'
type Definition_ID_Call struct {
	*mock.Call
}

// ID is a helper method to define mock.On call
func (_e *Definition_Expecter) ID() *Definition_ID_Call {
	return &Definition_ID_Call{Call: _e.mock.On("ID")}
}

func (_c *Definition_ID_Call) Run(run func()) *Definition_ID_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Definition_ID_Call) Return(_a0 di.ID) *Definition_ID_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Definition_ID_Call) RunAndReturn(run func() di.ID) *Definition_ID_Call {
	_c.Call.Return(run)
	return _c
}

// Labels provides a mock function with no fields
func (_m *Definition) Labels() []di.Label {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Labels")
	}

	var r0 []di.Label
	if rf, ok := ret.Get(0).(func() []di.Label); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]di.Label)
		}
	}

	return r0
}

// Definition_Labels_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Labels'
type Definition_Labels_Call struct {
	*mock.Call
}

// Labels is a helper method to define mock.On call
func (_e *Definition_Expecter) Labels() *Definition_Labels_Call {
	return &Definition_Labels_Call{Call: _e.mock.On("Labels")}
}

func (_c *Definition_Labels_Call) Run(run func()) *Definition_Labels_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Definition_Labels_Call) Return(_a0 []di.Label) *Definition_Labels_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Definition_Labels_Call) RunAndReturn(run func() []di.Label) *Definition_Labels_Call {
	_c.Call.Return(run)
	return _c
}

// Type provides a mock function with no fields
func (_m *Definition) Type() reflect.Type {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Type")
	}

	var r0 reflect.Type
	if rf, ok := ret.Get(0).(func() reflect.Type); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(reflect.Type)
		}
	}

	return r0
}

// Definition_Type_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Type'
type Definition_Type_Call struct {
	*mock.Call
}

// Type is a helper method to define mock.On call
func (_e *Definition_Expecter) Type() *Definition_Type_Call {
	return &Definition_Type_Call{Call: _e.mock.On("Type")}
}

func (_c *Definition_Type_Call) Run(run func()) *Definition_Type_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Definition_Type_Call) Return(_a0 reflect.Type) *Definition_Type_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Definition_Type_Call) RunAndReturn(run func() reflect.Type) *Definition_Type_Call {
	_c.Call.Return(run)
	return _c
}

// NewDefinition creates a new instance of Definition. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewDefinition(t interface {
	mock.TestingT
	Cleanup(func())
}) *Definition {
	mock := &Definition{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
