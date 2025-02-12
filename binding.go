package di

import (
	"reflect"

	"github.com/michalkurzeja/godi/v2/di"
)

type InterfaceBindingBuilder struct {
	typ    reflect.Type
	bindTo *ArgBuilder
}

func (b *InterfaceBindingBuilder) Build(scope *di.Scope) error {
	arg, err := b.bindTo.Build()
	if err != nil {
		return err
	}
	binding, err := di.NewInterfaceBinding(b.typ, arg)
	if err != nil {
		return err
	}

	scope.AddBindings(binding)

	return nil
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
