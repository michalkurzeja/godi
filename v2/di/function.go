package di

import (
	"fmt"
	"reflect"

	"github.com/samber/lo"
)

var errType = typeOf[error]()

type Factory struct {
	fn         reflect.Value
	args       FuncArgs
	creates    reflect.Type
	returnsErr bool
}

func NewFactoryT[T any](fn any, args ...Argument) (*Factory, error) {
	return NewFactory(typeOf[T](), reflect.ValueOf(fn), args...)
}

func NewAutoFactory(fn any, args ...Argument) (*Factory, error) {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory must be a function, got %s", fnType.Kind())
	}
	if fnType.NumOut() < 1 {
		return nil, fmt.Errorf("factory must return at least one value")
	}
	return NewFactory(fnType.Out(0), reflect.ValueOf(fn), args...)
}

func NewFactory(of reflect.Type, fn reflect.Value, args ...Argument) (*Factory, error) {
	fnType := fn.Type()
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory must be a function, got %s", fnType.Kind())
	}
	if fnType.NumOut() < 1 {
		return nil, fmt.Errorf("factory must return at least one value")
	}
	if fnType.NumOut() > 2 {
		return nil, fmt.Errorf("factory must return at most two values")
	}
	if !fnType.Out(0).AssignableTo(of) {
		return nil, fmt.Errorf("factory of %[1]s must return a value assignable to %[1]s as a first return value", fqn(of))
	}
	returnsErr := fnType.NumOut() == 2
	if returnsErr && !fnType.Out(1).AssignableTo(errType) {
		return nil, fmt.Errorf("factory may only return an error as a second return value, not %s", fqn(fnType.Out(1)))
	}

	fa, err := newFuncArgs(fnType, args...)
	if err != nil {
		return nil, fmt.Errorf("invalid factory of %s: %w", fqn(of), err)
	}

	return &Factory{fn: fn, args: fa, creates: of, returnsErr: returnsErr}, nil
}

func (fn *Factory) call(c *container) (any, error) {
	in, err := fn.args.resolve(c)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve factory arguments: %w", err)
	}

	call := lo.Ternary(fn.fn.Type().IsVariadic(), fn.fn.CallSlice, fn.fn.Call)
	out := call(in)

	if !fn.returnsErr {
		return out[0].Interface(), nil
	}
	return out[0].Interface(), out[1].Interface().(error)
}

func (fn *Factory) GetArgs() FuncArgs {
	return fn.args
}

type Method struct {
	fn         reflect.Method
	args       FuncArgs
	returnsErr bool
}

func NewMethod(fn reflect.Method, args ...Argument) (*Method, error) {
	fnType := fn.Type
	if fnType.NumOut() > 1 {
		return nil, fmt.Errorf("method %s must return at most one value", fn.Name)
	}
	returnsErr := fnType.NumOut() == 1
	if returnsErr && !fnType.Out(0).AssignableTo(errType) {
		return nil, fmt.Errorf("method %s may only return an error, not %s", fn.Name, fqn(fnType.Out(0)))
	}

	fa, err := newFuncArgs(fnType, args...)
	if err != nil {
		return nil, fmt.Errorf("invalid method %s: %w", fn.Name, err)
	}

	return &Method{fn: fn, args: fa, returnsErr: returnsErr}, nil
}

func (fn *Method) Name() string {
	return fn.fn.Name
}

