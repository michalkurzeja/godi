package di

import (
	"reflect"
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
	id     ID
	labels []Label

	factory     *Factory
	methodCalls map[string]*Method

	// val is the value the definition was built from, for the ones built from
	// one. The factory holding it is godi's own, so it is this that anything
	// reporting on the definition should report.
	val     reflect.Value
	fromVal bool

	scope      *Scope
	childScope *Scope

	// Properties
	lazy      bool
	shared    bool
	autowired bool

	source source
}

func NewServiceDefinition(factory *Factory) *ServiceDefinition {
	return &ServiceDefinition{
		id:      NewID(),
		factory: factory,
		source:  captureSource(),

		methodCalls: make(map[string]*Method),

		lazy:      defaultLazy,
		shared:    defaultShared,
		autowired: defaultAutowired,
	}
}

func (d *ServiceDefinition) ID() ID {
	return d.id
}

func (d *ServiceDefinition) Type() reflect.Type {
	return d.factory.Creates()
}

func (d *ServiceDefinition) Scope() *Scope {
	return d.scope
}

func (d *ServiceDefinition) SetScope(scope *Scope) *ServiceDefinition {
	d.scope = scope
	return d
}

func (d *ServiceDefinition) ChildScope() *Scope {
	return d.childScope
}

func (d *ServiceDefinition) SetChildScope(scope *Scope) *ServiceDefinition {
	d.childScope = scope
	return d
}

// EffectiveScope returns the scope in which all the dependencies should be resolved.
// For most services this is the scope where that service is defined.
// But if a service has a child-scope, then the dependencies should be resolved with that scope included.
func (d *ServiceDefinition) EffectiveScope() *Scope {
	if d.childScope != nil {
		return d.childScope
	}
	return d.scope
}

func (d *ServiceDefinition) Factory() *Factory {
	return d.factory
}

func (d *ServiceDefinition) SetFactory(factory *Factory) *ServiceDefinition {
	d.factory = factory
	return d
}

func (d *ServiceDefinition) MethodCalls() []*Method {
	return util.SortedAsc(lo.Values(d.methodCalls), func(m *Method) string {
		return m.Name()
	})
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

func (d *ServiceDefinition) Labels() []Label {
	return d.labels
}

func (d *ServiceDefinition) SetLabels(labels ...Label) *ServiceDefinition {
	d.labels = labels
	return d
}

func (d *ServiceDefinition) AddLabels(labels ...Label) *ServiceDefinition {
	d.labels = append(d.labels, labels...)
	return d
}

func (d *ServiceDefinition) RemoveLabels(labels ...Label) *ServiceDefinition {
	d.labels = lo.Without(d.labels, labels...)
	return d
}

func (d *ServiceDefinition) IsLazy() bool {
	return d.lazy
}

func (d *ServiceDefinition) SetLazy(lazy bool) *ServiceDefinition {
	d.lazy = lazy
	return d
}

func (d *ServiceDefinition) IsShared() bool {
	return d.shared
}

func (d *ServiceDefinition) SetShared(shared bool) *ServiceDefinition {
	d.shared = shared
	return d
}

func (d *ServiceDefinition) IsAutowired() bool {
	return d.autowired
}

func (d *ServiceDefinition) SetAutowired(autowired bool) *ServiceDefinition {
	d.autowired = autowired
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

// RegisteredAt is where this definition was declared: the first frame outside
// godi at the moment it was created.
func (d *ServiceDefinition) RegisteredAt() Location {
	return d.source.Location()
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
	if len(d.labels) > 0 {
		bld.WriteString(" (")
		for i, label := range d.labels {
			if i > 0 {
				bld.WriteString(", ")
			}
			bld.WriteString(label.String())
		}
		bld.WriteString(")")
	}
	return bld.String()
}

type FunctionDefinition struct {
	id       ID
	function *Func
	labels   []Label

	scope      *Scope
	childScope *Scope

	// Properties
	lazy      bool
	autowired bool

	source source
}

func NewFunctionDefinition(function *Func) *FunctionDefinition {
	return &FunctionDefinition{
		id:       NewID(),
		function: function,
		source:   captureSource(),

		lazy:      defaultLazy,
		autowired: defaultAutowired,
	}
}

func (d *FunctionDefinition) ID() ID {
	return d.id
}

func (d *FunctionDefinition) Type() reflect.Type {
	return d.function.Type()
}

func (d *FunctionDefinition) Scope() *Scope {
	return d.scope
}

func (d *FunctionDefinition) SetScope(scope *Scope) *FunctionDefinition {
	d.scope = scope
	return d
}

func (d *FunctionDefinition) ChildScope() *Scope {
	return d.childScope
}

func (d *FunctionDefinition) SetChildScope(scope *Scope) *FunctionDefinition {
	d.childScope = scope
	return d
}

// EffectiveScope returns the scope in which all the dependencies should be resolved.
// For most services this is the scope where that service is defined.
// But if a service has a child-scope, then the dependencies should be resolved with that scope includes.
func (d *FunctionDefinition) EffectiveScope() *Scope {
	if d.childScope != nil {
		return d.childScope
	}
	return d.scope
}

func (d *FunctionDefinition) Func() *Func {
	return d.function
}

func (d *FunctionDefinition) SetFunc(fn *Func) *FunctionDefinition {
	d.function = fn
	return d
}

func (d *FunctionDefinition) Labels() []Label {
	return d.labels
}

func (d *FunctionDefinition) SetLabels(labels ...Label) *FunctionDefinition {
	d.labels = labels
	return d
}

func (d *FunctionDefinition) AddLabels(labels ...Label) *FunctionDefinition {
	d.labels = append(d.labels, labels...)
	return d
}

func (d *FunctionDefinition) RemoveLabels(labels ...Label) *FunctionDefinition {
	d.labels = lo.Without(d.labels, labels...)
	return d
}

func (d *FunctionDefinition) IsLazy() bool {
	return d.lazy
}

func (d *FunctionDefinition) SetLazy(lazy bool) *FunctionDefinition {
	d.lazy = lazy
	return d
}

func (d *FunctionDefinition) IsAutowired() bool {
	return d.autowired
}

func (d *FunctionDefinition) SetAutowired(autowired bool) *FunctionDefinition {
	d.autowired = autowired
	return d
}

// RegisteredAt is where this definition was declared: the first frame outside
// godi at the moment it was created.
func (d *FunctionDefinition) RegisteredAt() Location {
	return d.source.Location()
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
	if len(d.labels) > 0 {
		bld.WriteString(" (")
		for i, label := range d.labels {
			if i > 0 {
				bld.WriteString(", ")
			}
			bld.WriteString(label.String())
		}
		bld.WriteString(")")
	}
	return bld.String()
}
