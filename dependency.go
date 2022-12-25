package di

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

const autoPos = -1

var emptyDependency = Dependency{}

// Val returns a dependency that will always return the same value.
func Val(v any) Dependency {
	return Dependency{pos: autoPos, val: v}
}

// Ref returns a dependency that will resolve to a service with the given type.
// It can, optionally, be given an ID to disambiguate when there are multiple dependencies of the same type.
// Only the first ID will be taken into account.
func Ref[T any](id ...string) Dependency {
	t := typeOf[T]()

	theID := fqn(t)
	if len(id) > 0 {
		theID = id[0]
	}

	return Dependency{pos: autoPos, r: newRef(theID, t)}
}

func refDep(t reflect.Type) Dependency {
	return Dependency{pos: autoPos, r: newRef(fqn(t), t)}
}

// Dependency represents a service dependency which is resolved at (or slightly after) the creation time.
type Dependency struct {
	deferredTo string
	pos        int
	val        any
	r          *ref
}

// DeferTo returns a new Dependency which will be injected post-construction via the provided method.
func (d Dependency) DeferTo(method string) Dependency {
	d.deferredTo = method
	return d
}

func (d Dependency) Pos(pos int) Dependency {
	d.pos = pos
	return d
}

func (d Dependency) String() string {
	if d.isRef() {
		return fmt.Sprintf("ref(%s)", d.r.ID())
	}
	return fmt.Sprintf("%v", d.val)
}

func (d Dependency) hasAutoPos() bool {
	return d.pos == autoPos
}

func (d Dependency) isDeferred() bool {
	return d.deferredTo != ""
}
func (d Dependency) isRef() bool {
	return d.r != nil
}

func (d Dependency) typ() reflect.Type {
	if d.isRef() {
		return d.r.Type()
	}
	return reflect.TypeOf(d.val)
}

func (d Dependency) isEmpty() bool {
	return d == emptyDependency
}

func (d Dependency) resolve(c Container) (any, error) {
	if d.isRef() {
		node, err := c.Get(d.r.ID())
		if err != nil {
			return nil, err
		}

		return node.Value(c)
	}
	return d.val, nil
}

type Dependencies []Dependency

func (d Dependencies) normaliseForFuncInput(in []reflect.Type) (Dependencies, error) {
	if len(d) == 0 {
		return lo.Map(in, func(in reflect.Type, _ int) Dependency { return refDep(in) }), nil
	}
	if len(d) > len(in) {
		return nil, fmt.Errorf("provider expects %d argument(s), got %d", len(in), len(d))
	}

	const grpAutoPos, grpManualPos = true, false
	dd := lo.GroupBy(d, func(dep Dependency) bool { return dep.hasAutoPos() })

	deps := make(Dependencies, len(in))

	// Assign dependencies with manual positions first.
	for _, dep := range dd[grpManualPos] {
		if dep.pos >= len(deps) {
			return nil, fmt.Errorf("dependency %s has position %d but the provider only takes %d argument(s)", dep, dep.pos, len(deps))
		}
		if !dep.typ().AssignableTo(in[dep.pos]) {
			return nil, fmt.Errorf("dependency %s cannot be used as argument %d of the provider", dep, dep.pos)
		}
		deps[dep.pos] = dep
	}

	// Assign dependencies with auto positions in the first spot they match, in order.
	ddI := 0
	for i, dep := range deps {
		if !dep.isEmpty() {
			continue
		}
		if ddI < len(dd[grpAutoPos]) && dd[grpAutoPos][ddI].typ().AssignableTo(in[i]) {
			deps[i] = dd[grpAutoPos][ddI]
			ddI++
		} else {
			deps[i] = refDep(in[i])
		}
	}
	if ddI != len(dd[grpAutoPos]) {
		depsNotConsumed := strings.Join(toString(dd[grpAutoPos][ddI:]...), ", ")
		return nil, fmt.Errorf("argument(s) do not match provider inputs: [%s]", depsNotConsumed)
	}

	return deps, nil
}

type deferredDependency struct {
	dep    Dependency
	method reflect.Method
}

func newDeferredDependency(dep Dependency, receiverT reflect.Type, methodName string, argT reflect.Type) (deferredDependency, error) {
	extraInput := 1 // Method receiver.
	if receiverT.Kind() == reflect.Interface {
		extraInput = 0 // Interface methods do not have a receiver.
	}

	method, ok := receiverT.MethodByName(methodName)
	if !ok {
		return deferredDependency{}, fmt.Errorf(`method "%s" not found in type %s`, methodName, fqn(receiverT))
	}

	// The first argument is the receiver.
	if method.Type.NumIn() != 1+extraInput {
		return deferredDependency{}, fmt.Errorf(`method "%s" must have 1 input; got %d`, methodName, method.Type.NumIn()-extraInput)
	}

	// The first argument is the receiver.
	if !argT.AssignableTo(method.Type.In(0 + extraInput)) {
		return deferredDependency{}, fmt.Errorf(`expected method "%s" argument to be of type %s; got %s`, methodName, fqn(method.Type.In(0+extraInput)), fqn(argT))
	}
	if method.Type.NumOut() > 1 {
		return deferredDependency{}, fmt.Errorf(`method "%s" must not have more than 1 output; got %d`, methodName, method.Type.NumOut())
	}
	if method.Type.NumOut() == 1 && method.Type.Out(0) != errType {
		return deferredDependency{}, fmt.Errorf(`method "%s" must have no outputs or return an error; got %s`, methodName, fqn(method.Type.Out(0)))
	}

	return deferredDependency{dep: dep, method: method}, nil
}

func (d deferredDependency) Resolve(c Container, target any) error {
	depVal, err := d.dep.resolve(c)
	if err != nil {
		return err
	}

	out := d.getCallable(target).Call([]reflect.Value{reflect.ValueOf(target), reflect.ValueOf(depVal)})
	if len(out) == 1 {
		if errOut := out[0].Interface(); errOut != nil {
			return fmt.Errorf(`error while injecting %s via "%s": %w`, fqn(reflect.TypeOf(depVal)), d.method.Name, errOut.(error))
		}
		return nil
	}
	return nil
}

func (d deferredDependency) getCallable(target any) reflect.Value {
	if d.method.Func.IsValid() {
		return d.method.Func
	}
	// Method was earlier acquired from an interface, now we need to get it from the implementation (`target`).
	// We know the target has this method and that it has a valid signature, because we've already verified
	// that the target implements that interface.
	method, _ := reflect.TypeOf(target).MethodByName(d.method.Name)
	return method.Func
}
