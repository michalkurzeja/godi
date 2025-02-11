package di

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/elliotchance/orderedmap/v2"
	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/iterx"
)

func NewScope(name string, container *Container, parent *Scope) *Scope {
	s := &Scope{
		name:      name,
		container: container,
		parent:    parent,
		svcs:      NewDefinitionRegistry[*ServiceDefinition](),
		funs:      NewDefinitionRegistry[*FunctionDefinition](),
		bindings:  orderedmap.NewOrderedMap[reflect.Type, *InterfaceBinding](),
		instances: make(map[ID]any),
	}
	container.scopes.Set(name, s)
	return s
}

type Scope struct {
	name string

	container *Container
	parent    *Scope

	svcs      *DefinitionRegistry[*ServiceDefinition]
	funs      *DefinitionRegistry[*FunctionDefinition]
	bindings  *orderedmap.OrderedMap[reflect.Type, *InterfaceBinding]
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

func (s *Scope) HasServiceInChain(id ID) bool {
	for scope := range s.Chain() {
		if scope.HasService(id) {
			return true
		}
	}
	return false
}

func (s *Scope) GetService(id ID) (any, error) {
	def, ok := s.svcs.Get(id)
	if !ok {
		return nil, nil
	}
	return s.getServiceInstance(def)
}

func (s *Scope) GetServiceInChain(id ID) (any, error) {
	for scope := range s.Chain() {
		svc, err := scope.GetService(id)
		if svc != nil || err != nil {
			return svc, err
		}
	}
	return nil, nil
}

func (s *Scope) GetServices(ids ...ID) ([]any, error) {
	return s.getServicesInstances(s.svcs.GetByIDs(ids))
}

func (s *Scope) GetServicesInChain(ids ...ID) ([]any, error) {
	var defs []*ServiceDefinition
	for scope := range s.Chain() {
		defs = append(defs, scope.svcs.GetByIDs(ids)...)
	}
	return s.getServicesInstances(defs)
}

func (s *Scope) GetServicesIDsByType(typ reflect.Type) []ID {
	return s.svcs.GetIDsByType(typ)
}

func (s *Scope) GetServicesIDsByTypeInChain(typ reflect.Type) (ids []ID) {
	for scope := range s.Chain() {
		ids = append(ids, scope.GetServicesIDsByType(typ)...)
	}
	return ids
}

func (s *Scope) GetServicesByType(typ reflect.Type) ([]any, error) {
	return s.GetServices(s.GetServicesIDsByType(typ)...)
}

func (s *Scope) GetServicesByTypeInChain(typ reflect.Type) ([]any, error) {
	return s.GetServicesInChain(s.GetServicesIDsByTypeInChain(typ)...)
}

func (s *Scope) GetServicesIDsByLabel(label Label) []ID {
	return s.svcs.GetIDsByLabel(label)
}

func (s *Scope) GetServicesIDsByLabelInChain(label Label) (ids []ID) {
	for scope := range s.Chain() {
		ids = append(ids, scope.GetServicesIDsByLabel(label)...)
	}
	return ids
}

func (s *Scope) GetServicesByLabel(label Label) ([]any, error) {
	return s.GetServices(s.GetServicesIDsByLabel(label)...)
}

func (s *Scope) GetServicesByLabelInChain(label Label) ([]any, error) {
	return s.GetServicesInChain(s.GetServicesIDsByLabelInChain(label)...)
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

func (s *Scope) ExecuteFunction(id ID) ([]any, error) {
	def, ok := s.funs.Get(id)
	if !ok {
		return nil, fmt.Errorf("function %s not found", id)
	}
	return s.executeFunction(def)
}

func (s *Scope) ExecuteFunctionInChain(id ID) ([]any, error) {
	for scope := range s.Chain() {
		def, ok := s.funs.Get(id)
		if ok {
			return scope.executeFunction(def)
		}
	}
	return nil, fmt.Errorf("function %s not found", id)
}

func (s *Scope) ExecuteFunctions(ids ...ID) (results [][]any, joinedErrs error) {
	defs := s.funs.GetByIDs(ids)
	if len(defs) == 0 {
		return nil, errors.New("found no functions for given IDs")
	}
	return s.executeFunctions(defs)
}

func (s *Scope) ExecuteFunctionsInChain(ids ...ID) (results [][]any, joinedErrs error) {
	var defs []*FunctionDefinition
	for scope := range s.Chain() {
		defs = append(defs, scope.funs.GetByIDs(ids)...)
	}
	if len(defs) == 0 {
		return nil, errors.New("found no functions for given IDs")
	}
	return s.executeFunctions(defs)
}

func (s *Scope) GetFunctionsIDsByType(typ reflect.Type) []ID {
	return s.funs.GetIDsByType(typ)
}

func (s *Scope) GetFunctionsIDsByTypeInChain(typ reflect.Type) (ids []ID) {
	for scope := range s.Chain() {
		ids = append(ids, scope.GetFunctionsIDsByType(typ)...)
	}
	return ids
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

func (s *Scope) GetFunctionsIDsByLabelInChain(label Label) (ids []ID) {
	for scope := range s.Chain() {
		ids = append(ids, scope.GetFunctionsIDsByLabel(label)...)
	}
	return ids
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

func (s *Scope) GetBoundArgInChain(typ reflect.Type) (Arg, bool) {
	for scope := range s.Chain() {
		boundTo, ok := scope.GetBoundArg(typ)
		if ok {
			return boundTo, true
		}
	}
	return nil, false
}

func (s *Scope) getServiceInstance(def *ServiceDefinition) (any, error) {
	svc, ok := s.instances[def.ID()]
	if ok {
		return svc, nil
	}

	svc, err := s.instantiate(def)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to instantiate service %s", def)
	}

	return svc, nil
}

func (s *Scope) getServicesInstances(defs []*ServiceDefinition) (svcs []any, joinedErrs error) {
	svcs = make([]any, len(defs))
	for i, def := range defs {
		svc, err := s.getServiceInstance(def)
		svcs[i] = svc
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return svcs, joinedErrs
}

func (s *Scope) instantiate(def *ServiceDefinition) (any, error) {
	svc, err := def.factory.Execute(def.EffectiveScope())
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute factory for service %s", def)
	}

	if def.shared {
		s.instances[def.id] = svc
	}

	for _, method := range def.MethodCalls() {
		err = method.Execute(def.EffectiveScope())
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to execute method %s of service %s", method, def)
		}
	}

	return svc, nil
}

func (s *Scope) executeFunction(def *FunctionDefinition) ([]any, error) {
	res, err := def.function.Execute(def.EffectiveScope())
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute function %s", def)
	}
	return lo.Map(res, func(v reflect.Value, _ int) any { return v.Interface() }), nil
}

func (s *Scope) executeFunctions(defs []*FunctionDefinition) (results [][]any, joinedErrs error) {
	results = make([][]any, len(defs))
	for i, def := range defs {
		res, err := s.executeFunction(def)
		results[i] = res
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return results, joinedErrs
}

func (s *Scope) ServiceDefinitionsSeq() iter.Seq[*ServiceDefinition] {
	return s.svcs.Seq()
}

func (s *Scope) ServiceDefinitionsInChainSeq() iter.Seq[*ServiceDefinition] {
	return func(yield func(*ServiceDefinition) bool) {
		for scope := range s.Chain() {
			for def := range scope.ServiceDefinitionsSeq() {
				if !yield(def) {
					return
				}
			}
		}
	}
}

func (s *Scope) GetServiceDefinitions() []*ServiceDefinition {
	return iterx.Collect(s.ServiceDefinitionsSeq())
}

func (s *Scope) GetServiceDefinitionsInChain() []*ServiceDefinition {
	return iterx.Collect(s.ServiceDefinitionsInChainSeq())
}

func (s *Scope) GetServiceDefinitionsByType(typ reflect.Type) []*ServiceDefinition {
	return s.svcs.GetByType(typ)
}

func (s *Scope) GetServiceDefinitionsByTypeInChain(typ reflect.Type) (defs []*ServiceDefinition) {
	for scope := range s.Chain() {
		defs = append(defs, scope.GetServiceDefinitionsByType(typ)...)
	}
	return defs
}

func (s *Scope) GetServiceDefinitionsByLabel(label Label) []*ServiceDefinition {
	return s.svcs.GetByLabel(label)
}

func (s *Scope) GetServiceDefinitionsByLabelInChain(label Label) (defs []*ServiceDefinition) {
	for scope := range s.Chain() {
		defs = append(defs, scope.GetServiceDefinitionsByLabel(label)...)
	}
	return defs
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
	return func(yield func(*FunctionDefinition) bool) {
		for scope := range s.Chain() {
			for def := range scope.FunctionDefinitionsSeq() {
				if !yield(def) {
					return
				}
			}
		}
	}
}

func (s *Scope) GetFunctionDefinitions() []*FunctionDefinition {
	return iterx.Collect(s.FunctionDefinitionsSeq())
}

func (s *Scope) GetFunctionDefinitionsInChain() []*FunctionDefinition {
	return iterx.Collect(s.FunctionDefinitionsInChainSeq())
}

func (s *Scope) GetFunctionDefinitionsByType(typ reflect.Type) []*FunctionDefinition {
	return s.funs.GetByType(typ)
}

func (s *Scope) GetFunctionDefinitionsByTypeInChain(typ reflect.Type) (defs []*FunctionDefinition) {
	for scope := range s.Chain() {
		defs = append(defs, scope.GetFunctionDefinitionsByType(typ)...)
	}
	return defs
}

func (s *Scope) GetFunctionDefinitionsByLabel(label Label) []*FunctionDefinition {
	return s.funs.GetByLabel(label)
}

func (s *Scope) GetFunctionDefinitionsByLabelInChain(label Label) (defs []*FunctionDefinition) {
	for scope := range s.Chain() {
		defs = append(defs, scope.GetFunctionDefinitionsByLabel(label)...)
	}
	return defs
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

func (s *Scope) GetBindings() []*InterfaceBinding {
	return iterx.Collect(s.BindingsSeq())
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
	return iterx.Collect(r.Seq())
}

func (r *DefinitionRegistry[Def]) Len() int {
	return r.byID.Len()
}

func (r *DefinitionRegistry[Def]) getIDs(defs []Def) []ID {
	return lo.Map(defs, func(d Def, _ int) ID { return d.ID() })
}
