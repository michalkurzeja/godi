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
