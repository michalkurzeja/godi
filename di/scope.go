package di

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"sync"

	"github.com/elliotchance/orderedmap/v2"
	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/iterx"
)

func NewScope(name string, container *Container, parent *Scope) *Scope {
	s := &Scope{
		name:      container.unusedScopeName(name),
		container: container,
		parent:    parent,
		svcs:      NewDefinitionRegistry[*ServiceDefinition](),
		funs:      NewDefinitionRegistry[*FunctionDefinition](),
		bindings:  orderedmap.NewOrderedMap[reflect.Type, *InterfaceBinding](),
		instances: make(map[ID]any),
	}
	container.scopes.Set(s.name, s)
	return s
}

type Scope struct {
	name string

	container *Container
	parent    *Scope

	svcs     *DefinitionRegistry[*ServiceDefinition]
	funs     *DefinitionRegistry[*FunctionDefinition]
	bindings *orderedmap.OrderedMap[reflect.Type, *InterfaceBinding]

	// instances are the shared services this scope has built. mu guards the map
	// and nothing else: a factory or a method call must never run under it.
	mu        sync.Mutex
	instances map[ID]any
}

func (s *Scope) String() string {
	return s.Name()
}

func (s *Scope) Name() string {
	return s.name
}

func (s *Scope) NewChild(name string) *Scope {
	return NewScope(name, s.container, s)
}

func (s *Scope) Parent() *Scope {
	return s.parent
}

// Chain returns a sequence of scopes, starting from this scope and climbing up the parent chain.
func (s *Scope) Chain() iter.Seq[*Scope] {
	return func(yield func(*Scope) bool) {
		current := s
		for current != nil {
			if !yield(current) {
				return
			}
			current = current.parent
		}
	}
}

func (s *Scope) Services() *DefinitionRegistry[*ServiceDefinition] {
	return s.svcs
}

func (s *Scope) Functions() *DefinitionRegistry[*FunctionDefinition] {
	return s.funs
}

func (s *Scope) Bindings() *orderedmap.OrderedMap[reflect.Type, *InterfaceBinding] {
	return s.bindings
}

func (s *Scope) HasService(id ID) bool {
	return s.svcs.Contains(id)
}

// bindingInChain finds the binding covering typ, and the scope that declared it.
// GetBoundArgInChain alone would not say which scope it came from.
func (s *Scope) bindingInChain(typ reflect.Type) (*Scope, *InterfaceBinding, bool) {
	for scope := range s.Chain() {
		if binding, ok := scope.GetBinding(typ); ok {
			return scope, binding, true
		}
	}
	return nil, nil, false
}

// Instantiated reports whether this scope has already built the service. For a
// shared service, it means the container is holding the instance.
func (s *Scope) Instantiated(id ID) bool {
	_, ok := s.instance(id)
	return ok
}

func (s *Scope) HasServiceInChain(id ID) bool {
	for scope := range s.Chain() {
		if scope.HasService(id) {
			return true
		}
	}
	return false
}

func (s *Scope) GetService(id ID) (any, error) {
	return withInstantiationContext(func(ic *instantiationContext) (any, error) {
		return s.getService(ic, id)
	})
}

func (s *Scope) getService(ic *instantiationContext, id ID) (any, error) {
	def, ok := s.svcs.Get(id)
	if !ok {
		return nil, nil
	}
	return s.getServiceInstance(ic, def)
}

func (s *Scope) GetServiceInChain(id ID) (any, error) {
	return withInstantiationContext(func(ic *instantiationContext) (any, error) {
		return s.getServiceInChain(ic, id)
	})
}

func (s *Scope) getServiceInChain(ic *instantiationContext, id ID) (any, error) {
	for scope := range s.Chain() {
		svc, err := scope.getService(ic, id)
		if svc != nil || err != nil {
			return svc, err
		}
	}
	return nil, nil
}

func (s *Scope) GetServices(ids ...ID) ([]any, error) {
	return withInstantiationContext(func(ic *instantiationContext) ([]any, error) {
		return s.getServices(ic, ids...)
	})
}

func (s *Scope) getServices(ic *instantiationContext, ids ...ID) ([]any, error) {
	return s.getServicesInstances(ic, s.svcs.GetByIDs(ids))
}

