package di

import (
	"fmt"
	"reflect"
)

// Argument represents a dependency that can be resolved by the container.
// Service factory functions and methods calls use Argument objects.
// Arguments can be either values, references or collections of tagged services.
type Argument interface {
	fmt.Stringer
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

// Value represents a literal dependency, known at compilation time and passed to the function as-is.
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

func (v Value) String() string {
	return fmt.Sprintf("%v", v.v)
}

func (v Value) Type() reflect.Type {
	return reflect.TypeOf(v.v)
}

func (v Value) resolve(_ *container) (any, error) {
	return v.v, nil
}

// Reference represents a dependency on another service. It is resolved by the container
// and must point at a valid service ID.
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

func (r Reference) String() string {
	return fmt.Sprintf("@%s", r.ID())
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

// TaggedCollection represents a dependency on a collection of services tagged with a specific tag.
// It is resolved by the container and may be empty.
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

func (t TaggedCollection) String() string {
	return fmt.Sprintf("#%s", t.tag)
}

func (t TaggedCollection) Type() reflect.Type {
	return t.typ
}

func (t TaggedCollection) resolve(c *container) (any, error) {
	return c.getByTag(t.tag, false)
}
