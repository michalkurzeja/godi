package di

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

type DefinitionBuilder struct {
	def *Definition
	err *multierror.Error
}

func SvcT[T any](factory any) *DefinitionBuilder {
	return newDefinitionBuilder(NewFactoryT[T], factory)
}

func Svc(factory any) *DefinitionBuilder {
	return newDefinitionBuilder(NewAutoFactory, factory)
}

func newDefinitionBuilder(newFactory func(fn any, args ...Argument) (*Factory, error), factory any) *DefinitionBuilder {
	b := &DefinitionBuilder{}

	f, err := newFactory(factory)
	if err != nil {
		b.def = NewDefinition("", f)
		b.addError(err)
	} else {
		b.def = NewDefinition(fqn(f.creates), f)
	}

	return b
}

func (b *DefinitionBuilder) ID(id ID) *DefinitionBuilder {
	b.def.id = id
	return b
}

func (b *DefinitionBuilder) Args(args ...*ArgumentBuilder) *DefinitionBuilder {
	for _, argBuilder := range args {
		err := b.def.factory.args.SetAutomatically(argBuilder.Build())
		if err != nil {
			b.addError(err)
		}
	}
	return b
}

func (b *DefinitionBuilder) MethodCall(name string, args ...*ArgumentBuilder) *DefinitionBuilder {
	method, ok := b.creates().MethodByName(name)
	if !ok {
		b.addError(fmt.Errorf("no such method: %s", name))
		return b
	}

	methodArgs := lo.Map(args, func(builder *ArgumentBuilder, _ int) Argument {
		return builder.Build()
	})

	m, err := NewMethod(method, methodArgs...)
	if err != nil {
		b.addError(err)
		return b
	}

	b.def.AddMethodCalls(m)
	return b
}

func (b *DefinitionBuilder) Tags(tags ...Tag) *DefinitionBuilder {
	b.def.AddTags(tags...)
	return b
}

func (b *DefinitionBuilder) Public() *DefinitionBuilder {
	b.def.SetPublic(true)
	return b
}

func (b *DefinitionBuilder) Private() *DefinitionBuilder {
	b.def.SetPublic(false)
	return b
}

func (b *DefinitionBuilder) Lazy() *DefinitionBuilder {
	b.def.SetLazy(true)
	return b
}

func (b *DefinitionBuilder) Eager() *DefinitionBuilder {
	b.def.SetLazy(false)
	return b
}

func (b *DefinitionBuilder) Cached() *DefinitionBuilder {
	b.def.SetCached(true)
	return b
}

func (b *DefinitionBuilder) Uncached() *DefinitionBuilder {
	b.def.SetCached(false)
	return b
}

func (b *DefinitionBuilder) Autowired() *DefinitionBuilder {
	b.def.SetAutowire(true)
	return b
}

func (b *DefinitionBuilder) Manual() *DefinitionBuilder {
	b.def.SetAutowire(false)
	return b
}

func (b *DefinitionBuilder) Build() (*Definition, error) {
	if b.err.ErrorOrNil() != nil {
		return nil, b.err.ErrorOrNil()
	}

	return b.def, nil
}

func (b *DefinitionBuilder) creates() reflect.Type {
	return b.def.factory.creates
}

func (b *DefinitionBuilder) addError(err error) {
	id := b.def.ID()
	if id == "" {
		b.err = multierror.Append(b.err, fmt.Errorf("invalid definition: %w", err))
	} else {
		b.err = multierror.Append(b.err, fmt.Errorf("invalid definition of %s: %w", b.def, err))
	}
}
