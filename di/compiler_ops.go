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

	for _, def := range builder.ServiceDefinitionsSeq() {
		for i, slot := range def.Factory().Args().Slots() {
			err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
			if err != nil {
				joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "could not bind argument %d of service %s", i, def))
			}
		}

		for _, method := range def.MethodCalls() {
			for i, slot := range method.Args().Slots() {
				err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
				if err != nil {
					joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "could not bind argument %d of method %s", i, method))
				}
			}
		}
	}
	for _, def := range builder.FunctionDefinitionsSeq() {
		for i, slot := range def.Func().Args().Slots() {
			err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
			if err != nil {
				joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "could not bind argument %d of function %s", i, def))
			}
		}
	}

	return joinedErr
}

func (p *InterfaceBindingPass) checkAndBind(scope *Scope, parentID ID, slot *Slot) error {
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

	if _, ok := scope.GetBoundArgInChain(iface); ok {
		return nil // The interface is already bound, nothing to do.
	}

	impls := p.findImplementations(scope, parentID, iface)
	if len(impls) == 0 {
		return nil // No implementations found, nothing to bind.
	}

	var bindTo Arg
	if slot.IsSlice() {
		args := lo.Map(impls, func(impl *ServiceDefinition, _ int) Arg {
			arg, _ := NewRefArg(impl)
			return arg
		})
		bindTo, _ = NewCompoundArg(iface, args...) // No error possible - we know that impls implement iface.
	} else {
		if len(impls) > 1 {
			return fmt.Errorf("multiple implementations of interface %s found: %s", util.Signature(iface), impls)
		}
		bindTo, _ = NewRefArg(impls[0])
	}

	binding, err := NewInterfaceBinding(iface, bindTo)
	if err != nil {
		return err
	}

	scope.AddBindings(binding)

	return nil
}

func (p *InterfaceBindingPass) findImplementations(scope *Scope, parentID ID, iface reflect.Type) []*ServiceDefinition {
	var impls []*ServiceDefinition
	for def := range scope.ServiceDefinitionsInChainSeq() {
		if def.Type() != iface && def.ID() != parentID && def.Type().Implements(iface) {
			impls = append(impls, def)
		}
	}
	return impls
}

// AutowiringPass fills in the arguments nobody wrote, by type. It is exported as
// an example of the shape a pass takes.
type AutowiringPass struct{}

// NewAutowiringPass returns a compiler pass that automatically wires the arguments
// of factories, method calls and functions based on their types.
func NewAutowiringPass() CompilerOp { return new(AutowiringPass) }

func (p *AutowiringPass) Run(builder *ContainerBuilder) error {
	for _, def := range builder.ServiceDefinitionsSeq() {
		if !def.IsAutowired() {
			continue
		}

		err := p.autowire(def.Factory().Args())
		if err != nil {
			return errorsx.Wrapf(err, "failed to autowire service %s", def)
		}
		for _, method := range def.MethodCalls() {
			err := p.autowire(method.Args())
			if err != nil {
				return errorsx.Wrapf(err, "failed to autowire method %s", method)
			}
		}
	}

	for _, def := range builder.FunctionDefinitionsSeq() {
		if !def.IsAutowired() {
			continue
		}

		err := p.autowire(def.Func().Args())
		if err != nil {
			return errorsx.Wrapf(err, "failed to autowire function %s", def)
		}
	}
	return nil
}

func (p *AutowiringPass) autowire(args *ArgList) error {
	for _, slot := range args.Slots() {
		if slot.IsFilled() {
			continue
		}

		if slot.IsSlice() {
			if err := slot.Fill(NewFlexibleSliceArg(slot.ElemType(), slot.IsVariadicSlice())); err != nil {
				return err
			}
			continue
		}

		if err := slot.Fill(NewTypeArg(slot.Type(), false)); err != nil {
			return err
		}
	}
	return nil
}

// stage: Validation

