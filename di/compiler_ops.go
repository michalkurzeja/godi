package di

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

// stage: Automation

type InterfaceBindingPass struct{}

func NewInterfaceBindingPass() CompilerOp { return new(InterfaceBindingPass) }

func (p *InterfaceBindingPass) Run(builder *ContainerBuilder) error {
	var joinedErr error

	for _, def := range builder.GetServiceDefinitions() {
		for i, slot := range def.GetFactory().Args().Slots() {
			err := p.checkAndBind(builder, def.ID(), slot)
			if err != nil {
				joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "could not bind argument %d of service %s", i, def))
			}
		}
	}

	return joinedErr
}

func (p *InterfaceBindingPass) checkAndBind(builder *ContainerBuilder, parentID ID, slot *Slot) error {
	if slot.IsFilled() {
		return nil // The argument is already set, nothing to bind.
	}

	iface := slot.Type()
	if slot.IsSlice() {
		iface = slot.ElemType()
	}

	if iface.Kind() != reflect.Interface {
		return nil // Not an interface, nothing to resolve.
	}

	if _, ok := builder.GetBinding(iface); ok {
		return nil // The interface is already bound, nothing to do.
	}

	impls := p.findImplementations(builder, parentID, iface)
	if len(impls) == 0 {
		return nil // No implementations found, nothing to bind.
	}
	if len(impls) > 1 {
		return fmt.Errorf("multiple implementations of interface %s found: %s", util.Signature(iface), impls)
	}

	impl := impls[0]
	boundTo := lo.TernaryF(slot.IsSlice(),
		func() Arg { a, _ := NewCompoundArg(iface, NewRefArg(impl)); return a }, // No error possible - we know that impl implements iface.
		func() Arg { return NewRefArg(impl) },
	)
	binding, err := NewInterfaceBinding(iface, boundTo)
	if err != nil {
		return err
	}
	builder.AddBindings(binding)
	return nil
}

func (p *InterfaceBindingPass) findImplementations(builder *ContainerBuilder, parentID ID, iface reflect.Type) []*ServiceDefinition {
	var impls []*ServiceDefinition
	for _, def := range builder.GetServiceDefinitions() {
		if def.Type() != iface && def.ID() != parentID && def.Type().Implements(iface) {
			impls = append(impls, def)
		}
	}
	return impls
}

type autowiringPass struct{}

// NewAutowiringPass returns a compiler pass that automatically wires the arguments
// of factories, method calls and functions based on their types.
func NewAutowiringPass() CompilerOp { return new(autowiringPass) }

func (p *autowiringPass) Run(builder *ContainerBuilder) error {
	for _, def := range builder.GetServiceDefinitions() {
		if !def.IsAutowired() {
			continue
		}

		err := p.autowire(def.GetFactory().Args())
		if err != nil {
			return errorsx.Wrapf(err, "failed to autowire service %s", def)
		}
		for _, method := range def.GetMethodCalls() {
			err := p.autowire(method.Args())
			if err != nil {
				return errorsx.Wrapf(err, "failed to autowire service %s", def)
			}
		}
	}

	for _, def := range builder.GetFunctionDefinitions() {
		if !def.IsAutowired() {
			continue
		}

		err := p.autowire(def.GetFunc().Args())
		if err != nil {
			return errorsx.Wrapf(err, "failed to autowire function %s", def)
		}
	}
	return nil
}

func (p *autowiringPass) autowire(args *ArgList) error {
	for _, slot := range args.Slots() {
		if slot.IsFilled() {
			continue
		}
		if slot.IsSlice() {
			return slot.Fill(NewFlexibleSliceArg(slot.ElemType(), slot.IsVariadicSlice()))
		}
		return slot.Fill(NewTypeArg(slot.Type(), false))
	}
	return nil
}

// stage: Validation

type argValidationPass struct{}

// NewArgValidationPass returns a compiler pass that validates all arguments of factories, method calls and functions
// that reference other services. It ensures that the referenced services exist.
func NewArgValidationPass() CompilerOp {
	return new(argValidationPass)
}

func (p *argValidationPass) Run(builder *ContainerBuilder) error {
	var joinedErr error

	for _, def := range builder.GetServiceDefinitions() {
		err := p.validateArgs(builder.container.resolver, def.GetFactory().Args())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid service %s: invalid factory %s", def, def.GetFactory()))
		}

		for _, method := range def.GetMethodCalls() {
			err := p.validateArgs(builder.container.resolver, method.Args())
			if err != nil {
				joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid service %s: invalid method %s", def, method))
			}
		}
	}

	for _, def := range builder.GetFunctionDefinitions() {
		err := p.validateArgs(builder.container.resolver, def.GetFunc().Args())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid function %s", def))
		}
	}

	return joinedErr
}

func (p *argValidationPass) validateArgs(resolver ArgResolver, args *ArgList) error {
	var joinedErr error
	for i, slot := range args.Slots() {
		if !slot.IsFilled() {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("argument %d is not set", i))
			continue
		}
		err := resolver.Validate(slot.Arg())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid argument %d", i))
		}
	}
	return joinedErr
}

// NewCycleValidationPass returns a compiler pass that validates that there are no circular references.
func NewCycleValidationPass() CompilerOpFunc {
	return func(builder *ContainerBuilder) error {
		var joinedErr error
		g := graph.New((*ServiceDefinition).ID, graph.PreventCycles(), graph.Directed())

		for _, def := range builder.GetServiceDefinitions() {
			err := g.AddVertex(def)
			if err != nil {
				return err
			}
		}

		for _, def := range builder.GetServiceDefinitions() {
			for _, slot := range def.GetFactory().Args().Slots() {
				for _, id := range builder.container.resolver.ResolveIDs(slot.Arg()) {
					err := g.AddEdge(def.ID(), id)
					if errors.Is(err, graph.ErrEdgeAlreadyExists) {
						continue
					}
					if errors.Is(err, graph.ErrEdgeCreatesCycle) {
						argDef, _ := builder.GetServiceDefinition(id) // Definition must exist, it's been validated earlier.
						joinedErr = errors.Join(joinedErr, fmt.Errorf("service %s has a circular dependency on %s", def, argDef))
					}
				}
			}
		}

		return joinedErr
	}
}

// stage: Finalization

// NewEagerInitPass returns a compiler pass that initializes all eager services and functions.
func NewEagerInitPass() CompilerOpFunc {
	return func(builder *ContainerBuilder) error {
		for _, def := range builder.GetServiceDefinitions() {
			if def.IsLazy() {
				continue
			}
			_, err := builder.container.GetService(def.ID())
			if err != nil {
				return errorsx.Wrapf(err, "failed to initialise eager service %s", def)
			}
		}
		for _, def := range builder.GetFunctionDefinitions() {
			if def.IsLazy() {
				continue
			}
			_, err := builder.container.ExecuteFunction(def.ID())
			if err != nil {
				return errorsx.Wrapf(err, "failed to execute eager function %s", def)
			}
		}
		return nil
	}
}
