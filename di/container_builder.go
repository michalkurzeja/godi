package di

import (
	"cmp"
	"errors"
	"reflect"
	"slices"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// ContainerBuilder is a builder for Container. It provides a fluent interface to
// inspect and configure the container.
// Once Build() is called, this builder is locked and no longer usable. Subsequent calls
// to Build() will return an error and any other method may panic.
type ContainerBuilder struct {
	container *Container
	compiler  *Compiler

	built bool
}

func NewContainerBuilder() *ContainerBuilder {
	return &ContainerBuilder{
		container: NewContainer(),
		compiler:  NewCompiler(),
	}
}

func (b *ContainerBuilder) GetServiceDefinitions() []*ServiceDefinition {
	return b.container.services.GetAll()
}

func (b *ContainerBuilder) GetServiceDefinitionsByType(typ reflect.Type) []*ServiceDefinition {
	return b.container.services.GetByType(typ)
}

func (b *ContainerBuilder) GetServiceDefinitionsByLabel(label Label) []*ServiceDefinition {
	return b.container.services.GetByLabel(label)
}

func (b *ContainerBuilder) GetServiceDefinition(id ID) (*ServiceDefinition, bool) {
	return b.container.services.Get(id)
}

func (b *ContainerBuilder) AddServiceDefinitions(definitions ...*ServiceDefinition) *ContainerBuilder {
	b.container.services.Add(definitions...)
	return b
}

func (b *ContainerBuilder) RemoveServiceDefinitions(ids ...ID) *ContainerBuilder {
	b.container.services.Remove(ids...)
	return b
}

func (b *ContainerBuilder) ClearServiceDefinitions() *ContainerBuilder {
	b.container.services.Clear()
	return b
}

func (b *ContainerBuilder) GetFunctionDefinitions() []*FunctionDefinition {
	return b.container.functions.GetAll()
}

func (b *ContainerBuilder) GetFunctionDefinitionsByType(typ reflect.Type) []*FunctionDefinition {
	return b.container.functions.GetByType(typ)
}

func (b *ContainerBuilder) GetFunctionDefinitionsByLabel(label Label) []*FunctionDefinition {
	return b.container.functions.GetByLabel(label)
}

func (b *ContainerBuilder) GetFunctionDefinition(id ID) (*FunctionDefinition, bool) {
	return b.container.functions.Get(id)
}

func (b *ContainerBuilder) AddFunctionDefinitions(functions ...*FunctionDefinition) *ContainerBuilder {
	b.container.functions.Add(functions...)
	return b
}

func (b *ContainerBuilder) RemoveFunctionDefinitions(ids ...ID) *ContainerBuilder {
	b.container.functions.Remove(ids...)
	return b
}

func (b *ContainerBuilder) GetBindings() []*InterfaceBinding {
	bindings := lo.Values(b.container.bindings)
	slices.SortFunc(bindings, func(a, b *InterfaceBinding) int {
		return cmp.Compare(util.Signature(a.ifaceTyp), util.Signature(b.ifaceTyp))
	})
	return bindings
}

func (b *ContainerBuilder) GetBinding(typ reflect.Type) (*InterfaceBinding, bool) {
	binding, ok := b.container.bindings[typ]
	return binding, ok
}

func (b *ContainerBuilder) SetBindings(bindings ...*InterfaceBinding) *ContainerBuilder {
	b.container.bindings = make(map[reflect.Type]*InterfaceBinding, len(bindings))
	return b.AddBindings(bindings...)
}

func (b *ContainerBuilder) AddBindings(bindings ...*InterfaceBinding) *ContainerBuilder {
	for _, binding := range bindings {
		b.container.bindings[binding.ifaceTyp] = binding
	}
	return b
}

func (b *ContainerBuilder) RemoveBindings(types ...reflect.Type) *ContainerBuilder {
	b.container.bindings = lo.OmitByKeys(b.container.bindings, types)
	return b
}

func (b *ContainerBuilder) Compiler() *Compiler {
	return b.compiler
}

func (b *ContainerBuilder) Build() (*Container, error) {
	if b.built {
		return nil, errors.New("container already built")
	}
	b.built = true

	err := b.compiler.Run(b)
	if err != nil {
		return nil, errorsx.Wrap(err, "compilation failed")
	}

	container := b.container
	b.container = nil

	return container, nil
}
