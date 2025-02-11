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

type autowiringPass struct{}

// NewAutowiringPass returns a compiler pass that automatically wires the arguments
// of factories, method calls and functions based on their types.
func NewAutowiringPass() CompilerOp { return new(autowiringPass) }

func (p *autowiringPass) Run(builder *ContainerBuilder) error {
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
				return errorsx.Wrapf(err, "failed to autowire smethod %s", method)
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

func (p *autowiringPass) autowire(args *ArgList) error {
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

type argValidationPass struct{}

// NewArgValidationPass returns a compiler pass that validates all arguments of factories, method calls and functions
// that reference other services. It ensures that the referenced services exist.
func NewArgValidationPass() CompilerOp {
	return new(argValidationPass)
}

func (p *argValidationPass) Run(builder *ContainerBuilder) error {
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

	for scope, def := range builder.FunctionDefinitionsSeq() {
		err := p.validateArgs(scope, def.Func().Args())
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "invalid function %s", def))
		}
	}

	return joinedErr
}

func (p *argValidationPass) validateArgs(scope *Scope, args *ArgList) error {
	var joinedErr error
	for i, slot := range args.Slots() {
		if !slot.IsFilled() {
			joinedErr = errors.Join(joinedErr, fmt.Errorf("argument %d is not set", i))
			continue
		}
		err := ValidateArg(scope, slot.Arg())
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
	}
}

// stage: Finalization

// NewEagerInitPass returns a compiler pass that initializes all eager services and functions.
func NewEagerInitPass() CompilerOpFunc {
	return func(builder *ContainerBuilder) error {
		for scope, def := range builder.ServiceDefinitionsSeq() {
			if def.IsLazy() {
				continue
			}
			_, err := scope.GetService(def.ID())
			if err != nil {
				return errorsx.Wrapf(err, "failed to initialise eager service %s", def)
			}
		}
		for scope, def := range builder.FunctionDefinitionsSeq() {
			if def.IsLazy() {
				continue
			}
			_, err := scope.ExecuteFunction(def.ID())
			if err != nil {
				return errorsx.Wrapf(err, "failed to execute eager function %s", def)
			}
		}
		return nil
	}
}
