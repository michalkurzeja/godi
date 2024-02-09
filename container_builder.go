package di

import (
	"errors"

	"github.com/samber/lo"
)

// ContainerBuilder is a builder for Container. It provides a fluent interface to
// inspect and configure the container.
// Once Build() is called, this builder is locked and no longer usable. Subsequent calls
// to Build() will return an error and any other method may panic.
type ContainerBuilder struct {
	container *container
	compiler  *compiler

	built bool
}

func NewContainerBuilder() *ContainerBuilder {
	return &ContainerBuilder{
		container: newContainer(),
		compiler:  newCompiler(),
	}
}

func (b *ContainerBuilder) GetFunctions() []*FunctionDefinition {
	return sortedAsc(lo.Values(b.container.functions), func(fn *FunctionDefinition) ID {
		return fn.ID()
	})
}

func (b *ContainerBuilder) GetFunction(id ID) (*FunctionDefinition, bool) {
	fn, ok := b.container.functions[id]
	return fn, ok
}

func (b *ContainerBuilder) SetFunctions(functions ...*FunctionDefinition) {
	b.container.functions = make(map[ID]*FunctionDefinition, len(functions))
	b.AddFunctions(functions...)
}

func (b *ContainerBuilder) AddFunctions(functions ...*FunctionDefinition) {
	for _, fn := range functions {
		b.container.functions[fn.id] = fn
	}
}

func (b *ContainerBuilder) GetDefinitions() []*Definition {
	return sortedAsc(lo.Values(b.container.definitions), func(def *Definition) ID {
		return def.ID()
	})
}

func (b *ContainerBuilder) GetDefinition(id ID) (*Definition, bool) {
	def, ok := b.container.definitions[id]
	return def, ok
}

func (b *ContainerBuilder) SetDefinitions(definitions ...*Definition) *ContainerBuilder {
	b.container.definitions = make(map[ID]*Definition)
	return b.AddDefinitions(definitions...)
}

func (b *ContainerBuilder) AddDefinitions(definitions ...*Definition) *ContainerBuilder {
	for _, def := range definitions {
		b.container.definitions[def.id] = def
	}
	return b
}

func (b *ContainerBuilder) RemoveDefinitions(ids ...ID) *ContainerBuilder {
	b.container.definitions = lo.OmitByKeys(b.container.definitions, ids)
	return b
}

func (b *ContainerBuilder) GetAliases() []Alias {
	return sortedAsc(lo.Values(b.container.aliases), func(a Alias) ID {
		return a.ID()
	})
}

func (b *ContainerBuilder) GetAlias(id ID) (Alias, bool) {
	alias, ok := b.container.aliases[id]
	return alias, ok
}

func (b *ContainerBuilder) SetAliases(aliases ...Alias) *ContainerBuilder {
	b.container.aliases = make(map[ID]Alias)
	return b.AddAliases(aliases...)
}

func (b *ContainerBuilder) AddAliases(aliases ...Alias) *ContainerBuilder {
	for _, alias := range aliases {
		b.container.aliases[alias.id] = alias
	}
	return b
}

func (b *ContainerBuilder) RemoveAliases(ids ...ID) *ContainerBuilder {
	b.container.aliases = lo.OmitByKeys(b.container.aliases, ids)
	return b
}

func (b *ContainerBuilder) AddCompilerPass(stage CompilerPassStage, priority int, pass CompilerPass) *ContainerBuilder {
	b.compiler.AddPass(stage, priority, pass)
	return b
}

func (b *ContainerBuilder) Build() (Container, error) {
	if b.built {
		return nil, errors.New("container already built")
	}
	b.built = true

	err := b.compiler.Compile(b)
	if err != nil {
		return nil, err
	}

	container := b.container
	b.container = nil

	return container, nil
}
