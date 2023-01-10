package di

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/dominikbraun/graph"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

// CompilerPass is a component of the compiler that can inspect
// and modify the container.
type CompilerPass interface {
	Compile(builder *ContainerBuilder) error
}

type CompilerPassFunc func(builder *ContainerBuilder) error

func (fn CompilerPassFunc) Compile(builder *ContainerBuilder) error {
	return fn(builder)
}

// Stage: Optimisation

// InterfaceResolutionPass resolves interfaces to their implementations.
// It inspects arguments of factories and method calls of all definitions.
// If those arguments are interfaces, it tries to find a single implementation
// of that interface. If there is exactly one implementation, it creates an
// alias for that implementation, which allows the container to use it for instantiation.
type InterfaceResolutionPass struct{}

func NewInterfaceResolutionPass() CompilerPass {
	return InterfaceResolutionPass{}
}

func (p InterfaceResolutionPass) Compile(builder *ContainerBuilder) error {
	for _, def := range builder.GetDefinitions() {
		for _, arg := range def.GetFactory().GetArgs() {
			err := p.checkAndResolve(builder, arg)
			if err != nil {
				return err
			}
		}

		for _, method := range def.GetMethodCalls() {
			for _, arg := range method.GetArgs() {
				err := p.checkAndResolve(builder, arg)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p InterfaceResolutionPass) checkAndResolve(builder *ContainerBuilder, arg *FuncArgument) error {
	if !arg.IsEmpty() {
		return nil // The argument is already set, nothing to resolve.
	}
	if arg.Type().Kind() != reflect.Interface {
		return nil // Not an interface, nothing to resolve.
	}

	aliasID := fqn(arg.Type())

	// Check if the interface is already aliased. We don't need to resolve those.
	if _, ok := builder.GetAlias(aliasID); ok {
		return nil
	}

	impl, err := p.findImplementation(builder, arg.Type())
	if err != nil {
		return err
	}
	if impl == nil {
		return nil
	}

	builder.AddAliases(NewAlias(impl.ID(), aliasID))
	return nil
}

func (p InterfaceResolutionPass) findImplementation(builder *ContainerBuilder, iface reflect.Type) (*Definition, error) {
	var impls []*Definition
	for _, def := range builder.GetDefinitions() {
		if def.Of() != iface && def.Of().Implements(iface) {
			impls = append(impls, def)
		}
	}

	if len(impls) == 0 {
		return nil, nil
	}

	if len(impls) > 1 {
		ids := lo.Map(impls, func(def *Definition, _ int) ID { return def.ID() })
		return nil, fmt.Errorf("multiple implementations of %s found: %s", fqn(iface), ids)
	}

	return impls[0], nil
}

// NewAutowirePass returns a compiler pass that automatically wires the arguments
// of factories and method calls based on their types.
func NewAutowirePass() CompilerPassFunc {
	setReferences := func(args FuncArgumentsList) error {
		return args.ForEach(func(i uint, arg *FuncArgument) error {
			if !arg.IsEmpty() {
				return nil
			}

			return args.Set(i, NewReference(fqn(arg.Type()), arg.Type()))
		})
	}

	return func(builder *ContainerBuilder) error {
		for _, def := range builder.GetDefinitions() {
			if !def.IsAutowired() {
				continue
			}

			err := setReferences(def.GetFactory().GetArgs())
			if err != nil {
				return err
			}

			for _, method := range def.GetMethodCalls() {
				err := setReferences(method.GetArgs())
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
}

// Stage: Validation

// NewAliasValidationPass returns a compiler pass that validates all aliases.
// It ensures that they point to existing service definitions.
func NewAliasValidationPass() CompilerPassFunc {
	return func(builder *ContainerBuilder) error {
		var errs *multierror.Error
		for _, alias := range builder.GetAliases() {
			_, ok := builder.GetDefinition(alias.Target())
			if !ok {
				errs = multierror.Append(errs, fmt.Errorf("alias %s points to a non-existing service %s", alias.ID(), alias.Target()))
			}
		}
		return errs.ErrorOrNil()
	}
}

// NewReferenceValidationPass returns a compiler pass that validates all arguments of factories and method calls
// that reference other services. It ensures that the referenced services exist.
func NewReferenceValidationPass() CompilerPassFunc {
	return func(builder *ContainerBuilder) error {
		var errs *multierror.Error
		for _, def := range builder.GetDefinitions() {
			err := def.GetFactory().GetArgs().ForEach(func(i uint, arg *FuncArgument) error {
				if arg.IsEmpty() {
					return fmt.Errorf("argument %d of %s factory is not set", i, def)
				}

				ref, ok := arg.Argument().(*Reference)
				if !ok {
					return nil
				}

				refID := resolveAlias(builder, ref.ID())
				if _, ok = builder.GetDefinition(refID); !ok {
					return fmt.Errorf("service %s is not registered but is referenced by factory of: %s", refID, def)
				}
				return nil
			})
			errs = multierror.Append(errs, err)

			for _, method := range def.GetMethodCalls() {
				err := method.GetArgs().ForEach(func(i uint, arg *FuncArgument) error {
					// Skip the first argument (receiver) because it's empty until the actual call.
					if arg.IsEmpty() && i > 0 {
						return fmt.Errorf("argument %d of %s.%s is not set", i, def, method.Name())
					}

					ref, ok := arg.Argument().(*Reference)
					if !ok {
						return nil
					}

					refID := resolveAlias(builder, ref.ID())
					if _, ok = builder.GetDefinition(refID); !ok {
						return fmt.Errorf("service %s is not registered but is referenced by: %s.%s", refID, def, method.Name())
					}

					return nil
				})
				errs = multierror.Append(errs, err)
			}
		}
		return errs.ErrorOrNil()
	}
}

// NewCycleValidationPass returns a compiler pass that validates that there are no circular references.
func NewCycleValidationPass() CompilerPassFunc {
	hash := func(def *Definition) ID { return def.ID() }

	return func(builder *ContainerBuilder) error {
		var errs *multierror.Error
		g := graph.New(hash, graph.PreventCycles(), graph.Directed())

		for _, def := range builder.GetDefinitions() {
			err := g.AddVertex(def)
			if err != nil {
				return err
			}
		}

		for _, def := range builder.GetDefinitions() {
			err := def.factory.GetArgs().ForEach(func(i uint, arg *FuncArgument) error {
				ref, ok := arg.Argument().(*Reference)
				if !ok {
					return nil
				}

				refID := resolveAlias(builder, ref.ID())
				argDef, _ := builder.GetDefinition(refID)
				err := g.AddEdge(def.ID(), argDef.ID())
				if errors.Is(err, graph.ErrEdgeAlreadyExists) {
					return nil
				}
				if errors.Is(err, graph.ErrEdgeCreatesCycle) {
					return fmt.Errorf("service %s has a circular dependency on %s", def, argDef)
				}
				return err
			})
			errs = multierror.Append(errs, err)
		}

		return errs.ErrorOrNil()
	}
}

// Stage: PostValidation

// NewEagerInitPass returns a compiler pass that initializes all services that are marked as eager.
func NewEagerInitPass() CompilerPassFunc {
	return func(builder *ContainerBuilder) error {
		for _, def := range builder.GetDefinitions() {
			if def.IsLazy() {
				return nil
			}
			_, err := builder.container.get(def.ID(), false)
			if err != nil {
				return fmt.Errorf("failed to initialise eager service %s: %w", def, err)
			}
		}
		return nil
	}
}

func resolveAlias(builder *ContainerBuilder, id ID) ID {
	if alias, ok := builder.GetAlias(id); ok {
		return alias.Target()
	}
	return id
}
