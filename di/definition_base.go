package di

import (
	"strings"

	"github.com/samber/lo"
)

// definition is what a service definition and a function definition are the
// same: an identity, labels, where it lives, where it was registered, and the
// two properties that decide when it runs.
//
// self is the concrete definition embedding this one, so that a setter returns
// the type the caller started from and the fluent chains keep working. Nothing
// else needs it.
type definition[T any] struct {
	self   T
	id     ID
	labels []Label
	source source

	scope      *Scope
	childScope *Scope

	// Properties
	lazy      bool
	autowired bool
}

// init fills in what every definition starts with. The source is captured by
// the constructor rather than here, so that the stack it records is the
// caller's own and not one frame of godi's.
func (d *definition[T]) init(self T, src source) {
	d.self = self
	d.id = NewID()
	d.source = src
	d.lazy = defaultLazy
	d.autowired = defaultAutowired
}

func (d *definition[T]) ID() ID {
	return d.id
}

func (d *definition[T]) Scope() *Scope {
	return d.scope
}

func (d *definition[T]) SetScope(scope *Scope) T {
	d.scope = scope
	return d.self
}

func (d *definition[T]) ChildScope() *Scope {
	return d.childScope
}

func (d *definition[T]) SetChildScope(scope *Scope) T {
	d.childScope = scope
	return d.self
}

// NewChildScope creates a scope of this definition's own inside parent: named
// after the definition, and back-linked to it.
//
// Both halves matter, and doing them by hand means knowing a convention nobody
// wrote down. A scope made with Scope.NewChild belongs to no definition, and
// everything that reports on a container can only describe it as such.
func (d *definition[T]) NewChildScope(parent *Scope) *Scope {
	d.childScope = parent.NewChild(d.id.String())
	return d.childScope
}

// EffectiveScope returns the scope in which all the dependencies should be resolved.
// For most definitions this is the scope where it is defined.
// But if it has a child-scope, then the dependencies should be resolved with that scope included.
func (d *definition[T]) EffectiveScope() *Scope {
	if d.childScope != nil {
		return d.childScope
	}
	return d.scope
}

func (d *definition[T]) Labels() []Label {
	return d.labels
}

func (d *definition[T]) SetLabels(labels ...Label) T {
	d.labels = labels
	return d.self
}

func (d *definition[T]) AddLabels(labels ...Label) T {
	d.labels = append(d.labels, labels...)
	return d.self
}

func (d *definition[T]) RemoveLabels(labels ...Label) T {
	d.labels = lo.Without(d.labels, labels...)
	return d.self
}

func (d *definition[T]) IsLazy() bool {
	return d.lazy
}

func (d *definition[T]) SetLazy(lazy bool) T {
	d.lazy = lazy
	return d.self
}

func (d *definition[T]) IsAutowired() bool {
	return d.autowired
}

func (d *definition[T]) SetAutowired(autowired bool) T {
	d.autowired = autowired
	return d.self
}

// RegisteredAt is where this definition was declared: the first frame outside
// godi at the moment it was created.
func (d *definition[T]) RegisteredAt() Location {
	return d.source.Location()
}

// labelSuffix is how a definition names its labels when it describes itself.
func (d *definition[T]) labelSuffix() string {
	if len(d.labels) == 0 {
		return ""
	}

	var bld strings.Builder
	bld.WriteString(" (")
	for i, label := range d.labels {
		if i > 0 {
			bld.WriteString(", ")
		}
		bld.WriteString(label.String())
	}
	bld.WriteString(")")
	return bld.String()
}
