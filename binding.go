package godi

import (
	"reflect"

	"github.com/michalkurzeja/godi/v2/di"
)

type InterfaceBindingBuilder struct {
	typ    reflect.Type
	bindTo *ArgBuilder
}

func (b *InterfaceBindingBuilder) Build() (*di.InterfaceBinding, error) {
	arg, err := b.bindTo.Build()
	if err != nil {
		return nil, err
	}
	return di.NewInterfaceBinding(b.typ, arg)
}

// BindArg binds the interface Iface to the given argument (bindTo).
func BindArg[Iface any](bindTo *ArgBuilder) *InterfaceBindingBuilder {
	return &InterfaceBindingBuilder{typ: reflect.TypeFor[Iface](), bindTo: bindTo}
}

// BindType binds the interface Iface to the type To.
func BindType[Iface, To any]() *InterfaceBindingBuilder {
	return BindArg[Iface](Type[To]())
}

// BindSlice binds the interface Iface, or []Iface, to a slice []To.
func BindSlice[Iface, To any]() *InterfaceBindingBuilder {
	return BindArg[Iface](Compound[Iface](Type[To]()))
}
