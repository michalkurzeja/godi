package di

import (
	"reflect"
	"sort"

	"golang.org/x/exp/constraints"
)

// FQN returns the fully qualified name of the type parameter.
func FQN[T any]() ID {
	return fqn(typeOf[T]())
}

// fqn returns the fully qualified name of a type.
// It's a reflection-based, internal implementation.
func fqn(typ reflect.Type) ID {
	if typ == nil {
		return "<nil>"
	}
	if typ.Kind() == reflect.Ptr {
		return "*" + fqn(typ.Elem())
	}
	if pkgPath := typ.PkgPath(); pkgPath != "" {
		return ID(pkgPath + "." + typ.Name())
	}
	return ID(typ.String())
}

// zero returns the zero value of the type parameter.
func zero[T any]() (v T) {
	return
}

// typeOf returns the reflect.Type of the type parameter.
func typeOf[T any]() reflect.Type {
	typ := reflect.TypeOf(zero[T]())
	if typ == nil { // This happens when T is an interface.
		typ = reflect.TypeOf((*T)(nil)).Elem()
	}
	return typ
}

// sorted returns the given slice, sorted by the given property.
func sorted[T any, O constraints.Ordered](s []T, by func(v T) O) []T {
	sort.Slice(s, func(i, j int) bool {
		return by(s[i]) < by(s[j])
	})
	return s
}

// convert returns the given value converted to the given type.
// In the case of scalar values, it doesn't do any conversion.
// In the case of a slice, it ensures that is element is of the requested type.
// Used to convert []any to []T.
func convert(v reflect.Value, to reflect.Type) (reflect.Value, bool) {
	if to.Kind() != reflect.Slice {
		return v, true
	}

	sl := reflect.New(to).Elem()
	for i := 0; i < v.Len(); i++ {
		el := v.Index(i)
		if el.Type().Kind() == reflect.Interface {
			el = el.Elem()
		}
		sl = reflect.Append(sl, el)
	}

	return sl, true
}
