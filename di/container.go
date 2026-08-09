package di

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"slices"
	"sync"

	"github.com/elliotchance/orderedmap/v2"

	"github.com/michalkurzeja/godi/v2/internal/iterx"
)

const RootScope = "root"

type Container struct {
	root   *Scope
	scopes *orderedmap.OrderedMap[string, *Scope]

	defaults Defaults

	// diagnostics is what the compiler passes had to say about this container.
	// They live here rather than on the builder so that they outlast a build,
	// whether it succeeded or not.
	diagnostics []Diagnostic

	// buildMu serialises construction: one call into the container builds at a
	// time and the rest wait. A factory runs while it is held, which is why a
	// factory must not resolve from the container. See insideUserCode.
	buildMu sync.Mutex

	// mu guards the instances map of every scope. Nothing holds it across user
	// code, so reading a graph out of a live container does not wait for a
	// factory.
	//
	// Take buildMu before mu, never the other way round.
	mu sync.RWMutex
}

func NewContainer(defaults Defaults) *Container {
	c := &Container{
		scopes:   orderedmap.NewOrderedMap[string, *Scope](),
		defaults: defaults,
	}
	c.root = NewScope(RootScope, c, nil)
	return c
}

func (c *Container) Defaults() Defaults {
	return c.defaults
}

// Diagnostics is what the compiler passes had to say about this container, in
// the order they said it. It is a copy: a pass may still be reporting.
func (c *Container) Diagnostics() []Diagnostic {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.diagnostics)
}

// report records what a pass has to say. Nothing may hold mu while calling it.
func (c *Container) report(d Diagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagnostics = append(c.diagnostics, d)
}

// creditDiagnosticsTo names the pass behind everything reported since from. The
// Compiler calls it after each pass, the same way it credits wiring: a pass
// reports what it found, and naming itself is not its job.
func (c *Container) creditDiagnosticsTo(from int, pass string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.diagnostics[from:] {
		c.diagnostics[from+i].Pass = pass
	}
}

// diagnosticErrors is what compilation should fail with, out of everything
// reported since the given point.
func (c *Container) diagnosticErrors(from int) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var joined error
	for _, d := range c.diagnostics[from:] {
		if d.Severity == SeverityError {
			joined = errors.Join(joined, d.err())
		}
	}
	return joined
}

// diagnosticCount is how many diagnostics have been reported so far.
func (c *Container) diagnosticCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.diagnostics)
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
//	g, err := extract.From(c)
//	err = g.Encode(w, text.New())
func (c *Container) Print(w io.Writer) {
	Print(c.root, w)
}

// Root is the scope everything else hangs off.
func (c *Container) Root() *Scope {
	return c.root
}

// Scope finds a scope by name.
func (c *Container) Scope(name string) (*Scope, bool) {
	return c.scopes.Get(name)
}

// Scopes yields every scope in the container, in the order they were created.
func (c *Container) Scopes() iter.Seq[*Scope] {
	return iterx.Values(c.scopes.Iterator())
}

// ServiceDefinitionsSeq yields every service definition in the container, with
// the scope it is registered in.
func (c *Container) ServiceDefinitionsSeq() iter.Seq2[*Scope, *ServiceDefinition] {
	return func(yield func(*Scope, *ServiceDefinition) bool) {
		for scope := range c.Scopes() {
			for def := range scope.ServiceDefinitionsSeq() {
				if !yield(scope, def) {
					return
				}
			}
		}
	}
}

// FunctionDefinitionsSeq yields every function definition in the container,
// with the scope it is registered in.
func (c *Container) FunctionDefinitionsSeq() iter.Seq2[*Scope, *FunctionDefinition] {
	return func(yield func(*Scope, *FunctionDefinition) bool) {
		for scope := range c.Scopes() {
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
		for _, def := range c.ServiceDefinitionsSeq() {
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
		for _, def := range c.FunctionDefinitionsSeq() {
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
		for scope := range c.Scopes() {
			for binding := range scope.BindingsSeq() {
				if !yield(binding) {
					return
				}
			}
		}
	}
}

// unusedScopeName stops a second scope from taking a name that is already in use.
//
// The container holds its scopes in a map keyed by name. A scope that lost its
// place there would still exist in the parent tree, but nothing walking the
// container would find it again, and its definitions would never be built.
//
// godi names its own child scopes after the definition that declared them, so
// those never collide. Two calls to NewChild("plugins") do.
func (c *Container) unusedScopeName(name string) string {
	if _, taken := c.scopes.Get(name); !taken {
		return name
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s#%d", name, n)
		if _, taken := c.scopes.Get(candidate); !taken {
			return candidate
		}
	}
}
