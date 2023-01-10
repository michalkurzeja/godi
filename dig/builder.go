package dig

import (
	di "github.com/michalkurzeja/godi"
)

type Builder struct {
	b *di.Builder
}

func (b *Builder) Aliases(aliases ...di.Alias) *Builder {
	b.b.Aliases(aliases...)
	return b
}

func (b *Builder) Services(services ...*di.DefinitionBuilder) *Builder {
	b.b.Services(services...)
	return b
}

func (b *Builder) CompilerPass(stage di.CompilerPassStage, priority int, pass di.CompilerPass) *Builder {
	b.b.CompilerPass(stage, priority, pass)
	return b
}

func (b *Builder) Build() (err error) {
	c, err = b.b.Build()
	return err
}