func (fn *Method) call(c *container, target any) error {
	err := fn.fixIfAttachedToInterface(target)
	if err != nil {
		return err
	}

	err = fn.args.Set(0, NewValue(target)) // Receiver.
	if err != nil {
		return err
	}

	in, err := fn.args.resolve(c)
	if err != nil {
		return fmt.Errorf("failed to resolve arguments of %s: %w", fn.Name(), err)
	}

	call := lo.Ternary(fn.fn.Type.IsVariadic(), fn.fn.Func.CallSlice, fn.fn.Func.Call)
	out := call(in)

	if fn.returnsErr && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

func (fn *Method) fixIfAttachedToInterface(target any) error {
	if !fn.fn.Func.IsValid() {
		// Method was earlier acquired from an interface, now we need to get it from the implementation (`target`).
		// We know the target has this method and that it has a valid signature, because we've already verified
		// that the target implements that interface.
		method, _ := reflect.TypeOf(target).MethodByName(fn.Name())
		args := append([]Argument{NewValue(target)}, fn.args.Arguments()...)

		m, err := NewMethod(method, args...)
		if err != nil {
			return err
		}

		*fn = *m
	}
	return nil
}

func (fn *Method) GetArgs() FuncArgs {
	return fn.args
}

type FuncArg struct {
	typ reflect.Type
	arg Argument
}

func NewFuncArg(typ reflect.Type, arg Argument) (*FuncArg, error) {
	if !arg.Type().AssignableTo(typ) {
		return nil, fmt.Errorf("argument %s must be assignable to %s", fqn(arg.Type()), fqn(typ))
	}
	return &FuncArg{typ: typ, arg: arg}, nil
}

func (a FuncArg) IsEmpty() bool {
	return a.arg == nil
}

func (a FuncArg) Type() reflect.Type {
	return a.typ
}

func (a FuncArg) Argument() Argument {
	return a.arg
}

type FuncArgs []*FuncArg

func newFuncArgs(fn reflect.Type, args ...Argument) (FuncArgs, error) {
	fa := make(FuncArgs, fn.NumIn())

	for i := range fa {
		fa[i] = &FuncArg{typ: fn.In(i)}
	}

	return fa, fa.SetAutomatically(args...)
}

func (fa FuncArgs) ForEach(fn func(i uint, a *FuncArg) error) error {
	for i, a := range fa {
		err := fn(uint(i), a)
		if err != nil {
			return err
		}
	}
	return nil
}

func (fa FuncArgs) Set(i uint, arg Argument) error {
	if uint(len(fa)) <= i {
		return fmt.Errorf("argument index out of range: %d", i)
	}
	if !arg.Type().AssignableTo(fa[i].Type()) {
		return fmt.Errorf("argument %d must be assignable to %s, got %s", i, fqn(fa[i].Type()), fqn(arg.Type()))
	}
	fa[i].arg = arg
	return nil
}

func (fa FuncArgs) SetAutomatically(args ...Argument) error {
	// First, pass over manually-indexed arguments.
	for _, arg := range args {
		if arg.index().auto {
			continue
		}
		err := fa.Set(arg.index().i, arg)
		if err != nil {
			return err
		}
	}

	// Then pass over automatically-indexed arguments.
OUTER:
	for _, arg := range args {
		if !arg.index().auto {
			continue
		}

		for i, a := range fa {
			if a.IsEmpty() && arg.Type().AssignableTo(fa[i].Type()) {
				fa[i].arg = arg
				arg.setIndex(uint(i))
				continue OUTER
			}
		}
		return fmt.Errorf("argument %s cannot be assigned to any of the function arguments", fqn(arg.Type()))
	}

	return nil
}

func (fa FuncArgs) Arguments() []Argument {
	return lo.Map(fa, func(a *FuncArg, _ int) Argument {
		return a.arg
	})
}

func (fa FuncArgs) resolve(c *container) ([]reflect.Value, error) {
	in := make([]reflect.Value, len(fa))
	for i, a := range fa {
		if a.IsEmpty() {
			return nil, fmt.Errorf("argument %d is not set", i)
		}
		vAny, err := a.Argument().resolve(c)
		if err != nil {
			return nil, err
		}
		v, ok := convert(reflect.ValueOf(vAny), a.Type())
		if !ok {
			return nil, fmt.Errorf("argument %d must be assignable to %s, got %s", i, fqn(a.Type()), fqn(reflect.TypeOf(vAny)))
		}
		in[i] = v
	}
	return in, nil
}
