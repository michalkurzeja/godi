package di

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/errorsx"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

var resolver = NewArgResolver()

func ValidateArg(scope *Scope, arg Arg) error {
	return resolver.Validate(scope, arg)
}

func ResolveArg(scope *Scope, arg Arg) (any, error) {
	return resolver.Resolve(scope, arg)
}

func ResolveArgIDs(scope *Scope, arg Arg) []ID {
	return resolver.ResolveIDs(scope, arg)
}

type ArgResolver struct {
	literalArgResolver       *literalArgResolver
	refArgResolver           *refArgResolver
	typeArgResolver          *typeArgResolver
	labelArgResolver         *labelArgResolver
	flexibleSliceArgResolver *flexibleSliceArgResolver
	compoundArgResolver      *compoundArgResolver
}

func NewArgResolver() *ArgResolver {
	r := &ArgResolver{}
	r.literalArgResolver = &literalArgResolver{}
	r.refArgResolver = &refArgResolver{}
	r.typeArgResolver = &typeArgResolver{resolver: r}
	r.labelArgResolver = &labelArgResolver{resolver: r}
	r.flexibleSliceArgResolver = &flexibleSliceArgResolver{resolver: r}
	r.compoundArgResolver = &compoundArgResolver{resolver: r}
	return r
}

func (r *ArgResolver) Validate(scope *Scope, arg Arg) error {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.Validate(scope, a)
	case *refArg:
		return r.refArgResolver.Validate(scope, a)
	case *typeArg:
		return r.typeArgResolver.Validate(scope, a)
	case *labelArg:
		return r.labelArgResolver.Validate(scope, a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.Validate(scope, a)
	case *compoundArg:
		return r.compoundArgResolver.Validate(scope, a)
	default:
		return fmt.Errorf("unsupported arg type %T", arg)
	}
}

func (r *ArgResolver) Resolve(scope *Scope, arg Arg) (any, error) {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.Resolve(scope, a)
	case *refArg:
		return r.refArgResolver.Resolve(scope, a)
	case *typeArg:
		return r.typeArgResolver.Resolve(scope, a)
	case *labelArg:
		return r.labelArgResolver.Resolve(scope, a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.Resolve(scope, a)
	case *compoundArg:
		return r.compoundArgResolver.Resolve(scope, a)
	default:
		return reflect.Value{}, fmt.Errorf("unsupported arg type %T", arg)
	}
}

func (r *ArgResolver) ResolveIDs(scope *Scope, arg Arg) []ID {
	switch a := arg.(type) {
	case *literalArg:
		return r.literalArgResolver.ResolveIDs(scope, a)
	case *refArg:
		return r.refArgResolver.ResolveIDs(scope, a)
	case *typeArg:
		return r.typeArgResolver.ResolveIDs(scope, a)
	case *labelArg:
		return r.labelArgResolver.ResolveIDs(scope, a)
	case *flexibleSliceArg:
		return r.flexibleSliceArgResolver.ResolveIDs(scope, a)
	case *compoundArg:
		return r.compoundArgResolver.ResolveIDs(scope, a)
	default:
		return nil
	}
}

type literalArgResolver struct{}

func (r *literalArgResolver) Validate(_ *Scope, _ *literalArg) error {
	return nil
}

func (r *literalArgResolver) Resolve(_ *Scope, a *literalArg) (any, error) {
	return a.v, nil
}

func (r *literalArgResolver) ResolveIDs(_ *Scope, _ *literalArg) []ID {
	return nil
}

type refArgResolver struct{}

func (r *refArgResolver) Validate(scope *Scope, a *refArg) error {
	if !scope.HasServiceInChain(a.def.ID()) {
		return fmt.Errorf("service %s not found", a.def.ID())
	}
	return nil
}

func (r *refArgResolver) Resolve(scope *Scope, a *refArg) (any, error) {
	v, err := scope.GetServiceInChain(a.def.ID())
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve ID arg")
	}
	if v == nil {
		return nil, fmt.Errorf("service %s not found", a.def.ID())
	}
	return v, nil
}

func (r *refArgResolver) ResolveIDs(_ *Scope, a *refArg) []ID {
	return []ID{a.def.ID()}
}

type typeArgResolver struct{ resolver *ArgResolver }

func (r *typeArgResolver) Validate(scope *Scope, a *typeArg) error {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return r.resolver.Validate(scope, boundTo)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.typ)
	if len(ids) == 0 {
		return fmt.Errorf("no services found for type %s", util.Signature(a.typ))
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.typ))
	}
	return nil
}

func (r *typeArgResolver) Resolve(scope *Scope, a *typeArg) (any, error) {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return r.resolver.Resolve(scope, boundTo)
	}
	vals, err := scope.GetServicesByTypeInChain(a.typ)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve type arg")
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("no services found for type %s", util.Signature(a.typ))
	}
	if a.slice {
		return convertSlice(vals, a.typ)
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for type %s", util.Signature(a.typ))
	}
	return vals[0], nil
}

func (r *typeArgResolver) ResolveIDs(scope *Scope, a *typeArg) []ID {
	if boundTo, ok := scope.GetBoundArgInChain(a.typ); ok {
		return r.resolver.ResolveIDs(scope, boundTo)
	}
	return scope.GetServicesIDsByTypeInChain(a.typ)
}