func (s *Scope) GetServicesInChain(ids ...ID) ([]any, error) {
	return withInstantiationContext(func(ic *instantiationContext) ([]any, error) {
		return s.getServicesInChain(ic, ids...)
	})
}

func (s *Scope) getServicesInChain(ic *instantiationContext, ids ...ID) ([]any, error) {
	var defs []*ServiceDefinition
	for scope := range s.Chain() {
		defs = append(defs, scope.svcs.GetByIDs(ids)...)
	}
	return s.getServicesInstances(ic, defs)
}

func (s *Scope) GetServicesIDsByType(typ reflect.Type) []ID {
	return s.svcs.GetIDsByType(typ)
}

// ServicesIDsByTypeInChainSeq yields the services of the type visible from this
// scope, nearest scope first. Use it when the first match is all you need.
func (s *Scope) ServicesIDsByTypeInChainSeq(typ reflect.Type) iter.Seq[ID] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[ID] {
		return slices.Values(scope.GetServicesIDsByType(typ))
	})
}

func (s *Scope) GetServicesIDsByTypeInChain(typ reflect.Type) []ID {
	return slices.Collect(s.ServicesIDsByTypeInChainSeq(typ))
}

func (s *Scope) GetServicesByType(typ reflect.Type) ([]any, error) {
	return s.GetServices(s.GetServicesIDsByType(typ)...)
}

func (s *Scope) GetServicesByTypeInChain(typ reflect.Type) ([]any, error) {
	return s.GetServicesInChain(s.GetServicesIDsByTypeInChain(typ)...)
}

func (s *Scope) getServicesByTypeInChain(ic *instantiationContext, typ reflect.Type) ([]any, error) {
	return s.getServicesInChain(ic, s.GetServicesIDsByTypeInChain(typ)...)
}

func (s *Scope) GetServicesIDsByLabel(label Label) []ID {
	return s.svcs.GetIDsByLabel(label)
}

// ServicesIDsByLabelInChainSeq yields the services carrying the label that are
// visible from this scope, nearest scope first.
func (s *Scope) ServicesIDsByLabelInChainSeq(label Label) iter.Seq[ID] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[ID] {
		return slices.Values(scope.GetServicesIDsByLabel(label))
	})
}

func (s *Scope) GetServicesIDsByLabelInChain(label Label) []ID {
	return slices.Collect(s.ServicesIDsByLabelInChainSeq(label))
}

func (s *Scope) GetServicesByLabel(label Label) ([]any, error) {
	return s.GetServices(s.GetServicesIDsByLabel(label)...)
}

func (s *Scope) GetServicesByLabelInChain(label Label) ([]any, error) {
	return s.GetServicesInChain(s.GetServicesIDsByLabelInChain(label)...)
}

func (s *Scope) getServicesByLabelInChain(ic *instantiationContext, label Label) ([]any, error) {
	return s.getServicesInChain(ic, s.GetServicesIDsByLabelInChain(label)...)
}

func (s *Scope) HasFunction(id ID) bool {
	return s.funs.Contains(id)
}

func (s *Scope) HasFunctionInChain(id ID) bool {
	for scope := range s.Chain() {
		if scope.HasFunction(id) {
			return true
		}
	}
	return false
}

// ExecuteFunction calls the function, building whatever it needs. The services
// it is handed are fully configured by the time it runs, as against a factory's
// dependencies, whose method calls have not run yet.
func (s *Scope) ExecuteFunction(id ID) ([]any, error) {
	def, ok := s.funs.Get(id)
	if !ok {
		return nil, fmt.Errorf("function %s not found", id)
	}
	return withInstantiationContext(func(ic *instantiationContext) ([]any, error) {
		return s.executeFunction(ic, def)
	})
}

// ExecuteFunctionInChain calls the function from the nearest scope that has it.
// See ExecuteFunction.
func (s *Scope) ExecuteFunctionInChain(id ID) ([]any, error) {
	for scope := range s.Chain() {
		def, ok := s.funs.Get(id)
		if ok {
			return withInstantiationContext(func(ic *instantiationContext) ([]any, error) {
				return scope.executeFunction(ic, def)
			})
		}
	}
	return nil, fmt.Errorf("function %s not found", id)
}

