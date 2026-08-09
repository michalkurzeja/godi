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

// The values NewDefaults returns. Nothing else reads them.
var (
	defaultLazy      = true
	defaultShared    = true
	defaultAutowired = true
)

// SetDefaultLazy sets the default for every container in the process.
//
// Deprecated: use Config.Defaults, which is per container. A package-level
// default means two containers in one process cannot disagree, a library that
// sets one changes its host's containers, and a test that flips one leaks into
// the next.
func SetDefaultLazy(b bool) {
	defaultLazy = b
}

// SetDefaultShared sets the default for every container in the process.
//
// Deprecated: use Config.Defaults, which is per container.
func SetDefaultShared(b bool) {
	defaultShared = b
}

// SetDefaultAutowired sets the default for every container in the process.
//
// Deprecated: use Config.Defaults, which is per container.
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

	// val is the value this definition was handed, rather than one it builds.
	// fromVal says whether there is one. Checking val is not enough: a nil
	// interface leaves it invalid.
	val     reflect.Value
	fromVal bool

	shared property
}

func NewServiceDefinition(factory *Factory) *ServiceDefinition {
	d := &ServiceDefinition{
		factory:     factory,
		methodCalls: make(map[string]*Method),
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
	return d.shared.val
}

func (d *ServiceDefinition) SetShared(shared bool) *ServiceDefinition {
	d.shared.set(shared)
	return d
}

func (d *ServiceDefinition) applyDefaults(defaults Defaults) {
	d.definition.applyDefaults(defaults)
	d.shared.fillDefault(defaults.Shared)
}

// SetVal records that this definition serves a value it was handed, rather than
// one it builds. The factory is then only a wrapper godi wrote to hold the
// value.
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

// DeclaredAt is where the factory function is written, as against RegisteredAt,
// which is where this definition was created.
func (d *ServiceDefinition) DeclaredAt() Location {
	return declaredAt(d.factory.value())
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

// DeclaredAt is where the function itself is written, as against RegisteredAt,
// which is where this definition was created.
func (d *FunctionDefinition) DeclaredAt() Location {
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
