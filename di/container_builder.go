package di

import (
	"errors"
	"iter"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/iterx"
)

// ContainerBuilder is a builder for Container. It provides a fluent interface to
// inspect and configure the container.
// Once Build() is called, this builder is locked and no longer usable. Subsequent calls
// to Build() will return an error and any other method may panic.
type ContainerBuilder struct {
	container *Container
	compiler  *Compiler

	built bool
}

func NewContainerBuilder() *ContainerBuilder {
	return &ContainerBuilder{
		container: NewContainer(),
		compiler:  NewCompiler(),
	}
}

func (b *ContainerBuilder) RootScope() *Scope {
	return b.container.root
}

func (b *ContainerBuilder) Scope(name string) (*Scope, bool) {
	return b.container.scopes.Get(name)
}

func (b *ContainerBuilder) Scopes() iter.Seq[*Scope] {
	return iterx.Values(b.container.scopes.Iterator())
}

func (b *ContainerBuilder) ServiceDefinitionsSeq() iter.Seq2[*Scope, *ServiceDefinition] {
	return func(yield func(*Scope, *ServiceDefinition) bool) {
		for scope := range b.Scopes() {
			for def := range scope.ServiceDefinitionsSeq() {
				if !yield(scope, def) {
					return
				}
			}
		}
	}
}

func (b *ContainerBuilder) FunctionDefinitionsSeq() iter.Seq2[*Scope, *FunctionDefinition] {
	return func(yield func(*Scope, *FunctionDefinition) bool) {
		for scope := range b.Scopes() {
			for def := range scope.FunctionDefinitionsSeq() {
				if !yield(scope, def) {
					return
				}
			}
		}
	}
}

func (b *ContainerBuilder) Compiler() *Compiler {
	return b.compiler
}

func (b *ContainerBuilder) Build() (*Container, error) {
	if b.built {
		return nil, errors.New("container already built")
	}
	b.built = true

	err := b.compiler.Run(b)
	if err != nil {
		return nil, errorsx.Wrap(err, "compilation failed")
	}

	container := b.container
	b.container = nil

	return container, nil
}
