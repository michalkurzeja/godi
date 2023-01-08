package di

import (
	"reflect"

	"github.com/samber/lo"
)

type ID string
type Tag string

type Definition struct {
	id          ID
	factory     *Factory
	methodCalls map[string]*Method

	tags []Tag

	public   bool
	lazy     bool
	cached   bool
	autowire bool
}

func NewDefinition(id ID, factory *Factory) *Definition {
	return &Definition{
		id:          id,
		factory:     factory,
		methodCalls: make(map[string]*Method),

		public:   false,
		lazy:     true,
		cached:   true,
		autowire: true,
	}
}

func (d *Definition) ID() ID {
	return d.id
}

func (d *Definition) Of() reflect.Type {
	return d.factory.creates
}

func (d *Definition) GetFactory() *Factory {
	return d.factory
}

func (d *Definition) SetFactory(factory *Factory) *Definition {
	d.factory = factory
	return d
}

func (d *Definition) GetMethodCalls() []*Method {
	return lo.Values(d.methodCalls)
}

func (d *Definition) SetMethodCalls(methodCalls ...*Method) *Definition {
	d.methodCalls = make(map[string]*Method)
	return d.AddMethodCalls(methodCalls...)
}

func (d *Definition) AddMethodCalls(methodCalls ...*Method) *Definition {
	for _, call := range methodCalls {
		d.methodCalls[call.Name()] = call
	}
	return d
}

func (d *Definition) RemoveMethodCalls(names ...string) *Definition {
	d.methodCalls = lo.OmitByKeys(d.methodCalls, names)
	return d
}

func (d *Definition) GetTags() []Tag {
	return d.tags
}

func (d *Definition) SetTags(tags ...Tag) *Definition {
	d.tags = tags
	return d
}

func (d *Definition) AddTags(tags ...Tag) *Definition {
	d.tags = append(d.tags, tags...)
	return d
}

func (d *Definition) RemoveTags(tags ...Tag) *Definition {
	d.tags = lo.Without(d.tags, tags...)
	return d
}

func (d *Definition) IsPublic() bool {
	return d.public
}

func (d *Definition) SetPublic(public bool) *Definition {
	d.public = public
	return d
}

func (d *Definition) IsLazy() bool {
	return d.lazy
}

func (d *Definition) SetLazy(lazy bool) *Definition {
	d.lazy = lazy
	return d
}

func (d *Definition) IsCached() bool {
	return d.cached
}

func (d *Definition) SetCached(cached bool) *Definition {
	d.cached = cached
	return d
}

func (d *Definition) IsAutowire() bool {
	return d.autowire
}

func (d *Definition) SetAutowire(autowire bool) *Definition {
	d.autowire = autowire
	return d
}

func (d *Definition) String() string {
	return string(d.ID())
}
