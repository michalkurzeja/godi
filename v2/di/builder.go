package di

import "github.com/hashicorp/go-multierror"

func New() *Builder {
	return &Builder{cb: NewContainerBuilder()}
}

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

func (b *Builder) Build() (Container, error) {
	container, err := b.cb.Build()
	return container, multierror.Append(b.err, err).ErrorOrNil()
}

func (b *Builder) addError(err error) {
	b.err = multierror.Append(b.err, err)
}