// ExecuteFunctions calls each of the functions, in the order the IDs are given.
// One failing does not stop the rest; their errors are joined. See
// ExecuteFunction.
func (s *Scope) ExecuteFunctions(ids ...ID) (results [][]any, joinedErrs error) {
	defs := s.funs.GetByIDs(ids)
	if len(defs) == 0 {
		return nil, errors.New("found no functions for given IDs")
	}
	return withInstantiationContext(func(ic *instantiationContext) ([][]any, error) {
		return s.executeFunctions(ic, defs)
	})
}

// ExecuteFunctionsInChain calls every function found under the IDs, nearest
// scope first. See ExecuteFunctions.
func (s *Scope) ExecuteFunctionsInChain(ids ...ID) (results [][]any, joinedErrs error) {
	var defs []*FunctionDefinition
	for scope := range s.Chain() {
		defs = append(defs, scope.funs.GetByIDs(ids)...)
	}
	if len(defs) == 0 {
		return nil, errors.New("found no functions for given IDs")
	}
	return withInstantiationContext(func(ic *instantiationContext) ([][]any, error) {
		return s.executeFunctions(ic, defs)
	})
}

func (s *Scope) GetFunctionsIDsByType(typ reflect.Type) []ID {
	return s.funs.GetIDsByType(typ)
}

// FunctionsIDsByTypeInChainSeq yields the functions of the type visible from
// this scope, nearest scope first.
func (s *Scope) FunctionsIDsByTypeInChainSeq(typ reflect.Type) iter.Seq[ID] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[ID] {
		return slices.Values(scope.GetFunctionsIDsByType(typ))
	})
}

func (s *Scope) GetFunctionsIDsByTypeInChain(typ reflect.Type) []ID {
	return slices.Collect(s.FunctionsIDsByTypeInChainSeq(typ))
}

func (s *Scope) ExecuteFunctionsByType(typ reflect.Type) ([][]any, error) {
	return s.ExecuteFunctions(s.GetFunctionsIDsByType(typ)...)
}

func (s *Scope) ExecuteFunctionsByTypeInChain(typ reflect.Type) ([][]any, error) {
	return s.ExecuteFunctionsInChain(s.GetFunctionsIDsByTypeInChain(typ)...)
}

func (s *Scope) GetFunctionsIDsByLabel(label Label) []ID {
	return s.funs.GetIDsByLabel(label)
}

// FunctionsIDsByLabelInChainSeq yields the functions carrying the label that
// are visible from this scope, nearest scope first.
func (s *Scope) FunctionsIDsByLabelInChainSeq(label Label) iter.Seq[ID] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[ID] {
		return slices.Values(scope.GetFunctionsIDsByLabel(label))
	})
}

func (s *Scope) GetFunctionsIDsByLabelInChain(label Label) []ID {
	return slices.Collect(s.FunctionsIDsByLabelInChainSeq(label))
}

func (s *Scope) ExecuteFunctionsByLabel(label Label) ([][]any, error) {
	return s.ExecuteFunctions(s.GetFunctionsIDsByLabel(label)...)
}

func (s *Scope) ExecuteFunctionsByLabelInChain(label Label) ([][]any, error) {
	return s.ExecuteFunctionsInChain(s.GetFunctionsIDsByLabelInChain(label)...)
}

func (s *Scope) GetBoundArg(typ reflect.Type) (Arg, bool) {
	binding, ok := s.bindings.Get(typ)
	if !ok {
		return nil, false
	}
	return binding.boundTo, true
}

// GetBoundArgInChain is the argument bound to the type in the nearest scope that
// binds it. That is the binding that takes effect.
func (s *Scope) GetBoundArgInChain(typ reflect.Type) (Arg, bool) {
	return iterx.First(iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[Arg] {
		return func(yield func(Arg) bool) {
			if boundTo, ok := scope.GetBoundArg(typ); ok {
				yield(boundTo)
			}
		}
	}))
}

func (s *Scope) instance(id ID) (any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	svc, ok := s.instances[id]
	return svc, ok
}

func (s *Scope) setInstance(id ID, svc any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.instances[id] = svc
}

func (s *Scope) getServiceInstance(ic *instantiationContext, def *ServiceDefinition) (any, error) {
	svc, ok := s.instance(def.ID())
	if ok {
		return svc, nil
	}

	svc, err := s.instantiate(ic, def)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to instantiate service %s", def)
	}

	return svc, nil
}

