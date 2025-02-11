package godi

import (
	"errors"

	"github.com/michalkurzeja/godi/v2/di"
)

// New creates a new Builder.
// This is the recommended entrypoint to the godi library.
func New() *Builder {
	return &Builder{cb: di.NewContainerBuilder()}
}

// Builder is a helper for building a container.
// It offers a fluent interface that incorporates other helpers to make
// the process of setting up the container easy and convenient for the user.
// This is the recommended way of building a container.
type Builder struct {
	cb *di.ContainerBuilder

	services  []*ServiceDefinitionBuilder
	functions []*FunctionDefinitionBuilder
	bindings  []*InterfaceBindingBuilder
	passes    []*di.CompilerPass
}

func (b *Builder) Services(services ...*ServiceDefinitionBuilder) *Builder {
	b.services = append(b.services, services...)
	return b
}

func (b *Builder) Functions(functions ...*FunctionDefinitionBuilder) *Builder {
	b.functions = append(b.functions, functions...)
	return b
}

func (b *Builder) Bindings(bindings ...*InterfaceBindingBuilder) *Builder {
	b.bindings = append(b.bindings, bindings...)
	return b
}

func (b *Builder) CompilerPasses(passes ...*di.CompilerPass) *Builder {
	b.passes = append(b.passes, passes...)
	return b
}

func (b *Builder) Build() (Container, error) {
	var joinedErr error

	for _, builder := range b.services {
		if err := builder.ParseFactory(); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
	}

	for _, builder := range b.services {
		if err := builder.Build(b.cb.RootScope()); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
	}

	for _, builder := range b.functions {
		if err := builder.Build(b.cb.RootScope()); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
	}

	for _, builder := range b.bindings {
		if err := builder.Build(b.cb.RootScope()); err != nil {
			joinedErr = errors.Join(joinedErr, err)
			continue
		}
	}

	for _, pass := range b.passes {
		b.cb.Compiler().AddPass(pass)
	}

	container, err := b.cb.Build()
	return container, errors.Join(joinedErr, err)
}
