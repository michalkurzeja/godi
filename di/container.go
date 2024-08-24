package di

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

type Container struct {
	services  *DefinitionRegistry[*ServiceDefinition]
	functions *DefinitionRegistry[*FunctionDefinition]
	bindings  map[reflect.Type]*InterfaceBinding

	resolver  ArgResolver
	instances map[ID]any
}

func NewContainer() *Container {
	c := &Container{
		services:  NewDefinitionRegistry[*ServiceDefinition](),
		functions: NewDefinitionRegistry[*FunctionDefinition](),

		bindings:  make(map[reflect.Type]*InterfaceBinding),
		instances: make(map[ID]any),
	}
	c.resolver = NewArgResolver(c)
	return c
}

func (c *Container) HasService(id ID) bool {
	return c.services.Contains(id)
}

func (c *Container) GetService(id ID) (any, error) {
	def, ok := c.services.Get(id)
	if !ok {
		return nil, nil
	}
	return c.getServiceInstance(def)
}

func (c *Container) GetServices(ids ...ID) ([]any, error) {
	defs := c.services.GetByIDs(ids)
	if len(defs) == 0 {
		return nil, nil
	}
	return c.getServicesInstances(defs)
}

func (c *Container) GetServicesIDsByType(typ reflect.Type) []ID {
	return c.services.GetIDsByType(typ)
}

func (c *Container) GetServicesByType(typ reflect.Type) ([]any, error) {
	return c.GetServices(c.GetServicesIDsByType(typ)...)
}

func (c *Container) GetServicesIDsByLabel(label Label) []ID {
	return c.services.GetIDsByLabel(label)
}

func (c *Container) GetServicesByLabel(label Label) ([]any, error) {
	return c.GetServices(c.GetServicesIDsByLabel(label)...)
}

func (c *Container) HasFunction(id ID) bool {
	return c.functions.Contains(id)
}

func (c *Container) ExecuteFunction(id ID) ([]any, error) {
	def, ok := c.functions.Get(id)
	if !ok {
		return nil, fmt.Errorf("function %s not found", id)
	}
	return c.executeFunction(def)
}

func (c *Container) ExecuteFunctions(ids ...ID) (results [][]any, joinedErrs error) {
	defs := c.functions.GetByIDs(ids)
	if len(defs) == 0 {
		return nil, errors.New("found no functions for given IDs")
	}
	return c.executeFunctions(defs)
}

func (c *Container) GetFunctionsIDsByType(typ reflect.Type) []ID {
	return c.functions.GetIDsByType(typ)
}

func (c *Container) ExecuteFunctionsByType(typ reflect.Type) ([][]any, error) {
	return c.ExecuteFunctions(c.GetFunctionsIDsByType(typ)...)
}

func (c *Container) GetFunctionsIDsByLabel(label Label) []ID {
	return c.functions.GetIDsByLabel(label)
}

func (c *Container) ExecuteFunctionsByLabel(label Label) ([][]any, error) {
	return c.ExecuteFunctions(c.GetFunctionsIDsByLabel(label)...)
}

func (c *Container) GetBindingFor(typ reflect.Type) (Arg, bool) {
	binding, ok := c.bindings[typ]
	if !ok {
		return nil, false
	}
	return binding.boundTo, true
}

func (c *Container) getServiceInstance(def *ServiceDefinition) (any, error) {
	svc, ok := c.instances[def.ID()]
	if ok {
		return svc, nil
	}

	svc, err := c.instantiate(def)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to instantiate service %s", def)
	}

	return svc, nil
}

func (c *Container) getServicesInstances(defs []*ServiceDefinition) (svcs []any, joinedErrs error) {
	svcs = make([]any, len(defs))
	for i, def := range defs {
		svc, err := c.getServiceInstance(def)
		svcs[i] = svc
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return svcs, joinedErrs
}

func (c *Container) instantiate(def *ServiceDefinition) (any, error) {
	svc, err := def.factory.Execute(c.resolver)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute factory for service %s", def)
	}

	if def.shared {
		c.instances[def.id] = svc
	}

	for _, method := range def.methodCalls {
		err = method.Execute(c.resolver)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to execute method %s of service %s", method, def)
		}
	}

	return svc, nil
}

