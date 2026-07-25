package di

import (
	"io"
	"iter"
	"reflect"

	"github.com/elliotchance/orderedmap/v2"

	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/internal/iterx"
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

// Deprecated: use the graph package with the text encoder:
//
//	g, err := graph.Extract(c)
//	err = g.Encode(w, text.New())
func (c *Container) Print(w io.Writer) {
	Print(c.root, w)
}

// Graph returns the dependency graph of the container.
//
// Prefer graph.Extract, which takes the options rather than a built Config.
func (c *Container) Graph(cfg graph.Config) *graph.Graph {
	return newExtractor(c, cfg, nil).extract()
}

// scopesSeq yields every scope in the container, in the order they were created.
func (c *Container) scopesSeq() iter.Seq[*Scope] {
	return iterx.Values(c.scopes.Iterator())
}

// serviceDefsSeq yields every service definition in the container, with the
// scope it is registered in.
func (c *Container) serviceDefsSeq() iter.Seq2[*Scope, *ServiceDefinition] {
	return func(yield func(*Scope, *ServiceDefinition) bool) {
		for scope := range c.scopesSeq() {
			for def := range scope.ServiceDefinitionsSeq() {
				if !yield(scope, def) {
					return
				}
			}
		}
	}
}

// functionDefsSeq yields every function definition in the container, with the
// scope it is registered in.
func (c *Container) functionDefsSeq() iter.Seq2[*Scope, *FunctionDefinition] {
	return func(yield func(*Scope, *FunctionDefinition) bool) {
		for scope := range c.scopesSeq() {
			for def := range scope.FunctionDefinitionsSeq() {
				if !yield(scope, def) {
					return
				}
			}
		}
	}
}

// slotsSeq yields every argument slot in the container: factory arguments,
// method call arguments and function arguments, across all scopes.
func (c *Container) slotsSeq() iter.Seq[*Slot] {
	return func(yield func(*Slot) bool) {
		for _, def := range c.serviceDefsSeq() {
			for _, slot := range def.Factory().Args().Slots() {
				if !yield(slot) {
					return
				}
			}
			for _, method := range def.MethodCalls() {
				for _, slot := range method.Args().Slots() {
					if !yield(slot) {
						return
					}
				}
			}
		}
		for _, def := range c.functionDefsSeq() {
			for _, slot := range def.Func().Args().Slots() {
				if !yield(slot) {
					return
				}
			}
		}
	}
}

// bindingsSeq yields every interface binding in every scope.
func (c *Container) bindingsSeq() iter.Seq[*InterfaceBinding] {
	return func(yield func(*InterfaceBinding) bool) {
		for scope := range c.scopesSeq() {
			for binding := range scope.BindingsSeq() {
				if !yield(binding) {
					return
				}
			}
		}
	}
}
