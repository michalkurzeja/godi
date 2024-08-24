package godi

import (
	"reflect"

	"github.com/michalkurzeja/godi/v2/di"
)

// ArgBuilder is a helper for building arguments.
// It is used by the DefinitionBuilder to define arguments for a service factory and method calls.
type ArgBuilder struct {
	newArg  func() (di.Arg, error)
	slot    uint
	slotSet bool
}

func Arg(v any) *ArgBuilder {
	if builder, ok := v.(*ArgBuilder); ok {
		return builder
	}
	return Val(v)
}

// Slot sets the slot of the argument.
// E.g. Slot(1) will set the argument as the second argument of a function.
func (b *ArgBuilder) Slot(n uint) *ArgBuilder {
	b.slot = n
	b.slotSet = true
	return b
}

func (b *ArgBuilder) Build() (di.Arg, error) {
	arg, err := b.newArg()
	if err != nil {
		return nil, err
	}
	if b.slotSet {
		return di.NewSlottedArg(arg, b.slot), nil
	}
	return arg, nil
}

func Ref(ref *SvcReference) *ArgBuilder {
	return &ArgBuilder{newArg: func() (di.Arg, error) {
		return di.NewRefArg(ref.def), nil
	}}
}

// Val returns an argument build for a literal value.
func Val(v any) *ArgBuilder {
	return &ArgBuilder{newArg: func() (di.Arg, error) {
		return di.NewLiteralArg(v), nil
	}}
}

// Type returns an argument build for a typed reference.
func Type[T any](label ...Label) *ArgBuilder {
	if len(label) > 0 {
		return &ArgBuilder{newArg: func() (di.Arg, error) {
			return di.NewLabelArg(label[len(label)-1], reflect.TypeFor[T](), false), nil
		}}
	}
	return &ArgBuilder{newArg: func() (di.Arg, error) {
		return di.NewTypeArg(reflect.TypeFor[T](), false), nil
	}}
}

// SliceOf returns an argument build for a typed reference to a slice.
func SliceOf[T any](label ...Label) *ArgBuilder {
	if len(label) > 0 {
		return &ArgBuilder{newArg: func() (di.Arg, error) {
			return di.NewLabelArg(label[len(label)-1], reflect.TypeFor[T](), true), nil
		}}
	}
	return &ArgBuilder{newArg: func() (di.Arg, error) {
		return di.NewTypeArg(reflect.TypeFor[T](), true), nil
	}}
}

func Compound[T any](builders ...*ArgBuilder) *ArgBuilder {
	return &ArgBuilder{newArg: func() (di.Arg, error) {
		args := make([]di.Arg, 0, len(builders))
		for _, builder := range builders {
			arg, err := builder.Build()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		return di.NewCompoundArg(reflect.TypeFor[T](), args...)
	}}
}