func (c *Container) executeFunction(def *FunctionDefinition) ([]any, error) {
	res, err := def.function.Execute(c.resolver)
	if err != nil {
		return nil, errorsx.Wrapf(err, "failed to execute function %s", def)
	}
	return lo.Map(res, func(v reflect.Value, _ int) any { return v.Interface() }), nil
}

func (c *Container) executeFunctions(defs []*FunctionDefinition) (results [][]any, joinedErrs error) {
	results = make([][]any, len(defs))
	for i, def := range defs {
		res, err := c.executeFunction(def)
		results[i] = res
		joinedErrs = errors.Join(joinedErrs, err)
	}
	return results, joinedErrs
}

type Definition interface {
	ID() ID
	Type() reflect.Type
	GetLabels() []Label
}

type DefinitionRegistry[Def Definition] struct {
	defs []Def

	byID    map[ID]Def
	byType  map[reflect.Type][]Def
	byLabel map[Label][]Def
}

func NewDefinitionRegistry[Def Definition]() *DefinitionRegistry[Def] {
	r := &DefinitionRegistry[Def]{}
	r.Clear()
	return r
}

func (r *DefinitionRegistry[Def]) Add(defs ...Def) {
	for _, d := range defs {
		r.defs = append(r.defs, d)

		r.byID[d.ID()] = d
		r.byType[d.Type()] = append(r.byType[d.Type()], d)
		for _, label := range d.GetLabels() {
			r.byLabel[label] = append(r.byLabel[label], d)
		}
	}
}

func (r *DefinitionRegistry[Def]) Remove(ids ...ID) {
	for _, id := range ids {
		def, ok := r.byID[id]
		if !ok {
			continue
		}

		defEq := func(d Def) bool { return d.ID() == def.ID() }

		i := slices.IndexFunc(r.defs, defEq)
		if i != -1 {
			fastRemove(r.defs, i)
		}

		delete(r.byID, id)
		r.byType[def.Type()] = slices.DeleteFunc(r.byType[def.Type()], defEq)
		for _, label := range def.GetLabels() {
			r.byLabel[label] = slices.DeleteFunc(r.byLabel[label], defEq)
		}
	}
}

func fastRemove[T any](sl []T, i int) []T {
	sl[i] = sl[len(sl)-1]
	return sl[:len(sl)-1]
}

func (r *DefinitionRegistry[Def]) Clear() {
	r.defs = nil
	r.byID = make(map[ID]Def)
	r.byType = make(map[reflect.Type][]Def)
	r.byLabel = make(map[Label][]Def)
}

func (r *DefinitionRegistry[Def]) Contains(id ID) bool {
	_, ok := r.byID[id]
	return ok
}

func (r *DefinitionRegistry[Def]) Get(id ID) (Def, bool) {
	def, ok := r.byID[id]
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
	return r.getIDs(r.byType[typ])
}

func (r *DefinitionRegistry[Def]) GetByType(typ reflect.Type) []Def {
	return r.byType[typ]
}

func (r *DefinitionRegistry[Def]) GetIDsByLabel(label Label) []ID {
	return r.getIDs(r.byLabel[label])
}

func (r *DefinitionRegistry[Def]) GetByLabel(label Label) []Def {
	return r.byLabel[label]
}

func (r *DefinitionRegistry[Def]) GetAll() []Def {
	return lo.Filter(r.defs, func(d Def, _ int) bool { return d.ID() != "" })
}

func (r *DefinitionRegistry[Def]) getIDs(defs []Def) []ID {
	return lo.Map(defs, func(d Def, _ int) ID { return d.ID() })
}

func (c *Container) Print(w io.Writer) {
	Print(c, w)
}
