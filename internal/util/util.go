package util

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/exp/constraints"
)

// Signature returns the fully qualified name of a type.
// In case of a function, it returns its argument and return values list.
func Signature(typ reflect.Type) string {
	if typ == nil {
		return "<nil>"
	}
	//if typ.Kind() == reflect.Func {
	//	return funcSignature(typ)
	//}

	var isPtr bool
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		isPtr = true
	}

	if pkgPath := typ.PkgPath(); pkgPath != "" {
		name := typ.Name()
		if isPtr {
			name = fmt.Sprintf("(*%s)", name)
		}
		return pkgPath + "." + name
	}

	return typ.String()
}

func FuncName(val reflect.Value) string {
	if val.Kind() != reflect.Func {
		return "<not a func>"
	}
	return runtime.FuncForPC(val.Pointer()).Name()
}

func FuncNameShort(val reflect.Value) string {
	split := strings.Split(FuncName(val), ".")
	if len(split) == 1 {
		return split[0]
	}
	return split[len(split)-1]
}

func funcSignature(typ reflect.Type) string {
	var args []string
	for i := 0; i < typ.NumIn(); i++ {
		args = append(args, typ.In(i).String())
	}
	var results []string
	for i := 0; i < typ.NumOut(); i++ {
		results = append(results, Signature(typ.Out(i)))
	}

	sig := "func(" + strings.Join(args, ", ") + ")"
	if len(results) == 0 {
		return sig
	}
	if len(results) == 1 {
		return sig + " " + results[0]
	}
	return sig + " (" + strings.Join(results, ", ") + ")"
}

func Zero[T any]() T {
	var v T
	return v
}

// SortedAsc returns the given slice, sorted by the given property in ascending order.
func SortedAsc[T any, O constraints.Ordered](s []T, by func(v T) O) []T {
	sort.Slice(s, func(i, j int) bool {
		return by(s[i]) < by(s[j])
	})
	return s
}

// SortedDesc returns the given slice, sorted by the given property in descending order.
func SortedDesc[T any, O constraints.Ordered](s []T, by func(v T) O) []T {
	sort.Slice(s, func(i, j int) bool {
		return by(s[i]) > by(s[j])
	})
	return s
}