type labelArgResolver struct{ resolver *ArgResolver }

func (r *labelArgResolver) Validate(scope *Scope, a *labelArg) error {
	ids := scope.GetServicesIDsByLabelInChain(a.label)
	if len(ids) == 0 {
		return fmt.Errorf("no services found with label %s", a.label)
	}
	if !a.slice && len(ids) > 1 {
		return fmt.Errorf("multiple services found with label %s", a.label)
	}
	return nil
}

func (r *labelArgResolver) Resolve(scope *Scope, a *labelArg) (any, error) {
	vals, err := scope.GetServicesByLabelInChain(a.label)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve type arg")
	}
	if len(vals) == 0 {
		return nil, fmt.Errorf("no services found with label %s", a.label)
	}
	if a.slice {
		return convertSlice(vals, a.typ)
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for label %s", a.label)
	}
	argType := reflect.TypeOf(vals[0])
	if argType != a.Type() {
		return nil, fmt.Errorf("service labeled as %s should be of type %s, got %s", a.label, util.Signature(a.Type()), util.Signature(argType))
	}
	return vals[0], nil
}

func (r *labelArgResolver) ResolveIDs(scope *Scope, a *labelArg) []ID {
	return scope.GetServicesIDsByLabelInChain(a.label)
}

type flexibleSliceArgResolver struct{ resolver *ArgResolver }

func (r *flexibleSliceArgResolver) Validate(scope *Scope, a *flexibleSliceArg) error {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return r.resolver.Validate(scope, boundTo)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.Type())
	if len(ids) > 1 {
		return fmt.Errorf("multiple services found for type %s", util.Signature(a.Type()))
	}
	if len(ids) == 1 {
		return nil // Slice type matched!
	}

	// Now let's try to match by the element type.
	elemType := a.Type().Elem()
	if boundTo, ok := scope.GetBoundArgInChain(elemType); ok {
		return r.resolver.Validate(scope, boundTo)
	}
	ids = scope.GetServicesIDsByTypeInChain(elemType)
	if len(ids) > 0 {
		return nil // Slice element type matched!
	}
	if a.allowEmpty {
		return nil
	}

	return fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (r *flexibleSliceArgResolver) Resolve(scope *Scope, a *flexibleSliceArg) (any, error) {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return r.resolver.Resolve(scope, boundTo)
	}
	vals, err := scope.GetServicesByTypeInChain(a.Type())
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve flexible slice arg")
	}
	if len(vals) > 1 {
		// This should never happen under normal circumstances - the built-in compiler passes verify args.
		return nil, fmt.Errorf("multiple services found for type %s", util.Signature(a.Type()))
	}
	if len(vals) == 1 {
		return vals[0], nil // Slice type matched!
	}

	// Now let's try to match by the element type.
	elemType := a.Type().Elem()
	if boundTo, ok := scope.GetBoundArgInChain(elemType); ok {
		return r.resolver.Resolve(scope, boundTo)
	}
	vals, err = scope.GetServicesByTypeInChain(elemType)
	if err != nil {
		return nil, errorsx.Wrap(err, "failed to resolve flexible slice arg element")
	}
	if len(vals) > 0 {
		return convertSlice(vals, elemType) // Slice element type matched!
	}
	if a.allowEmpty {
		return convertSlice(nil, elemType)
	}

	return nil, fmt.Errorf("no services found for type %s", util.Signature(a.Type()))
}

func (r *flexibleSliceArgResolver) ResolveIDs(scope *Scope, a *flexibleSliceArg) []ID {
	// First try to match by the slice type.
	if boundTo, ok := scope.GetBoundArgInChain(a.Type()); ok {
		return r.resolver.ResolveIDs(scope, boundTo)
	}
	ids := scope.GetServicesIDsByTypeInChain(a.Type())
	if len(ids) > 0 {
		return ids // Slice type matched!
	}

	// Now let's try to match by the element type.
	elemType := a.Type().Elem()
	if boundTo, ok := scope.GetBoundArgInChain(elemType); ok {
		return r.resolver.ResolveIDs(scope, boundTo)
	}
	return scope.GetServicesIDsByTypeInChain(elemType)
}

type compoundArgResolver struct {
	resolver *ArgResolver
}

func (r *compoundArgResolver) Validate(scope *Scope, a *compoundArg) error {
	var joinedErr error
	for i, arg := range a.args {
		err := r.resolver.Validate(scope, arg)
		if err != nil {
			joinedErr = errors.Join(joinedErr, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i))
		}
	}
	return joinedErr
}

func (r *compoundArgResolver) Resolve(scope *Scope, a *compoundArg) (any, error) {
	vals := make([]any, len(a.args))
	for i, arg := range a.args {
		v, err := r.resolver.Resolve(scope, arg)
		if err != nil {
			return nil, errorsx.Wrapf(err, "failed to resolve compound sub-arg %d", i)
		}
		vals[i] = v
	}
	return convertSlice(vals, a.typ)
}

func (r *compoundArgResolver) ResolveIDs(scope *Scope, a *compoundArg) []ID {
	return lo.FlatMap(a.args, func(a Arg, _ int) []ID {
		return r.resolver.ResolveIDs(scope, a)
	})
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
