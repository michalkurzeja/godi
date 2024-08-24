package di

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

var errType = reflect.TypeFor[error]()

type Factory struct {
	fn           *Func
	returnedType reflect.Type
	returnsErr   bool
}

func NewFactory(fn any, args ...Arg) (*Factory, error) {
	fnVal := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)

	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("factory kind must be func, got %s", fnType.Kind())
	}

	fnName := util.FuncName(fnVal)
	if fnType.NumOut() < 1 {
		return nil, fmt.Errorf("factory %s must return at least one value", fnName)
	}
	if fnType.NumOut() > 2 {
		return nil, fmt.Errorf("factory %s must return at most two values", fnName)
	}
	returnsErr := fnType.NumOut() == 2
	if returnsErr && !fnType.Out(1).AssignableTo(errType) {
		return nil, fmt.Errorf("factory %s may only return an error as a second return value, not %s", fnName, util.Signature(fnType.Out(1)))
	}

	f, err := NewFunc(fnVal, args...)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to create factory %s", fnName)
	}

	return &Factory{fn: f, returnedType: fnType.Out(0), returnsErr: returnsErr}, nil
}

func (f *Factory) Execute(resolver ArgResolver) (any, error) {
	out, err := f.fn.Execute(resolver)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to execute factory")
	}

	if f.returnsErr && !out[1].IsNil() {
		return out[0].Interface(), out[1].Interface().(error)
	}
	return out[0].Interface(), nil
}

func (f *Factory) Args() *ArgList {
	return f.fn.Args()
}

func (f *Factory) AddArgs(args ...Arg) error {
	return f.fn.AddArgs(args...)
}

func (f *Factory) Creates() reflect.Type {
	return f.returnedType
}

func (f *Factory) Name() string {
	return f.fn.Name()
}

func (f *Factory) String() string {
	return f.Name()
}

type Method struct {
	fn         *Func
	returnsErr bool
}

func NewMethod(fn any, receiver Arg, args ...Arg) (*Method, error) {
	fnVal := reflect.ValueOf(fn)
	fnName := util.FuncName(fnVal)

	_, ok := receiver.Type().MethodByName(util.FuncNameShort(fnVal))
	if !ok {
		return nil, fmt.Errorf("method %s not found on receiver %s", fnName, util.Signature(receiver.Type()))
	}

	fnType := fnVal.Type()
	if fnType.NumOut() > 1 {
		return nil, fmt.Errorf("method %s must return at most one value", fnName)
	}
	returnsErr := fnType.NumOut() == 1
	if returnsErr && !fnType.Out(0).AssignableTo(errType) {
		return nil, fmt.Errorf("method %s may only return an error, not %s", fnName, util.Signature(fnType.Out(0)))
	}

	f, err := NewFunc(fnVal, append([]Arg{NewSlottedArg(receiver, 0)}, args...)...)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to create method %s", fnName)
	}

	return &Method{fn: f, returnsErr: returnsErr}, nil
}

func (m *Method) Execute(resolver ArgResolver) error {
	out, err := m.fn.Execute(resolver)
	if err != nil {
		return errorsx.Wrap(err, "failed to execute method")
	}

	if m.returnsErr && !out[0].IsNil() {
		return out[0].Interface().(error)
	}
	return nil
}

func (m *Method) Args() *ArgList {
	return m.fn.Args()
}

func (m *Method) AddArgs(args ...Arg) error {
	return m.fn.AddArgs(args...)
}

func (m *Method) Name() string {
	return m.fn.Name()
}

func (m *Method) String() string {
	return m.Name()
}

type Func struct {
	fn      reflect.Value
	args    *ArgList
	returns []reflect.Type
	name    string
}

func NewFunc(fn reflect.Value, args ...Arg) (*Func, error) {
	fnName := util.FuncName(fn)

	fnType := fn.Type()
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("function kind must be func, got %s", fnType.Kind())
	}

	returns := make([]reflect.Type, fnType.NumOut())
	for i := range fnType.NumOut() {
		returns[i] = fnType.Out(i)
	}

	f := &Func{fn: fn, args: NewArgList(fnType), returns: returns, name: fnName}

	err := f.AddArgs(args...)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to add function arguments")
	}

	return f, nil
}

func (f *Func) Execute(resolver ArgResolver) ([]reflect.Value, error) {
	args, err := f.args.ValidateAndCollect()
	if err != nil {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, errorsx.Wrap(err, "failed to compile function arguments")
	}

	resolvedArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		val, err := resolver.Resolve(arg)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to resolve argument %d", i)
		}
		resolvedArgs[i] = reflect.ValueOf(val)
	}

	call := lo.Ternary(f.args.IsVariadic(), f.fn.CallSlice, f.fn.Call)
	return call(resolvedArgs), nil
}

func (f *Func) Args() *ArgList {
	return f.args
}

func (f *Func) AddArgs(args ...Arg) error {
	var joinedErrs error

	for _, arg := range args {
		if _, ok := arg.(*SlottedArg); !ok {
			continue // First fill in all slotted arguments.
		}

		err := f.args.Assign(arg)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
		}
	}
	for _, arg := range args {
		if _, ok := arg.(*SlottedArg); ok {
			continue // Not fill in all non-slotted arguments.
		}

		err := f.args.Assign(arg)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
		}
	}

	return joinedErrs
}

func (f *Func) Type() reflect.Type {
	return f.fn.Type()
}

func (f *Func) Name() string {
	return f.name
}

func (f *Func) String() string {
	return f.Name()
}