func (s *Scope) getServicesInstances(ic *instantiationContext, defs []*ServiceDefinition) (svcs []any, joinedErrs error) {
	svcs = make([]any, len(defs))
	for i, def := range defs {
		svc, err := s.getServiceInstance(ic, def)
		svcs[i] = svc
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return svcs, joinedErrs
}

// instantiate runs the factory and hands the method calls to the context. They
// run once the whole factory chain is done, so that nothing configures a
// service another factory is still building.
func (s *Scope) instantiate(ic *instantiationContext, def *ServiceDefinition) (any, error) {
	err := ic.pushDefinition(def)
	if err != nil {
		return nil, err
	}
	defer ic.popDefinition()

	svc, err := def.factory.execute(ic, def.EffectiveScope())
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute factory for service %s", def)
	}

	if def.shared {
		s.setInstance(def.id, svc)
	}
	ic.enqueueMethodCalls(def, svc, def.EffectiveScope())

	return svc, nil
}

func (s *Scope) executeFunction(ic *instantiationContext, def *FunctionDefinition) ([]any, error) {
	fn := def.function

	args, err := fn.resolveArgs(ic, def.EffectiveScope(), nil)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute function %s", def)
	}
	if err := ic.executeAllMethodCalls(); err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute function %s", def)
	}

	return lo.Map(fn.call(args), func(v reflect.Value, _ int) any { return v.Interface() }), nil
}

func (s *Scope) executeFunctions(ic *instantiationContext, defs []*FunctionDefinition) (results [][]any, joinedErrs error) {
	results = make([][]any, len(defs))
	for i, def := range defs {
		res, err := s.executeFunction(ic, def)
		results[i] = res
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return results, joinedErrs
}

func (s *Scope) ServiceDefinitionsSeq() iter.Seq[*ServiceDefinition] {
	return s.svcs.Seq()
}

func (s *Scope) ServiceDefinitionsInChainSeq() iter.Seq[*ServiceDefinition] {
	return iterx.Flatten(s.Chain(), (*Scope).ServiceDefinitionsSeq)
}

func (s *Scope) GetServiceDefinitions() []*ServiceDefinition {
	return slices.Collect(s.ServiceDefinitionsSeq())
}

func (s *Scope) GetServiceDefinitionsInChain() []*ServiceDefinition {
	return slices.Collect(s.ServiceDefinitionsInChainSeq())
}

func (s *Scope) GetServiceDefinitionsByType(typ reflect.Type) []*ServiceDefinition {
	return s.svcs.GetByType(typ)
}

// ServiceDefinitionsByTypeInChainSeq yields the definitions of the type visible
// from this scope, nearest scope first.
func (s *Scope) ServiceDefinitionsByTypeInChainSeq(typ reflect.Type) iter.Seq[*ServiceDefinition] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[*ServiceDefinition] {
		return slices.Values(scope.GetServiceDefinitionsByType(typ))
	})
}

func (s *Scope) GetServiceDefinitionsByTypeInChain(typ reflect.Type) []*ServiceDefinition {
	return slices.Collect(s.ServiceDefinitionsByTypeInChainSeq(typ))
}

func (s *Scope) GetServiceDefinitionsByLabel(label Label) []*ServiceDefinition {
	return s.svcs.GetByLabel(label)
}

// ServiceDefinitionsByLabelInChainSeq yields the definitions carrying the label
// that are visible from this scope, nearest scope first.
func (s *Scope) ServiceDefinitionsByLabelInChainSeq(label Label) iter.Seq[*ServiceDefinition] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[*ServiceDefinition] {
		return slices.Values(scope.GetServiceDefinitionsByLabel(label))
	})
}

func (s *Scope) GetServiceDefinitionsByLabelInChain(label Label) []*ServiceDefinition {
	return slices.Collect(s.ServiceDefinitionsByLabelInChainSeq(label))
}

func (s *Scope) GetServiceDefinition(id ID) (*ServiceDefinition, bool) {
	return s.svcs.Get(id)
}

func (s *Scope) GetServiceDefinitionInChain(id ID) (*ServiceDefinition, bool) {
	for scope := range s.Chain() {
		if def, ok := scope.GetServiceDefinition(id); ok {
			return def, true
		}
	}
	return nil, false
}

