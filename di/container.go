package di

import (
	"io"
	"reflect"

	"github.com/elliotchance/orderedmap/v2"
)

const RootScope = "root"

type Container struct {
	root   *Scope
	scopes *orderedmap.OrderedMap[string, *Scope]
}

func NewContainer() *Container {
	c := &Container{scopes: orderedmap.NewOrderedMap[string, *Scope]()}
	c.root = NewScope(RootScope, c, nil)
	return c
}

func (c *Container) HasService(id ID) bool {
	return c.root.HasService(id)
}

func (c *Container) GetService(id ID) (any, error) {
	return c.root.GetService(id)
}

func (c *Container) GetServices(ids ...ID) ([]any, error) {
	return c.root.GetServices(ids...)
}

func (c *Container) GetServicesIDsByType(typ reflect.Type) []ID {
	return c.root.GetServicesIDsByType(typ)
}

func (c *Container) GetServicesByType(typ reflect.Type) ([]any, error) {
	return c.root.GetServicesByType(typ)
}

func (c *Container) GetServicesIDsByLabel(label Label) []ID {
	return c.root.GetServicesIDsByLabel(label)
}

func (c *Container) GetServicesByLabel(label Label) ([]any, error) {
	return c.root.GetServicesByLabel(label)
}

func (c *Container) HasFunction(id ID) bool {
	return c.root.HasFunction(id)
}

func (c *Container) ExecuteFunction(id ID) ([]any, error) {
	return c.root.ExecuteFunction(id)
}

func (c *Container) ExecuteFunctions(ids ...ID) (results [][]any, joinedErrs error) {
	return c.root.ExecuteFunctions(ids...)
}

func (c *Container) GetFunctionsIDsByType(typ reflect.Type) []ID {
	return c.root.GetFunctionsIDsByType(typ)
}

func (c *Container) ExecuteFunctionsByType(typ reflect.Type) ([][]any, error) {
	return c.root.ExecuteFunctionsByType(typ)
}

func (c *Container) GetFunctionsIDsByLabel(label Label) []ID {
	return c.root.GetFunctionsIDsByLabel(label)
}

func (c *Container) ExecuteFunctionsByLabel(label Label) ([][]any, error) {
	return c.root.ExecuteFunctionsByLabel(label)
}

func (c *Container) GetBindingFor(typ reflect.Type) (Arg, bool) {
	return c.root.GetBoundArg(typ)
}

func (c *Container) Print(w io.Writer) {
	Print(c.root, w)
}
