package di

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

type ID = di.ID
type Label = di.Label

type SvcReference struct {
	def *di.ServiceDefinition
}

func (r SvcReference) SvcID() ID {
	if r.def == nil {
		return ""
	}
	return r.def.ID()
}

func (r SvcReference) IsEmpty() bool {
	return r.def == nil
}

func (r SvcReference) String() string {
	if r.def == nil {
		return "<empty reference>"
	}
	return r.def.String()
}

type FuncReference struct {
	def *di.FunctionDefinition
}

func (r FuncReference) FuncID() ID {
	return r.def.ID()
}

func (r FuncReference) IsEmpty() bool {
	return r.def == nil
}

func (r FuncReference) String() string {
	if r.def == nil {
		return "<empty reference>"
	}
	return r.def.String()
}

type funcBuilder struct {
	fn   any
	args []any
}

// ServiceDefinitionBuilder is a helper for building di.ServiceDefinition objects.
// It offers a fluent interface that does all the heavy lifting for the user.
// This is the recommended way of building a di.ServiceDefinition.
type ServiceDefinitionBuilder struct {
	def      *di.ServiceDefinition
	factory  *funcBuilder
	methods  []*funcBuilder
	children []*ServiceDefinitionBuilder

	factoryParsed bool
}

// Svc creates a new ServiceDefinitionBuilder.
func Svc(factory any, args ...any) *ServiceDefinitionBuilder {
	b := &ServiceDefinitionBuilder{
		def: di.NewServiceDefinition(nil),
	}
	b.factory = &funcBuilder{fn: factory, args: args}
	return b
}

func SvcVal[T any](svc T) *ServiceDefinitionBuilder {
	return Svc(func() T { return svc })
}

func (b *ServiceDefinitionBuilder) Bind(ref *SvcReference) *ServiceDefinitionBuilder {
	ref.def = b.def
	return b
}

func (b *ServiceDefinitionBuilder) MethodCall(method any, args ...any) *ServiceDefinitionBuilder {
	b.methods = append(b.methods, &funcBuilder{fn: method, args: args})
	return b
}

func (b *ServiceDefinitionBuilder) Labels(labels ...Label) *ServiceDefinitionBuilder {
	b.def.SetLabels(labels...)
	return b
}

func (b *ServiceDefinitionBuilder) Lazy() *ServiceDefinitionBuilder {
	b.def.SetLazy(true)
	return b
}

func (b *ServiceDefinitionBuilder) Eager() *ServiceDefinitionBuilder {
	b.def.SetLazy(false)
	return b
}

func (b *ServiceDefinitionBuilder) Shared() *ServiceDefinitionBuilder {
	b.def.SetShared(true)
	return b
}

func (b *ServiceDefinitionBuilder) NotShared() *ServiceDefinitionBuilder {
	b.def.SetShared(false)
	return b
}

func (b *ServiceDefinitionBuilder) Autowired() *ServiceDefinitionBuilder {
	b.def.SetAutowired(true)
	return b
}

func (b *ServiceDefinitionBuilder) NotAutowired() *ServiceDefinitionBuilder {
	b.def.SetAutowired(false)
	return b
}

func (b *ServiceDefinitionBuilder) Children(services ...*ServiceDefinitionBuilder) *ServiceDefinitionBuilder {
	b.children = append(b.children, services...)
	return b
}

// parseFactory parses the factory function WITHOUT the arguments to determine the service type.
// This method MUST be called prior to build.
func (b *ServiceDefinitionBuilder) parseFactory() (joinedErrs error) {
	f, err := di.NewFactory(b.factory.fn)
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "failed to build factory"))
	} else {
		b.def.SetFactory(f)
	}

	for _, child := range b.children {
		err := child.parseFactory()
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "invalid child"))
		}
	}

	if joinedErrs != nil {
		return errorsx.Wrapf(joinedErrs, "invalid definition of %s", b.def)
	}

	b.factoryParsed = true

	return nil
}