// ArgValidationPass rejects an argument that names a dependency the container
// does not have, and one nothing has filled. A variadic slot is exempt: no
// arguments is a valid call.
type ArgValidationPass struct{}

// NewArgValidationPass returns a compiler pass that validates all arguments of factories, method calls and functions
// that reference other services. It ensures that the referenced services exist.
func NewArgValidationPass() CompilerOp {
	return new(ArgValidationPass)
}

func (p *ArgValidationPass) Run(builder *ContainerBuilder) error {
	var joinedErr error

	for _, def := range builder.ServiceDefinitionsSeq() {
		err := p.validateArgs(def.EffectiveScope(), def.Factory().Args())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid service %s: invalid factory %s", def, def.Factory()))
		}

		for _, method := range def.MethodCalls() {
			err := p.validateArgs(def.EffectiveScope(), method.Args())
			if err != nil {
				joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid service %s: invalid method %s", def, method))
			}
		}
	}

	for _, def := range builder.FunctionDefinitionsSeq() {
		err := p.validateArgs(def.EffectiveScope(), def.Func().Args())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid function %s", def))
		}
	}

	return joinedErr
}

func (p *ArgValidationPass) validateArgs(scope *Scope, args *ArgList) error {
	var joinedErr error
	for i, slot := range args.Slots() {
		if !slot.IsFilled() {
			// A variadic slot nobody filled is an optional dependency nothing
			// provides. The call gets an empty slice, the same as an autowired one
			// that finds no services.
			if !slot.IsVariadicSlice() {
				joinedErr = errors.Join(joinedErr, fmt.Errorf("argument %d is not set", i))
			}
			continue
		}
		err := ValidateArg(scope, slot.Arg())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid argument %d", i))
		}
	}
	return joinedErr
}

// NewCycleValidationPass returns a compiler pass that rejects a container whose
// services depend on each other in a circle.
func NewCycleValidationPass() CompilerOp {
	return CompilerOpFunc(func(builder *ContainerBuilder) error {
		var joinedErr error
		g := graph.New((*ServiceDefinition).ID, graph.PreventCycles(), graph.Directed())

		for _, def := range builder.ServiceDefinitionsSeq() {
			err := g.AddVertex(def)
			if err != nil {
				return err
			}
		}

		for _, def := range builder.ServiceDefinitionsSeq() {
			for _, slot := range def.Factory().Args().Slots() {
				for _, id := range ResolveArgIDs(def.EffectiveScope(), slot.Arg()) {
					err := g.AddEdge(def.ID(), id)
					if errors.Is(err, graph.ErrEdgeAlreadyExists) {
						continue
					}
					if errors.Is(err, graph.ErrEdgeCreatesCycle) {
						argDef, _ := def.EffectiveScope().GetServiceDefinitionInChain(id) // Definition must exist, it's been validated by the resolver.
						joinedErr = errors.Join(joinedErr, fmt.Errorf("service %s has a circular dependency on %s", def, argDef))
					}
				}
			}
		}

		return joinedErr
	})
}

// stage: Finalization

// NewEagerInitPass returns a compiler pass that builds the services and runs the
// functions that asked not to wait.
//
// The whole pass is one call into the container, so a build wires services the
// same way a later request for one would.
func NewEagerInitPass() CompilerOp {
	return CompilerOpFunc(func(builder *ContainerBuilder) error {
		_, err := withInstantiationContext(builder.Container(), func(ic *instantiationContext) (any, error) {
			for scope, def := range builder.ServiceDefinitionsSeq() {
				if def.IsLazy() {
					continue
				}
				_, err := scope.getService(ic, def.ID())
				if err != nil {
					return nil, errorsx.Wrapf(err, "failed to initialise eager service %s", def)
				}
			}
			for scope, def := range builder.FunctionDefinitionsSeq() {
				if def.IsLazy() {
					continue
				}
				_, err := scope.executeFunction(ic, def)
				if err != nil {
					return nil, errorsx.Wrapf(err, "failed to execute eager function %s", def)
				}
			}
			return nil, nil
		})
		return err
	})
}
