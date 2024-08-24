package di_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/di/mocks"
)

func emptyFunc() {}

func funcWith3ReturnVals() (string, int, error) {
	return "", 0, nil
}

func funcWith2ReturnValsWithoutError() (string, int) {
	return "", 0
}

func TestNewFactory(t *testing.T) {
	// The comprehensive tests of the returned *Func type are in TestFunction.
	t.Run("can handle a factory with no error", func(t *testing.T) {
		t.Parallel()

		f := func(s string) string {
			return s + s
		}
		sArg := di.NewLiteralArg("foo")

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)

		fn, err := di.NewFactory(f, sArg)
		require.NoError(t, err)

		svc, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Equal(t, "foofoo", svc)
	})
	t.Run("can handle a factory with error", func(t *testing.T) {
		t.Parallel()

		f := func(s string) (string, error) {
			return s + s, nil
		}
		sArg := di.NewLiteralArg("foo")

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)

		fn, err := di.NewFactory(f, sArg)
		require.NoError(t, err)

		svc, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Equal(t, "foofoo", svc)
	})
	t.Run("returns an error when function is not a function", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewFactory(42)
		require.ErrorContains(t, err, "factory kind must be func, got int")
	})
	t.Run("returns an error when factory returns nothing", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewFactory(emptyFunc)
		require.ErrorContains(t, err, "factory github.com/michalkurzeja/godi/v2/di_test.emptyFunc must return at least one value")
	})
	t.Run("returns an error when factory returns more than 2 values", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewFactory(funcWith3ReturnVals)
		require.ErrorContains(t, err, "factory github.com/michalkurzeja/godi/v2/di_test.funcWith3ReturnVals must return at most two values")
	})
	t.Run("returns an error when factory returns something else than error at second value", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewFactory(funcWith2ReturnValsWithoutError)
		require.ErrorContains(t, err, "factory github.com/michalkurzeja/godi/v2/di_test.funcWith2ReturnValsWithoutError may only return an error as a second return value, not int")
	})
}

type structForMethodTest struct {
	methodCalled bool
}

func (s *structForMethodTest) RequireMethodCalled(t *testing.T) {
	t.Helper()
	require.True(t, s.methodCalled, "expected a method to be called")
}

func (s *structForMethodTest) MethodNoArgsNoReturns() { s.methodCalled = true }

func (s *structForMethodTest) MethodNoArgsReturnsErr() error {
	s.methodCalled = true
	return nil
}

func (s *structForMethodTest) MethodNoArgsReturnsNonErr() bool {
	s.methodCalled = true
	return true
}

func (s *structForMethodTest) MethodNoArgsReturnsMultiple() (bool, error) {
	s.methodCalled = true
	return true, nil
}

func (s *structForMethodTest) MethodArgsReturnsErr(str string, i int) error {
	s.methodCalled = true
	return nil
}

func (s *structForMethodTest) MethodVariadicArgsReturnsErr(str string, i ...int) error {
	s.methodCalled = true
	return nil
}

func TestMethod(t *testing.T) {
	s := &structForMethodTest{}
	receiverArg := di.NewLiteralArg(s)

	t.Run("can handle a method with no arguments and no return values", func(t *testing.T) {
		t.Parallel()

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(receiverArg).Return(s, nil)

		fn, err := di.NewMethod((*structForMethodTest).MethodNoArgsNoReturns, receiverArg)
		require.NoError(t, err)

		err = fn.Execute(resolver)
		require.NoError(t, err)
		s.RequireMethodCalled(t)
	})
	t.Run("can handle a method with no arguments and return error", func(t *testing.T) {
		t.Parallel()

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(receiverArg).Return(s, nil)

		fn, err := di.NewMethod((*structForMethodTest).MethodNoArgsReturnsErr, receiverArg)
		require.NoError(t, err)

		err = fn.Execute(resolver)
		require.NoError(t, err)
		s.RequireMethodCalled(t)
	})
	t.Run("can handle a method with arguments and return error", func(t *testing.T) {
		t.Parallel()

		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(receiverArg).Return(s, nil)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(42, nil)

		fn, err := di.NewMethod((*structForMethodTest).MethodArgsReturnsErr, receiverArg, sArg, iArg)
		require.NoError(t, err)

		err = fn.Execute(resolver)
		require.NoError(t, err)
		s.RequireMethodCalled(t)
	})
	t.Run("can handle a method with arguments (variadic) and return error", func(t *testing.T) {
		t.Parallel()

		sArg := di.NewLiteralArg("foo")
		i1Arg := di.NewLiteralArg(42)
		i2Arg := di.NewLiteralArg(66)
		varArg, err := di.NewCompoundArg(reflect.TypeFor[int](), i1Arg, i2Arg)
		require.NoError(t, err)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(receiverArg).Return(s, nil)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(varArg).Return([]int{42, 66}, nil)

		fn, err := di.NewMethod((*structForMethodTest).MethodVariadicArgsReturnsErr, receiverArg, sArg, i1Arg, i2Arg)
		require.NoError(t, err)

		err = fn.Execute(resolver)
		require.NoError(t, err)
		s.RequireMethodCalled(t)
	})
	t.Run("returns error when method doesn't exist", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewMethod((*structForFuncTest).Foo, receiverArg)
		require.ErrorContains(t, err, "method github.com/michalkurzeja/godi/v2/di_test.(*structForFuncTest).Foo not found on receiver github.com/michalkurzeja/godi/v2/di_test.(*structForMethodTest)")
	})
	t.Run("returns error when method returns non-error", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewMethod((*structForMethodTest).MethodNoArgsReturnsNonErr, receiverArg)
		require.ErrorContains(t, err, "method github.com/michalkurzeja/godi/v2/di_test.(*structForMethodTest).MethodNoArgsReturnsNonErr may only return an error, not bool")
	})
	t.Run("returns error when method returns multiple values", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewMethod((*structForMethodTest).MethodNoArgsReturnsMultiple, receiverArg)
		require.ErrorContains(t, err, "method github.com/michalkurzeja/godi/v2/di_test.(*structForMethodTest).MethodNoArgsReturnsMultiple must return at most one value")
	})
}