func (s *Scope) AddServiceDefinitions(definitions ...*ServiceDefinition) *Scope {
	s.svcs.Add(definitions...)
	return s
}

func (s *Scope) RemoveServiceDefinitions(ids ...ID) *Scope {
	s.svcs.Remove(ids...)
	return s
}

func (s *Scope) ClearServiceDefinitions() *Scope {
	s.svcs.Clear()
	return s
}

func (s *Scope) FunctionDefinitionsSeq() iter.Seq[*FunctionDefinition] {
	return s.funs.Seq()
}

func (s *Scope) FunctionDefinitionsInChainSeq() iter.Seq[*FunctionDefinition] {
	return iterx.Flatten(s.Chain(), (*Scope).FunctionDefinitionsSeq)
}

func (s *Scope) GetFunctionDefinitions() []*FunctionDefinition {
	return slices.Collect(s.FunctionDefinitionsSeq())
}

func (s *Scope) GetFunctionDefinitionsInChain() []*FunctionDefinition {
	return slices.Collect(s.FunctionDefinitionsInChainSeq())
}

func (s *Scope) GetFunctionDefinitionsByType(typ reflect.Type) []*FunctionDefinition {
	return s.funs.GetByType(typ)
}

// FunctionDefinitionsByTypeInChainSeq yields the definitions of the type
// visible from this scope, nearest scope first.
func (s *Scope) FunctionDefinitionsByTypeInChainSeq(typ reflect.Type) iter.Seq[*FunctionDefinition] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[*FunctionDefinition] {
		return slices.Values(scope.GetFunctionDefinitionsByType(typ))
	})
}

func (s *Scope) GetFunctionDefinitionsByTypeInChain(typ reflect.Type) []*FunctionDefinition {
	return slices.Collect(s.FunctionDefinitionsByTypeInChainSeq(typ))
}

func (s *Scope) GetFunctionDefinitionsByLabel(label Label) []*FunctionDefinition {
	return s.funs.GetByLabel(label)
}

// FunctionDefinitionsByLabelInChainSeq yields the definitions carrying the
// label that are visible from this scope, nearest scope first.
func (s *Scope) FunctionDefinitionsByLabelInChainSeq(label Label) iter.Seq[*FunctionDefinition] {
	return iterx.Flatten(s.Chain(), func(scope *Scope) iter.Seq[*FunctionDefinition] {
		return slices.Values(scope.GetFunctionDefinitionsByLabel(label))
	})
}

func (s *Scope) GetFunctionDefinitionsByLabelInChain(label Label) []*FunctionDefinition {
	return slices.Collect(s.FunctionDefinitionsByLabelInChainSeq(label))
}

func (s *Scope) GetFunctionDefinition(id ID) (*FunctionDefinition, bool) {
	return s.funs.Get(id)
}

func (s *Scope) GetFunctionDefinitionInChain(id ID) (*FunctionDefinition, bool) {
	for scope := range s.Chain() {
		if def, ok := scope.GetFunctionDefinition(id); ok {
			return def, true
		}
	}
	return nil, false
}

func (s *Scope) AddFunctionDefinitions(functions ...*FunctionDefinition) *Scope {
	s.funs.Add(functions...)
	return s
}

func (s *Scope) RemoveFunctionDefinitions(ids ...ID) *Scope {
	s.funs.Remove(ids...)
	return s
}

func (s *Scope) BindingsSeq() iter.Seq[*InterfaceBinding] {
	return iterx.Values(s.bindings.Iterator())
}

// BindingsInChainSeq yields every binding visible from this scope, nearest scope
// first. That is the order they take effect in.
func (s *Scope) BindingsInChainSeq() iter.Seq[*InterfaceBinding] {
	return iterx.Flatten(s.Chain(), (*Scope).BindingsSeq)
}

func (s *Scope) GetBindings() []*InterfaceBinding {
	return slices.Collect(s.BindingsSeq())
}

func (s *Scope) GetBinding(typ reflect.Type) (*InterfaceBinding, bool) {
	binding, ok := s.bindings.Get(typ)
	return binding, ok
}

