package di

import (
	"errors"
	"fmt"
	"reflect"

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

// Execute builds the service. As during a build, the dependencies it is handed
// have not had their method calls run yet.
func (f *Factory) Execute(scope *Scope) (any, error) {
	return withInstantiationContext(scope.container, func(ic *instantiationContext) (any, error) {
		return f.execute(ic, scope)
	})
}

func (f *Factory) execute(ic *instantiationContext, scope *Scope) (any, error) {
	args, err := f.fn.resolveArgs(ic, scope, nil)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to execute factory")
	}

	out := f.fn.call(args)
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

// Type is the factory function's own type, as against what it creates.
func (f *Factory) Type() reflect.Type {
	return f.fn.Type()
}

// value is the factory function itself, for reading where it was declared.
func (f *Factory) value() reflect.Value {
	return f.fn.value()
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

// Execute resolves the receiver along with the rest of the arguments and calls
// the method on it.
func (m *Method) Execute(scope *Scope) error {
	_, err := withInstantiationContext(scope.container, func(ic *instantiationContext) (any, error) {
		return nil, m.execute(ic, scope, nil)
	})
	return err
}

// execute calls the method. A non-nil recv is the receiver to call it on; nil
// means resolve the receiver like any other argument.
//
// A queued call passes the instance godi built. A service that is not shared is
// never published, so resolving its receiver again would build a second one.
func (m *Method) execute(ic *instantiationContext, scope *Scope, recv any) error {
	args, err := m.fn.resolveArgs(ic, scope, recv)
	if err != nil {
		return errorsx.Wrap(err, "failed to execute method")
	}

	out := m.fn.call(args)
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

// Execute resolves the arguments and calls the function. The services it is
// handed are fully configured by the time it runs.
func (f *Func) Execute(scope *Scope) ([]reflect.Value, error) {
	return withInstantiationContext(scope.container, func(ic *instantiationContext) ([]reflect.Value, error) {
		args, err := f.resolveArgs(ic, scope, nil)
		if err != nil {
			return nil, err
		}
		if err := ic.commit(); err != nil {
			return nil, err
		}
		return f.call(args), nil
	})
}

// resolveArgs produces the values to pass. A non-nil recv fills argument 0
// instead of being resolved.
func (f *Func) resolveArgs(ic *instantiationContext, scope *Scope, recv any) ([]reflect.Value, error) {
	args, err := f.args.ValidateAndCollect()
	if err != nil {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, errorsx.Wrap(err, "failed to compile function arguments")
	}

	resolvedArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		if i == 0 && recv != nil {
			resolvedArgs[i] = reflect.ValueOf(recv)
			continue
		}

		val, err := resolveArg(ic, scope, arg)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to resolve argument %d", i)
		}
		resolvedArgs[i] = reflect.ValueOf(val)
	}

	return resolvedArgs, nil
}

func (f *Func) call(args []reflect.Value) []reflect.Value {
	return callUserCode(f.fn, args, f.args.IsVariadic())
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

// value is the function itself, for reading where it was declared.
func (f *Func) value() reflect.Value {
	return f.fn
}

func (f *Func) Name() string {
	return f.name
}

func (f *Func) String() string {
	return f.Name()
}
