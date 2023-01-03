package di

import (
	"fmt"
	"reflect"

	"github.com/samber/lo"
)

// FQN returns the fully qualified name of a type.
func FQN[T any]() string {
	return fqn(typeOf[T]())
}

// fqn returns the fully qualified name of a type.
// It's a reflection-based, internal implementation.
func fqn(typ reflect.Type) string {
	if typ.Kind() == reflect.Ptr {
		return "*" + fqn(typ.Elem())
	}
	if pkgPath := typ.PkgPath(); pkgPath != "" {
		return pkgPath + "." + typ.Name()
	}
	return typ.String()
}

// zero returns the zero value of a type.
func zero[T any]() (t T) {
	return
}

// typeOf returns a type object of the type parameter.
func typeOf[T any]() reflect.Type {
	typ := reflect.TypeOf(zero[T]())
	if typ == nil { // This happens when T is an interface.
		typ = reflect.TypeOf((*T)(nil)).Elem()
	}
	return typ
}

// toString returns string representations of values.
func toString[S fmt.Stringer](ss ...S) []string {
	return lo.Map(ss, func(s S, _ int) string {
		return s.String()
	})
}

func castSlice(s []any, to reflect.Type) (any, error) {
	if to.Kind() != reflect.Slice {
		return nil, fmt.Errorf("cannot cast %s to %s", fqn(reflect.TypeOf(s)), fqn(to))
	}

	vals := make([]reflect.Value, len(s))

	for i, v := range s {
		vv := reflect.ValueOf(v)
		if !vv.Type().AssignableTo(to.Elem()) {
			return nil, fmt.Errorf("type %s is not assignable to %s", fqn(vv.Type()), fqn(to.Elem()))
		}
		vals[i] = vv
	}

	return reflect.Append(reflect.New(to).Elem(), vals...).Interface(), nil
}
