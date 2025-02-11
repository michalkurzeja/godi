package di

import (
	"fmt"
	"reflect"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

type InterfaceBinding struct {
	ifaceTyp reflect.Type
	boundTo  Arg
}

func NewInterfaceBinding(iface reflect.Type, boundTo Arg) (*InterfaceBinding, error) {
	if iface.Kind() != reflect.Interface {
		return nil, fmt.Errorf("invalid binding: %s is not an interface", util.Signature(iface))
	}
	if !boundTo.Type().Implements(iface) {
		return nil, fmt.Errorf("invalid binding: %s does not implement %s", util.Signature(boundTo.Type()), util.Signature(iface))
	}
	return &InterfaceBinding{ifaceTyp: iface, boundTo: boundTo}, nil
}

func (b *InterfaceBinding) Interface() reflect.Type {
	return b.ifaceTyp
}

func (b *InterfaceBinding) BoundTo() Arg {
	return b.boundTo
}
