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

func NewContainerBuilder(conf Config) *ContainerBuilder {
	return &ContainerBuilder{
		container: NewContainer(),
		compiler:  NewCompiler(conf.CompilerConfig),
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

// Report records what a compiler pass has to say about the container: an
// argument it objects to, a service it could not build, a warning about wiring
// that will work but probably should not.
//
// This is how a fault reaches the dependency graph. The site says what the
// diagnostic is about, and the graph shows it there — on the argument, the node,
// the scope, or the container itself when it belongs to nothing narrower.
//
// An error-severity diagnostic stops compilation once the reporting pass
// returns, and Build fails with it. A pass that reports one should return nil
// rather than the same error twice.
func (b *ContainerBuilder) Report(d Diagnostic) {
	b.container.report(d)
}

// ReportError records an error a pass has in hand. The diagnostic carries the
// error itself as its message, so the graph shows the fault without the wrapping;
// wrapFormat is how the pass words it for the error Build returns.
func (b *ContainerBuilder) ReportError(site Site, err error, wrapFormat string, a ...any) {
	b.Report(newDiagnosticError(site, err, wrapFormat, a...))
}

// Container is the container being built. It is nil once Build has handed it
// over.
//
// A failed Build keeps it, on purpose. That container shows how far compilation
// got.
func (b *ContainerBuilder) Container() *Container {
	return b.container
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
