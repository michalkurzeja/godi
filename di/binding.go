package di

import (
	"fmt"
	"reflect"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// bindOrigin tells who created an InterfaceBinding. Unlike a Slot, a binding
// never exists before something creates it, so there is no "none" origin.
type bindOrigin uint8

const (
	bindOriginManual       bindOrigin = iota // Declared by the user.
	bindOriginAutobinding                    // Created by the interface binding pass.
	bindOriginCompilerPass                   // Created by a Compiler pass.
)

type InterfaceBinding struct {
	ifaceTyp reflect.Type
	boundTo  Arg

	origin     bindOrigin
	originPass string // Name of the pass, for bindOriginAutobinding and bindOriginCompilerPass.
	dirty      bool   // Whether the binding was created since the Compiler last looked.
}

func NewInterfaceBinding(iface reflect.Type, boundTo Arg) (*InterfaceBinding, error) {
	if iface.Kind() != reflect.Interface {
		return nil, fmt.Errorf("invalid binding: %s is not an interface", util.Signature(iface))
	}
	if !boundTo.Type().Implements(iface) {
		return nil, fmt.Errorf("invalid binding: %s does not implement %s", util.Signature(boundTo.Type()), util.Signature(iface))
	}
	return &InterfaceBinding{ifaceTyp: iface, boundTo: boundTo, origin: bindOriginManual, dirty: true}, nil
}

func (b *InterfaceBinding) Interface() reflect.Type {
	return b.ifaceTyp
}

func (b *InterfaceBinding) BoundTo() Arg {
	return b.boundTo
}

// creditTo names whoever created this binding.
func (b *InterfaceBinding) creditTo(origin bindOrigin, pass string) {
	b.origin = origin
	b.originPass = pass
	b.dirty = false
}