func (s *Scope) SetBindings(bindings ...*InterfaceBinding) *Scope {
	s.bindings = orderedmap.NewOrderedMap[reflect.Type, *InterfaceBinding]()
	return s.AddBindings(bindings...)
}

func (s *Scope) AddBindings(bindings ...*InterfaceBinding) *Scope {
	for _, binding := range bindings {
		s.bindings.Set(binding.ifaceTyp, binding)
	}
	return s
}

func (s *Scope) RemoveBindings(types ...reflect.Type) *Scope {
	for _, typ := range types {
		s.bindings.Delete(typ)
	}
	return s
}

type Definition interface {
	ID() ID
	Type() reflect.Type
	Labels() []Label
}

type DefinitionRegistry[Def Definition] struct {
	byID    *orderedmap.OrderedMap[ID, Def]
	byType  *orderedmap.OrderedMap[reflect.Type, []Def]
	byLabel *orderedmap.OrderedMap[Label, []Def]
}

func NewDefinitionRegistry[Def Definition]() *DefinitionRegistry[Def] {
	r := &DefinitionRegistry[Def]{}
	r.Clear()
	return r
}

func (r *DefinitionRegistry[Def]) Add(defs ...Def) {
	for _, d := range defs {
		r.byID.Set(d.ID(), d)
		byType := r.byType.GetOrDefault(d.Type(), nil)
		r.byType.Set(d.Type(), append(byType, d))
		for _, label := range d.Labels() {
			byLabel := r.byLabel.GetOrDefault(label, nil)
			r.byLabel.Set(label, append(byLabel, d))
		}
	}
}

func (r *DefinitionRegistry[Def]) Remove(ids ...ID) {
	for _, id := range ids {
		def, ok := r.byID.Get(id)
		if !ok {
			continue
		}

		defEq := func(d Def) bool { return d.ID() == def.ID() }

		r.byID.Delete(id)
		byType := r.byType.GetOrDefault(def.Type(), nil)
		r.byType.Set(def.Type(), slices.DeleteFunc(byType, defEq))
		for _, label := range def.Labels() {
			byLabel := r.byLabel.GetOrDefault(label, nil)
			r.byLabel.Set(label, slices.DeleteFunc(byLabel, defEq))
		}
	}
}

func (r *DefinitionRegistry[Def]) Clear() {
	r.byID = orderedmap.NewOrderedMap[ID, Def]()
	r.byType = orderedmap.NewOrderedMap[reflect.Type, []Def]()
	r.byLabel = orderedmap.NewOrderedMap[Label, []Def]()
}

func (r *DefinitionRegistry[Def]) Contains(id ID) bool {
	_, ok := r.byID.Get(id)
	return ok
}

func (r *DefinitionRegistry[Def]) Get(id ID) (Def, bool) {
	def, ok := r.byID.Get(id)
	return def, ok
}

func (r *DefinitionRegistry[Def]) GetByIDs(ids []ID) []Def {
	defs := make([]Def, 0, len(ids))
	for _, id := range ids {
		def, ok := r.Get(id)
		if ok {
			defs = append(defs, def)
		}
	}
	return defs
}

func (r *DefinitionRegistry[Def]) GetIDsByType(typ reflect.Type) []ID {
	return r.getIDs(r.byType.GetOrDefault(typ, nil))
}

func (r *DefinitionRegistry[Def]) GetByType(typ reflect.Type) []Def {
	return r.byType.GetOrDefault(typ, nil)
}

func (r *DefinitionRegistry[Def]) GetIDsByLabel(label Label) []ID {
	return r.getIDs(r.byLabel.GetOrDefault(label, nil))
}

func (r *DefinitionRegistry[Def]) GetByLabel(label Label) []Def {
	return r.byLabel.GetOrDefault(label, nil)
}

func (r *DefinitionRegistry[Def]) Seq() iter.Seq[Def] {
	return iterx.Values(r.byID.Iterator())
}

func (r *DefinitionRegistry[Def]) GetAll() []Def {
	return slices.Collect(r.Seq())
}

func (r *DefinitionRegistry[Def]) Len() int {
	return r.byID.Len()
}

func (r *DefinitionRegistry[Def]) getIDs(defs []Def) []ID {
	return lo.Map(defs, func(d Def, _ int) ID { return d.ID() })
}
