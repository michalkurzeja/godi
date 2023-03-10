package di

import (
	"reflect"

	"github.com/samber/lo"
)

// Defaults for Definition properties. Change them
// to change the default configuration of services.
// These can be overridden per Definition.
var (
	DefaultPublic    = false
	DefaultLazy      = true
	DefaultShared    = true
	DefaultAutowired = true
)

type ID string

// Definition describes a service. It stores all information needed to build
// an instance of a service, and it tells the container how to handle the service.
type Definition struct {
	id          ID
	factory     *Factory
	methodCalls map[string]*Method

	tags map[TagID]*Tag

	public    bool
	lazy      bool
	shared    bool
	autowired bool
}

func NewDefinition(id ID, factory *Factory) *Definition {
	return &Definition{
		id:          id,
		factory:     factory,
		methodCalls: make(map[string]*Method),

		tags: make(map[TagID]*Tag),

		public:    DefaultPublic,
		lazy:      DefaultLazy,
		shared:    DefaultShared,
		autowired: DefaultAutowired,
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
	return sortedAsc(lo.Values(d.methodCalls), func(m *Method) string {
		return m.Name()
	})
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

func (d *Definition) GetTags() []*Tag {
	return sortedAsc(lo.Values(d.tags), func(t *Tag) TagID {
		return t.ID()
	})
}

func (d *Definition) SetTags(tags ...*Tag) *Definition {
	d.tags = lo.SliceToMap(tags, func(t *Tag) (TagID, *Tag) {
		return t.ID(), t
	})
	return d
}

func (d *Definition) AddTags(tags ...*Tag) *Definition {
	for _, tag := range tags {
		d.tags[tag.ID()] = tag
	}
	return d
}

func (d *Definition) RemoveTags(ids ...TagID) *Definition {
	for _, id := range ids {
		delete(d.tags, id)
	}
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

func (d *Definition) IsShared() bool {
	return d.shared
}

func (d *Definition) SetShared(shared bool) *Definition {
	d.shared = shared
	return d
}

func (d *Definition) IsAutowired() bool {
	return d.autowired
}

func (d *Definition) SetAutowired(autowired bool) *Definition {
	d.autowired = autowired
	return d
}

func (d *Definition) String() string {
	return string(d.ID())
}
