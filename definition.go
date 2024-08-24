package godi

import (
	"errors"
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

// ServiceDefinitionBuilder is a helper for building di.ServiceDefinition objects.
// It offers a fluent interface that does all the heavy lifting for the user.
// This is the recommended way of building a di.ServiceDefinition.
type ServiceDefinitionBuilder struct {
	def        *di.ServiceDefinition
	setFactory func() error
	addMethods []func() error
}

// Svc creates a new ServiceDefinitionBuilder.
func Svc(factory any, args ...any) *ServiceDefinitionBuilder {

	b := &ServiceDefinitionBuilder{
		def: di.NewServiceDefinition(nil),
	}
	b.setFactory = func() error {
		args, err := parseArgs(args)
		if err != nil {
			return err
		}
		f, err := di.NewFactory(factory, args...)
		if err != nil {
			return err
		}
		b.def.SetFactory(f)
		return nil
	}
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
	b.addMethods = append(b.addMethods, func() error {
		args, err := parseArgs(args)
		if err != nil {
			return err
		}
		m, err := di.NewMethod(method, di.NewRefArg(b.def), args...)
		if err != nil {
			return err
		}
		b.def.AddMethodCalls(m)
		return nil
	})
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

func (b *ServiceDefinitionBuilder) Build() (def *di.ServiceDefinition, joinedErrs error) {
	err := b.setFactory()
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, err)
	}

	for _, getMethod := range b.addMethods {
		err := getMethod()
		if err != nil {
			joinedErrs = errors.Join(joinedErrs, err)
		}
	}

	if joinedErrs != nil {
		return nil, errorsx.Wrapf(joinedErrs, "invalid definition of %s", b.def)
	}

	return b.def, nil
}

// FunctionDefinitionBuilder is a helper for building di.FunctionDefinition objects.
// It offers a fluent interface that does all the heavy lifting for the user.
// This is the recommended way of building a di.FunctionDefinition.
type FunctionDefinitionBuilder struct {
	def     *di.FunctionDefinition
	setFunc func() error
}

// Func creates a new FunctionDefinitionBuilder.
func Func(fn any, args ...any) *FunctionDefinitionBuilder {
	b := &FunctionDefinitionBuilder{
		def: di.NewFunctionDefinition(nil),
	}
	b.setFunc = func() error {
		args, err := parseArgs(args)
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

func (b *FunctionDefinitionBuilder) Build() (def *di.FunctionDefinition, joinedErrs error) {
	err := b.setFunc()
	if err != nil {
		joinedErrs = errors.Join(joinedErrs, err)
	}

	if joinedErrs != nil {
		return nil, errorsx.Wrapf(joinedErrs, "invalid definition of %s", b.def)
	}

	return b.def, nil
}

func parseArgs(args []any) ([]di.Arg, error) {
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
