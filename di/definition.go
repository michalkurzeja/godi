package di

import (
	"cmp"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Defaults for Definition properties. Change them
// to change the default configuration of services.
// These can be overridden per Definition.
var (
	defaultLazy      = true
	defaultShared    = true
	defaultAutowired = true
)

func SetDefaultLazy(b bool) {
	defaultLazy = b
}

func SetDefaultShared(b bool) {
	defaultShared = b
}

func SetDefaultAutowired(b bool) {
	defaultAutowired = b
}

type ID string

func NewID() ID {
	return ID(uuid.NewString())
}

func (id ID) String() string {
	return string(id)
}

type Label string

func (l Label) String() string {
	return string(l)
}

type ServiceDefinition struct {
	definition[*ServiceDefinition]

	factory     *Factory
	methodCalls map[string]*Method

	// val is the value the definition was built from, for the ones built from
	// one. The factory holding it is godi's own, so it is this that anything
	// reporting on the definition should report.
	val     reflect.Value
	fromVal bool

	shared bool
}

func NewServiceDefinition(factory *Factory) *ServiceDefinition {
	d := &ServiceDefinition{
		factory:     factory,
		methodCalls: make(map[string]*Method),
		shared:      defaultShared,
	}
	d.init(d, captureSource())
	return d
}

func (d *ServiceDefinition) Type() reflect.Type {
	return d.factory.Creates()
}

func (d *ServiceDefinition) Factory() *Factory {
	return d.factory
}

func (d *ServiceDefinition) SetFactory(factory *Factory) *ServiceDefinition {
	d.factory = factory
	return d
}

func (d *ServiceDefinition) MethodCalls() []*Method {
	calls := lo.Values(d.methodCalls)
	slices.SortFunc(calls, func(a, b *Method) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	return calls
}

func (d *ServiceDefinition) SetMethodCalls(methodCalls ...*Method) *ServiceDefinition {
	d.methodCalls = make(map[string]*Method)
	return d.AddMethodCalls(methodCalls...)
}

func (d *ServiceDefinition) AddMethodCalls(methodCalls ...*Method) *ServiceDefinition {
	for _, call := range methodCalls {
		d.methodCalls[call.Name()] = call
	}
	return d
}

func (d *ServiceDefinition) RemoveMethodCalls(names ...string) *ServiceDefinition {
	d.methodCalls = lo.OmitByKeys(d.methodCalls, names)
	return d
}

func (d *ServiceDefinition) IsShared() bool {
	return d.shared
}

func (d *ServiceDefinition) SetShared(shared bool) *ServiceDefinition {
	d.shared = shared
	return d
}

// SetVal records that this definition serves a value it was handed, rather than
// one it builds. The factory is then a wrapper godi wrote to hold it, which is
// nobody's idea of the implementation.
func (d *ServiceDefinition) SetVal(val reflect.Value) *ServiceDefinition {
	d.val, d.fromVal = val, true
	return d
}

// Val is the value this definition was built from, and whether it was built
// from one at all. The value may be invalid even so: nil is a value.
func (d *ServiceDefinition) Val() (reflect.Value, bool) {
	return d.val, d.fromVal
}

func (d *ServiceDefinition) FactoryName() string {
	return d.factory.Name()
}

// Implementation is whatever actually provides the service: the factory that
// builds it, or - when the service was registered as a value - the value
// itself. A function is a value like any other, and one registered that way is
// the whole of the service, so it is what deserves to be named and pointed at.
//
// A value that is not a function has none of this to give. There is no name for
// a struct someone handed over and nowhere to point at for it, and the factory
// holding it is godi's own.
func (d *ServiceDefinition) Implementation() (name string, typ reflect.Type, at Location) {
	impl := d.factory.value()
	if d.fromVal {
		if !d.val.IsValid() || d.val.Kind() != reflect.Func {
			return "", nil, Location{}
		}
		impl = d.val
	}
	return util.FuncName(impl), impl.Type(), declaredAt(impl)
}

func (d *ServiceDefinition) String() string {
	var bld strings.Builder
	if d.factory != nil {
		bld.WriteString(util.Signature(d.Type()))
	} else {
		bld.WriteString("service")
	}
	bld.WriteString(d.labelSuffix())
	return bld.String()
}

type FunctionDefinition struct {
	definition[*FunctionDefinition]

	function *Func
}

func NewFunctionDefinition(function *Func) *FunctionDefinition {
	d := &FunctionDefinition{function: function}
	d.init(d, captureSource())
	return d
}

func (d *FunctionDefinition) Type() reflect.Type {
	return d.function.Type()
}

func (d *FunctionDefinition) Func() *Func {
	return d.function
}

func (d *FunctionDefinition) SetFunc(fn *Func) *FunctionDefinition {
	d.function = fn
	return d
}

// DefinedAt is where the function itself is declared.
func (d *FunctionDefinition) DefinedAt() Location {
	return declaredAt(d.function.value())
}

func (d *FunctionDefinition) String() string {
	var bld strings.Builder
	if d.function != nil {
		bld.WriteString(d.function.Name())
	} else {
		bld.WriteString("function")
	}
	bld.WriteString(d.labelSuffix())
	return bld.String()
}