type structForFuncTest struct {
	field string
}

func (ts *structForFuncTest) Foo(s string, i int) (string, string, int) {
	return ts.field, s + s, i + i
}

func TestFunction(t *testing.T) {
	t.Run("can handle a func with no args and no return values", func(t *testing.T) {
		t.Parallel()

		resolver := mocks.NewArgResolver(t)

		var called bool
		fn, err := di.NewFunc(reflect.ValueOf(func() {
			called = true
		}))
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Empty(t, out)
		require.True(t, called)
	})
	t.Run("can handle a func with args and return values", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(42, nil)

		fn, err := di.NewFunc(f, sArg, iArg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.Equal(t, "foofoo", out[0].Interface())
		require.Equal(t, 84, out[1].Interface())
	})
	t.Run("can handle a func with args and return values, with args out-of-order", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(42, nil)

		fn, err := di.NewFunc(f, iArg, sArg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.Equal(t, "foofoo", out[0].Interface())
		require.Equal(t, 84, out[1].Interface())
	})
	t.Run("can handle a func with slotted args and return values", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		sArgSlotted := di.NewSlottedArg(sArg, 0)
		iArg := di.NewLiteralArg(42)
		iArgSlotted := di.NewSlottedArg(iArg, 1)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(42, nil)

		fn, err := di.NewFunc(f, sArgSlotted, iArgSlotted)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Len(t, out, 2)
		require.Equal(t, "foofoo", out[0].Interface())
		require.Equal(t, 84, out[1].Interface())
	})
	t.Run("can handle a func with variadic arguments and no variadic value", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) bool {
			return true
		})
		sArg := di.NewLiteralArg("foo")
		varArg := di.NewLiteralArg([]int(nil))

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(varArg).Return([]int(nil), nil)

		fn, err := di.NewFunc(f, sArg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.Len(t, out, 1)
		require.True(t, out[0].Bool())
	})
	t.Run("can handle a func with variadic arguments and one variadic value", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) bool {
			return true
		})
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)
		varArg, err := di.NewCompoundArg(reflect.TypeFor[int](), iArg)
		require.NoError(t, err)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(varArg).Return([]int{42}, nil)

		fn, err := di.NewFunc(f, sArg, iArg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.Len(t, out, 1)
		require.True(t, out[0].Bool())
	})
	t.Run("can handle a func with variadic arguments and multiple variadic values", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) bool {
			return true
		})
		sArg := di.NewLiteralArg("foo")
		i1Arg := di.NewLiteralArg(42)
		i2Arg := di.NewLiteralArg(66)
		varArg, err := di.NewCompoundArg(reflect.TypeFor[int](), i1Arg, i2Arg)
		require.NoError(t, err)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(varArg).Return([]int{42, 66}, nil)

		fn, err := di.NewFunc(f, sArg, i1Arg, i2Arg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.Len(t, out, 1)
		require.True(t, out[0].Bool())
	})
	t.Run("can handle a func with variadic arguments and multiple variadic values, some slotted manually", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) []int {
			return i
		})
		sArg := di.NewLiteralArg("foo")
		i1Arg := di.NewLiteralArg(1)
		i2Arg := di.NewLiteralArg(2)
		i3Arg := di.NewLiteralArg(3)
		i4Arg := di.NewLiteralArg(4)
		sArgSlotted := di.NewSlottedArg(sArg, 0)
		i2ArgSlotted := di.NewSlottedArg(i2Arg, 1)
		i3ArgSlotted := di.NewSlottedArg(i3Arg, 1)
		varArg, err := di.NewCompoundArg(reflect.TypeFor[int](), i2Arg, i3Arg, i1Arg, i4Arg)
		require.NoError(t, err)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(varArg).Return([]int{2, 3, 1, 4}, nil)

		fn, err := di.NewFunc(f, sArgSlotted, i1Arg, i2ArgSlotted, i3ArgSlotted, i4Arg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.Len(t, out, 1)
		require.Equal(t, []int{2, 3, 1, 4}, out[0].Interface())
	})
	t.Run("can handle a method", func(t *testing.T) {
		t.Parallel()

		ts := &structForFuncTest{field: "bar"}
		f := reflect.ValueOf(ts.Foo)
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(42, nil)

		fn, err := di.NewFunc(f, sArg, iArg)
		require.NoError(t, err)

		out, err := fn.Execute(resolver)
		require.NoError(t, err)
		require.Len(t, out, 3)
		require.Equal(t, "bar", out[0].Interface())
		require.Equal(t, "foofoo", out[1].Interface())
		require.Equal(t, 84, out[2].Interface())
	})
	t.Run("returns an error if slotted argument isn't assignable to it's slot", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string) bool {
			return true
		})
		iArg := di.NewSlottedArg(di.NewLiteralArg(42), 0)

		_, err := di.NewFunc(f, iArg)
		require.ErrorContains(t, err, "failed to add function arguments: argument int cannot fill slot 0")
	})
	t.Run("returns an error if slotted argument isn't assignable to it's slot (variadic)", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s ...string) bool {
			return true
		})
		iArg := di.NewSlottedArg(di.NewLiteralArg(42), 0)

		_, err := di.NewFunc(f, iArg)
		require.ErrorContains(t, err, "failed to add function arguments: argument int cannot fill slot 0")
	})
	t.Run("returns an error if slotted argument isn't assignable to it's slot (variadic func)", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(i int, s ...string) bool {
			return true
		})
		sArg := di.NewSlottedArg(di.NewLiteralArg("foo"), 0)

		_, err := di.NewFunc(f, sArg)
		require.ErrorContains(t, err, "failed to add function arguments: argument string cannot fill slot 0")
	})
	t.Run("returns an error if an arg cannot be slotted", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		fArg := di.NewLiteralArg(3.14)

		_, err := di.NewFunc(f, sArg, fArg)
		require.ErrorContains(t, err, "argument float64 cannot be slotted to function")
	})
	t.Run("returns an error if an arg cannot be slotted (variadic)", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) bool {
			return true
		})
		sArg := di.NewLiteralArg("foo")
		fArg := di.NewLiteralArg(3.14)

		_, err := di.NewFunc(f, sArg, fArg)
		require.ErrorContains(t, err, "argument float64 cannot be slotted to function")
	})
	t.Run("returns an error if there are too few args when func is executed", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")

		resolver := mocks.NewArgResolver(t)

		fn, err := di.NewFunc(f, sArg)
		require.NoError(t, err)

		_, err = fn.Execute(resolver)
		require.ErrorContains(t, err, "function requires 2 arguments, got 1")
	})
	t.Run("returns an error if there are too few args (variadic)", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i ...int) bool {
			return true
		})

		resolver := mocks.NewArgResolver(t)

		fn, err := di.NewFunc(f)
		require.NoError(t, err)

		_, err = fn.Execute(resolver)
		require.ErrorContains(t, err, "function requires at least 1 arguments, got 0")
	})
	t.Run("returns an error there are too many args", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)
		fArg := di.NewLiteralArg(3.14)

		_, err := di.NewFunc(f, iArg, sArg, fArg)
		require.ErrorContains(t, err, "argument float64 cannot be slotted to function")
	})
	t.Run("returns an error if arg resolution fails", func(t *testing.T) {
		t.Parallel()

		f := reflect.ValueOf(func(s string, i int) (string, int) {
			return s + s, i + i
		})
		sArg := di.NewLiteralArg("foo")
		iArg := di.NewLiteralArg(42)

		resolver := mocks.NewArgResolver(t)
		resolver.EXPECT().Resolve(sArg).Return("foo", nil)
		resolver.EXPECT().Resolve(iArg).Return(nil, assert.AnError)

		fn, err := di.NewFunc(f, sArg, iArg)
		require.NoError(t, err)

		_, err = fn.Execute(resolver)
		require.ErrorIs(t, err, assert.AnError)
	})
	t.Run("returns an error if function is not a func", func(t *testing.T) {
		t.Parallel()

		_, err := di.NewFunc(reflect.ValueOf(42))

		require.ErrorContains(t, err, "function kind must be func, got int")
	})
}