func (b *ServiceDefinitionBuilder) build(scope *di.Scope) (joinedErrs error) {
	if !b.factoryParsed {
		return fmt.Errorf("failed to build service definition: factory of %s is not parsed", b.def)
	}

	args, err := buildArgs(b.factory.args)
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "failed to build factory args"))
	}
	err = b.def.Factory().AddArgs(args...)
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "failed to add factory args"))
	}

	for _, method := range b.methods {
		args, err := buildArgs(method.args)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
			continue
		}
		receiver, err := di.NewRefArg(b.def)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
			continue
		}
		m, err := di.NewMethod(method.fn, receiver, args...)
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
		} else {
			b.def.AddMethodCalls(m)
		}
	}

	if len(b.children) > 0 {
		childScope := scope.NewChild(b.def.String())

		for _, child := range b.children {
			err := child.build(childScope)
			if err != nil {
				joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "invalid child"))
			}
		}
		b.def.SetChildScope(childScope)
	}

	if joinedErrs != nil {
		return errorsx.Wrapf(joinedErrs, "invalid definition of %s", b.def)
	}

	scope.AddServiceDefinitions(b.def)
	b.def.SetScope(scope)

	return nil
}

// FunctionDefinitionBuilder is a helper for building di.FunctionDefinition objects.
// It offers a fluent interface that does all the heavy lifting for the user.
// This is the recommended way of building a di.FunctionDefinition.
type FunctionDefinitionBuilder struct {
	def      *di.FunctionDefinition
	setFunc  func() error
	children []*ServiceDefinitionBuilder
}

// Func creates a new FunctionDefinitionBuilder.
func Func(fn any, args ...any) *FunctionDefinitionBuilder {
	b := &FunctionDefinitionBuilder{
		def: di.NewFunctionDefinition(nil),
	}
	b.setFunc = func() error {
		args, err := buildArgs(args)
		if err != nil {
			return err
		}
		f, err := di.NewFunc(reflect.ValueOf(fn), args...)
		if err != nil {
			return err
		}
		b.def.SetFunc(f)
		return nil
	}
	return b
}

func (b *FunctionDefinitionBuilder) Bind(ref *FuncReference) *FunctionDefinitionBuilder {
	ref.def = b.def
	return b
}

func (b *FunctionDefinitionBuilder) Labels(labels ...Label) *FunctionDefinitionBuilder {
	b.def.SetLabels(labels...)
	return b
}

func (b *FunctionDefinitionBuilder) Lazy() *FunctionDefinitionBuilder {
	b.def.SetLazy(true)
	return b
}

func (b *FunctionDefinitionBuilder) Eager() *FunctionDefinitionBuilder {
	b.def.SetLazy(false)
	return b
}

func (b *FunctionDefinitionBuilder) Autowired() *FunctionDefinitionBuilder {
	b.def.SetAutowired(true)
	return b
}

func (b *FunctionDefinitionBuilder) NotAutowired() *FunctionDefinitionBuilder {
	b.def.SetAutowired(false)
	return b
}

func (b *FunctionDefinitionBuilder) Children(services ...*ServiceDefinitionBuilder) *FunctionDefinitionBuilder {
	b.children = append(b.children, services...)
	return b
}

func (b *FunctionDefinitionBuilder) build(scope *di.Scope) (joinedErrs error) {
	err := b.setFunc()
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, err)
	}

	if len(b.children) > 0 {
		childScope := scope.NewChild(b.def.String())

		for _, child := range b.children {
			err := child.build(childScope)
			if err != nil {
				joinedErrs = errors.Join(joinedErrs, errorsx.Wrap(err, "invalid child"))
			}
		}

		b.def.SetChildScope(childScope)
	}

	if joinedErrs != nil {
		return errorsx.Wrapf(joinedErrs, "invalid definition of %s", b.def)
	}

	scope.AddFunctionDefinitions(b.def)
	b.def.SetScope(scope)

	return nil
}

func buildArgs(args []any) ([]di.Arg, error) {
	parsedArgs := make([]di.Arg, 0, len(args))
	for _, arg := range args {
		parsed, err := Arg(arg).Build()
		if err != nil {
			return nil, err
		}
		parsedArgs = append(parsedArgs, parsed)
	}
	return parsedArgs, nil
}
