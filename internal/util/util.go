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

	named := typ
	var isPtr bool
	if typ.Kind() == reflect.Ptr {
		named = typ.Elem()
		isPtr = true
	}

	if pkgPath := named.PkgPath(); pkgPath != "" {
		name := named.Name()
		if isPtr {
			name = fmt.Sprintf("(*%s)", name)
		}
		return pkgPath + "." + name
	}

	// Report the type as given: a pointer to something unnamed is still a
	// pointer, so *int must not come back as int.
	return typ.String()
}

func FuncName(val reflect.Value) string {
	if val.Kind() != reflect.Func {
		return "<not a func>"
	}
	return runtime.FuncForPC(val.Pointer()).Name()
}

// FuncLocation is where a function is declared: the file, and the line the
// func keyword is on. Line is zero when the value is not a function, or when
// the binary carries no line table for it.
func FuncLocation(val reflect.Value) (file string, line int) {
	if val.Kind() != reflect.Func {
		return "", 0
	}

	pc := val.Pointer()
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", 0
	}
	return fn.FileLine(pc)
}

func FuncNameShort(val reflect.Value) string {
	split := strings.Split(FuncName(val), ".")
	if len(split) == 1 {
		return split[0]
	}
	return split[len(split)-1]
}

// Truncate shortens s to at most maxRunes runes, reporting whether it had to cut.
// A maxRunes of zero or less leaves s alone.
//
// It walks to the cut point rather than slicing by byte, which would split a
// multi-byte rune and leave invalid UTF-8 behind.
func Truncate(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return s, false
	}

	var n int
	for i := range s {
		if n == maxRunes {
			return s[:i], true
		}
		n++
	}
	return s, false
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
