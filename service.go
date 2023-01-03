package di

import (
	"fmt"
	"reflect"

	"github.com/samber/lo"
)

const notFound = -1

// Needed for comparisons later.
var errType = typeOf[error]()

// lazyService is a Node implementation that wraps a provider (constructor) function.
// It's used to defer the actual construction of the provided value until it's requested from the container.
// The provided value is constructed once and then cached.
type lazyService struct {
	id           string
	providerFn   reflect.Value
	providerDeps []Dependency
	deferredDeps []deferredDependency
	vType        reflect.Type
	vOut, errOut int

	built bool
	val   any
}

// newLazyService returns a new lazyService instance.
// It validates the provider function, it's input and the output.
func newLazyService(id string, vType reflect.Type, providerFn any, deps ...Dependency) (*lazyService, error) {
	// Validate `providerFn` signature.
	fnType := reflect.TypeOf(providerFn)
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("provider must be a function, got %s", fnType.Kind())
	}
	if fnType.NumOut() < 1 {
		return nil, fmt.Errorf("provider must return at least one value")
	}

	// Find which return values are `T` and `error`.
	vOut, errOut := notFound, notFound
	for i := 0; i < fnType.NumOut(); i++ {
		if fnType.Out(i).AssignableTo(vType) {
			if vOut != notFound {
				return nil, fmt.Errorf("provider must not return more than one value of type %s", fqn(vType))
			}
			vOut = i
		}
		if fnType.Out(i) == errType {
			if errOut != notFound {
				return nil, fmt.Errorf("provider must not return more than one value of type %s", errType)
			}
			errOut = i
		}
	}
	if vOut == notFound {
		return nil, fmt.Errorf("provider must return %s", fqn(vType))
	}

	depGroups := lo.GroupBy(deps, func(dep Dependency) bool { return dep.isDeferred() })

	// Prepare `providerFn` dependencies providers.
	normDeps, err := Dependencies(depGroups[false]).normaliseForFuncInput(
		lo.Times(fnType.NumIn(), func(i int) reflect.Type {
			return fnType.In(i)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("invalid dependencies of %s: %w", fqn(vType), err)
	}
	defDeps := make([]deferredDependency, len(depGroups[true]))
	for i, dep := range depGroups[true] {
		defDep, err := newDeferredDependency(dep, vType, dep.deferredTo, dep.typ())
		if err != nil {
			return nil, fmt.Errorf("invalid deferred dependency of %s: %w", fqn(vType), err)
		}
		defDeps[i] = defDep
	}

	return &lazyService{
		id:           id,
		providerFn:   reflect.ValueOf(providerFn),
		providerDeps: normDeps,
		deferredDeps: defDeps,
		vType:        vType,
		vOut:         vOut,
		errOut:       errOut,
	}, nil
}

// newLazyServiceWithAutoType returns a new lazyService instance with a type inferred from the provider function.
func newLazyServiceWithAutoType(id string, providerFn any, deps ...Dependency) (*lazyService, error) {
	fnType := reflect.TypeOf(providerFn)
	if fnType.Kind() != reflect.Func {
		return nil, fmt.Errorf("provider must be a function, got %s", fnType.Kind())
	}
	if fnType.NumOut() < 1 {
		return nil, fmt.Errorf("provider must return at least one value")
	}

	vType := fnType.Out(0)
	id = lo.Ternary(id == "", fqn(vType), id)

	return newLazyService(id, vType, providerFn, deps...)
}

func (s *lazyService) ID() string {
	return s.id
}

func (s *lazyService) Type() reflect.Type {
	return s.vType
}

func (s *lazyService) Value(c Container) (any, error) {
	if !s.built {
		val, err := s.build(c)
		if err != nil {
			return nil, err
		}

		s.val = val
		s.built = true

		err = s.injectDeferredDependencies(c)
		if err != nil {
			return nil, err
		}
	}

	return s.val, nil
}

func (s *lazyService) build(c Container) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = fmt.Errorf("building service %s failed: %w", s.ID(), e)
			} else {
				err = fmt.Errorf("building service %s failed: %v", s.ID(), r)
			}
		}
	}()

	in := make([]reflect.Value, len(s.providerDeps))
	for i, dep := range s.providerDeps {
		v, err := dep.resolve(c)
		if err != nil {
			return nil, fmt.Errorf("building service %s failed: %w", s.ID(), err)
		}
		in[i] = reflect.ValueOf(v)
	}

	call := s.providerFn.Call
	if s.providerFn.Type().IsVariadic() {
		call = s.providerFn.CallSlice
	}
	outs := call(in)

	if s.errOut != notFound {
		if errOut := outs[s.errOut].Interface(); errOut != nil {
			err = errOut.(error)
		}
	}

	return outs[s.vOut].Interface(), err
}

func (s *lazyService) injectDeferredDependencies(c Container) error {
	for _, dep := range s.deferredDeps {
		err := dep.Resolve(c, s.val)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *lazyService) providerDependencies() (nodes []Node) {
	deps := lo.Filter(s.providerDeps, func(dep Dependency, _ int) bool {
		return dep.isRef()
	})
	for _, dep := range deps {
		for _, ref := range dep.refs {
			nodes = append(nodes, ref)
		}
	}
	return nodes
}

func (s *lazyService) deferredDependencies() (nodes []Node) {
	deps := lo.Filter(s.deferredDeps, func(defDep deferredDependency, _ int) bool {
		return defDep.dep.isRef()
	})
	for _, dep := range deps {
		for _, ref := range dep.dep.refs {
			nodes = append(nodes, ref)
		}
	}
	return nodes
}
