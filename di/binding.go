package di

import (
	"fmt"
	"reflect"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// BindOrigin tells who created an InterfaceBinding. Unlike a Slot, a binding
// never exists before something creates it, so there is no "none" origin.
//
// It is the binding half of what ArgOrigin says about an argument, and a
// compiler pass reads it for the same reason.
type BindOrigin uint8

const (
	BindOriginManual       BindOrigin = iota // Declared by the user.
	BindOriginAutobinding                    // Created by the interface binding pass.
	BindOriginCompilerPass                   // Created by a Compiler pass.
)

type InterfaceBinding struct {
	ifaceTyp reflect.Type
	boundTo  Arg

	origin     BindOrigin
	originPass string // Name of the pass, for BindOriginAutobinding and BindOriginCompilerPass.
	dirty      bool   // Whether the binding was created since the Compiler last looked.
}

func NewInterfaceBinding(iface reflect.Type, boundTo Arg) (*InterfaceBinding, error) {
	if iface.Kind() != reflect.Interface {
		return nil, fmt.Errorf("invalid binding: %s is not an interface", util.Signature(iface))
	}
	if !boundTo.Type().Implements(iface) {
		return nil, fmt.Errorf("invalid binding: %s does not implement %s", util.Signature(boundTo.Type()), util.Signature(iface))
	}
	return &InterfaceBinding{ifaceTyp: iface, boundTo: boundTo, origin: BindOriginManual, dirty: true}, nil
}

func (b *InterfaceBinding) Interface() reflect.Type {
	return b.ifaceTyp
}

func (b *InterfaceBinding) BoundTo() Arg {
	return b.boundTo
}

// Origin says who created this binding, and names the pass when one did.
func (b *InterfaceBinding) Origin() (BindOrigin, string) {
	return b.origin, b.originPass
}

// creditTo names whoever created this binding.
func (b *InterfaceBinding) creditTo(origin BindOrigin, pass string) {
	b.origin = origin
	b.originPass = pass
	b.dirty = false
}
