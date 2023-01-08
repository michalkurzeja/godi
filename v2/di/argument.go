package di

import (
	"reflect"
)

type Argument interface {
	Type() reflect.Type
	resolve(c *container) (any, error)
	setIndex(i uint)
	index() idx
}

type idx struct {
	i    uint
	auto bool
}

func autoIdx() idx {
	return idx{auto: true}
}

type baseArg struct {
	typ reflect.Type
	idx idx
}

func newBaseArg(typ reflect.Type) baseArg {
	return baseArg{typ: typ, idx: autoIdx()}
}

func (b *baseArg) setIndex(i uint) {
	b.idx = idx{i: i}
}

func (b *baseArg) index() idx {
	return b.idx
}

type Value struct {
	baseArg
	v any
}

func NewValue(v any) *Value {
	return &Value{
		baseArg: newBaseArg(reflect.TypeOf(v)),
		v:       v,
	}
}

func (v Value) Type() reflect.Type {
	return reflect.TypeOf(v.v)
}

func (v Value) resolve(_ *container) (any, error) {
	return v.v, nil
}

type Reference struct {
	baseArg
	id ID
}

func NewReference(id ID, typ reflect.Type) *Reference {
	return &Reference{
		baseArg: newBaseArg(typ),
		id:      id,
	}
}

func (r Reference) ID() ID {
	return r.id
}

func (r Reference) Type() reflect.Type {
	return r.typ
}

func (r Reference) resolve(c *container) (any, error) {
	return c.get(r.id, false)
}

type TaggedCollection struct {
	baseArg
	tag Tag
}

func NewTaggedCollection(tag Tag, typ reflect.Type) *TaggedCollection {
	return &TaggedCollection{
		baseArg: newBaseArg(typ),
		tag:     tag,
	}
}

func (t TaggedCollection) Type() reflect.Type {
	return t.typ
}

func (t TaggedCollection) resolve(c *container) (any, error) {
	return c.getByTag(t.tag, false)
}
