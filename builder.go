package di

import (
	"github.com/hashicorp/go-multierror"
)

// New creates a new Builder.
// This is the recommended entrypoint to the godi library.
func New() *Builder {
	return &Builder{cb: NewContainerBuilder()}
}

// Builder is a helper for building a container.
// It offers a fluent interface that incorporates other helpers to make
// the process of setting up the container easy and convenient for the user.
// This is the recommended way of building a container.
type Builder struct {
	cb  *ContainerBuilder
	err *multierror.Error
}

func (b *Builder) Aliases(aliases ...Alias) *Builder {
	b.cb.AddAliases(aliases...)
	return b
}

func (b *Builder) Services(services ...*DefinitionBuilder) *Builder {
	for _, builder := range services {
		def, err := builder.Build()
		if err != nil {
			b.addError(err)
			continue
		}
		b.cb.AddDefinitions(def)
	}
	return b
}

func (b *Builder) Functions(functions ...*FunctionDefinitionBuilder) *Builder {
	for _, function := range functions {
		fn, err := function.Build()
		if err != nil {
			b.addError(err)
			continue
		}
		b.cb.AddFunctions(fn)
	}
	return b
}

func (b *Builder) CompilerPass(stage CompilerPassStage, priority int, pass CompilerPass) *Builder {
	b.cb.AddCompilerPass(stage, priority, pass)
	return b
}

func (b *Builder) Build() (Container, error) {
	container, err := b.cb.Build()
	return container, multierror.Append(b.err, err).ErrorOrNil()
}

func (b *Builder) addError(err error) {
	b.err = multierror.Append(b.err, err)
}
