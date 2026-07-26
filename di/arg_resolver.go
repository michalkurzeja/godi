package di

import (
	"fmt"
	"reflect"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// ValidateArg reports whether the argument can be resolved in the scope.
func ValidateArg(scope *Scope, arg Arg) error {
	if arg == nil {
		return fmt.Errorf("unsupported arg type %T", arg)
	}
	return arg.validate(scope)
}

// ResolveArg produces the value the argument stands for.
func ResolveArg(scope *Scope, arg Arg) (any, error) {
	if arg == nil {
		return reflect.Value{}, fmt.Errorf("unsupported arg type %T", arg)
	}
	return arg.resolve(scope)
}

// ResolveArgIDs lists the definitions the argument resolves to. It is empty for
// an argument that names no service, a literal above all.
func ResolveArgIDs(scope *Scope, arg Arg) []ID {
	if arg == nil {
		return nil
	}
	return arg.resolveIDs(scope)
}

// ArgResolver resolves arguments. Every kind of argument now does its own
// resolving, so this is the same three functions above under an older name.
type ArgResolver struct{}

func NewArgResolver() *ArgResolver {
	return &ArgResolver{}
}

func (r *ArgResolver) Validate(scope *Scope, arg Arg) error {
	return ValidateArg(scope, arg)
}

func (r *ArgResolver) Resolve(scope *Scope, arg Arg) (any, error) {
	return ResolveArg(scope, arg)
}

func (r *ArgResolver) ResolveIDs(scope *Scope, arg Arg) []ID {
	return ResolveArgIDs(scope, arg)
}

func convertSlice(vs []any, elemType reflect.Type) (any, error) {
	sl := reflect.MakeSlice(reflect.SliceOf(elemType), 0, len(vs))
	for _, v := range vs {
		rv := reflect.ValueOf(v)
		if !rv.Type().AssignableTo(elemType) {
			return nil, fmt.Errorf("type %s is not assignable to %s", util.Signature(rv.Type()), util.Signature(elemType))
		}
		sl = reflect.Append(sl, rv)
	}
	return sl.Interface(), nil
}
