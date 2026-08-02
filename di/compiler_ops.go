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
	for _, def := range builder.ServiceDefinitionsSeq() {
		for i, slot := range def.Factory().Args().Slots() {
			err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
			if err != nil {
				builder.ReportError(AtServiceArg(def, slot), err, "could not bind argument %d of service %s", i, def)
			}
		}

		for _, method := range def.MethodCalls() {
			for i, slot := range method.Args().Slots() {
				err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
				if err != nil {
					builder.ReportError(AtServiceArg(def, slot), err, "could not bind argument %d of method %s", i, method)
				}
			}
		}
	}
	for _, def := range builder.FunctionDefinitionsSeq() {
		for i, slot := range def.Func().Args().Slots() {
			err := p.checkAndBind(def.EffectiveScope(), def.ID(), slot)
			if err != nil {
				builder.ReportError(AtFunctionArg(def, slot), err, "could not bind argument %d of function %s", i, def)
			}
		}
	}

	return nil
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

// Run stops at the first slot it cannot fill. Wiring the rest would be guesswork
// on top of a container that is already wrong.
func (p *AutowiringPass) Run(builder *ContainerBuilder) error {
	for _, def := range builder.ServiceDefinitionsSeq() {
		if !def.IsAutowired() {
			continue
		}

		if slot, err := p.autowire(def.Factory().Args()); err != nil {
			builder.ReportError(AtServiceArg(def, slot), err, "failed to autowire service %s", def)
			return nil
		}
		for _, method := range def.MethodCalls() {
			if slot, err := p.autowire(method.Args()); err != nil {
				builder.ReportError(AtServiceArg(def, slot), err, "failed to autowire method %s", method)
				return nil
			}
		}
	}

	for _, def := range builder.FunctionDefinitionsSeq() {
		if !def.IsAutowired() {
			continue
		}

		if slot, err := p.autowire(def.Func().Args()); err != nil {
			builder.ReportError(AtFunctionArg(def, slot), err, "failed to autowire function %s", def)
			return nil
		}
	}
	return nil
}

// autowire fills every empty slot by type, and names the one it could not fill.
func (p *AutowiringPass) autowire(args *ArgList) (*Slot, error) {
	for _, slot := range args.Slots() {
		if slot.IsFilled() {
			continue
		}

		if slot.IsSlice() {
			if err := slot.Fill(NewFlexibleSliceArg(slot.ElemType(), slot.IsVariadicSlice())); err != nil {
				return slot, err
			}
			continue
		}

		if err := slot.Fill(NewTypeArg(slot.Type(), false)); err != nil {
			return slot, err
		}
	}
	return nil, nil
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
	for _, def := range builder.ServiceDefinitionsSeq() {
		serviceArg := func(slot *Slot) Site { return AtServiceArg(def, slot) }

		p.reportFaults(builder, def.EffectiveScope(), def.Factory().Args(), serviceArg,
			fmt.Sprintf("invalid service %s: invalid factory %s", def, def.Factory()))

		for _, method := range def.MethodCalls() {
			p.reportFaults(builder, def.EffectiveScope(), method.Args(), serviceArg,
				fmt.Sprintf("invalid service %s: invalid method %s", def, method))
		}
	}

	for _, def := range builder.FunctionDefinitionsSeq() {
		p.reportFaults(builder, def.EffectiveScope(), def.Func().Args(),
			func(slot *Slot) Site { return AtFunctionArg(def, slot) },
			fmt.Sprintf("invalid function %s", def))
	}

	return nil
}

// reportFaults reports every argument that will not resolve, one diagnostic per
// argument, so that the dependency graph can show each against the argument it
// is about. owner is what the call is called, for the error the build fails with.
func (p *ArgValidationPass) reportFaults(builder *ContainerBuilder, scope *Scope, args *ArgList, site func(*Slot) Site, owner string) {
	for _, slot := range args.Slots() {
		if !slot.IsFilled() {
			// A variadic slot nobody filled is an optional dependency nothing
			// provides. The call gets an empty slice, the same as an autowired one
			// that finds no services.
			if !slot.IsVariadicSlice() {
				builder.ReportError(site(slot), fmt.Errorf("argument %d is not set", slot.Index()), "%s", owner)
			}
			continue
		}
		err := ValidateArg(scope, slot.Arg())
		if err != nil {
			builder.ReportError(site(slot), err, "%s: invalid argument %d", owner, slot.Index())
		}
	}
}

// NewCycleValidationPass returns a compiler pass that rejects a container whose
// services depend on each other in a circle.
func NewCycleValidationPass() CompilerOp {
	return CompilerOpFunc(func(builder *ContainerBuilder) error {
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
						builder.ReportError(AtServiceArg(def, slot),
							fmt.Errorf("service %s has a circular dependency on %s", def, argDef), "")
					}
				}
			}
		}

		return nil
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
		// The definition that failed, for the graph. A factory error says what went
		// wrong and not what was being built, so the message says both.
		var failed Site

		_, err := withInstantiationContext(builder.Container(), func(ic *instantiationContext) (any, error) {
			for scope, def := range builder.ServiceDefinitionsSeq() {
				if def.IsLazy() {
					continue
				}
				_, err := scope.getService(ic, def.ID())
				if err != nil {
					failed = AtService(def)
					return nil, errorsx.Wrapf(err, "failed to initialise eager service %s", def)
				}
			}
			for scope, def := range builder.FunctionDefinitionsSeq() {
				if def.IsLazy() {
					continue
				}
				_, err := scope.executeFunction(ic, def)
				if err != nil {
					failed = AtFunction(def)
					return nil, errorsx.Wrapf(err, "failed to execute eager function %s", def)
				}
			}
			return nil, nil
		})
		if err != nil {
			builder.Report(eagerDiagnostic(failed, err))
		}
		return nil
	})
}

// eagerDiagnostic says what failed, as near to the argument as the failure can be
// pinned.
//
// Where an argument would not resolve, that argument is what has to change, and
// the message is the fault alone: the element it is shown against already says
// which argument of which definition. Where a factory returned an error of its
// own, no argument is implicated and the definition is the whole answer.
//
// Either way the build fails with the chain, so a reader in a terminal still sees
// how the container got from an eager service to the fault.
func eagerDiagnostic(failed Site, err error) Diagnostic {
	d := Diagnostic{Severity: SeverityError, Site: failed, Message: err.Error(), Err: err}
	if ae := deepestArgError(err); ae != nil {
		d.Site, d.Message = ae.site, ae.cause.Error()
	}
	return d
}
