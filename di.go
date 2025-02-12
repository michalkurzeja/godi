package di

import (
	"fmt"
	"io"
	"reflect"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/internal/util"
)

type Container interface {
	HasService(id di.ID) bool
	GetService(id di.ID) (any, error)
	GetServices(ids ...di.ID) (svcs []any, err error)
	GetServicesIDsByType(typ reflect.Type) []ID
	GetServicesByType(typ reflect.Type) ([]any, error)
	GetServicesIDsByLabel(label Label) []ID
	GetServicesByLabel(label di.Label) ([]any, error)
	HasFunction(id di.ID) bool
	ExecuteFunction(id di.ID) ([]any, error)
	ExecuteFunctions(ids ...di.ID) (results [][]any, err error)
	GetFunctionsIDsByType(typ reflect.Type) []ID
	ExecuteFunctionsByType(typ reflect.Type) ([][]any, error)
	GetFunctionsIDsByLabel(label Label) []ID
	ExecuteFunctionsByLabel(label di.Label) ([][]any, error)
	Print(w io.Writer)
}

// SvcByRef returns a service from the container by its reference.
func SvcByRef[T any](c Container, ref SvcReference) (T, error) {
	if ref.IsEmpty() {
		return util.Zero[T](), fmt.Errorf("service not found: empty reference")
	}
	svc, err := c.GetService(ref.SvcID())
	if err != nil {
		return util.Zero[T](), err
	}
	if svc == nil {
		return util.Zero[T](), fmt.Errorf("service %s not found", ref)
	}
	return castTo[T](svc)
}

// SvcByType returns a service from the container by its type.
func SvcByType[T any](c Container) (T, error) {
	typ := reflect.TypeFor[T]()

	svcs, err := c.GetServicesByType(typ)
	if err != nil {
		return util.Zero[T](), err
	}

	if len(svcs) == 0 {
		return util.Zero[T](), fmt.Errorf("service of type %s not found", util.Signature(typ))
	}
	if len(svcs) > 1 {
		return util.Zero[T](), fmt.Errorf("found multiple services of type %s", util.Signature(typ))
	}

	return castTo[T](svcs[0])
}

// SvcsByType returns all services from the container by their type.
func SvcsByType[T any](c Container) ([]T, error) {
	typ := reflect.TypeFor[T]()

	svcs, err := c.GetServicesByType(typ)
	if err != nil {
		return nil, err
	}

	return castSliceTo[T](svcs)
}

// SvcByLabel returns a service from the container by its label.
func SvcByLabel[T any](c Container, label Label) (T, error) {
	svcs, err := c.GetServicesByLabel(label)
	if err != nil {
		return util.Zero[T](), err
	}

	if len(svcs) == 0 {
		return util.Zero[T](), fmt.Errorf("service with label %s not found", label)
	}
	if len(svcs) > 1 {
		return util.Zero[T](), fmt.Errorf("found multiple services with label %s", label)
	}

	return castTo[T](svcs[0])
}

// SvcsByLabel returns all services from the container by their label.
func SvcsByLabel[T any](c Container, label Label) ([]T, error) {
	ids, err := c.GetServicesByLabel(label)
	if err != nil {
		return nil, err
	}

	return castSliceTo[T](ids)
}

// ExecByRef executes a function by its reference.
func ExecByRef(c Container, ref FuncReference) ([]any, error) {
	if ref.IsEmpty() {
		return nil, fmt.Errorf("function not found: empty reference")
	}
	return c.ExecuteFunction(ref.FuncID())
}

// ExecByType executes a function by its type.
func ExecByType[T any](c Container) ([]any, error) {
	typ := reflect.TypeFor[T]()

	ids := c.GetFunctionsIDsByType(typ)
	if len(ids) == 0 {
		return nil, fmt.Errorf("function of type %s not found", util.Signature(typ))
	}
	if len(ids) > 1 {
		return nil, fmt.Errorf("found multiple functions of type %s", util.Signature(typ))
	}

	return c.ExecuteFunction(ids[0])
}

// ExecAllByType executes all function by their type.
func ExecAllByType[T any](c Container) ([][]any, error) {
	return c.ExecuteFunctionsByType(reflect.TypeFor[T]())
}

// ExecByLabel executes a function by its label.
func ExecByLabel(c Container, label Label) ([]any, error) {
	ids := c.GetFunctionsIDsByLabel(label)
	if len(ids) == 0 {
		return nil, fmt.Errorf("function with label %s not found", label)
	}
	if len(ids) > 1 {
		return nil, fmt.Errorf("found multiple functions with label %s", label)
	}

	return c.ExecuteFunction(ids[0])
}

// ExecAllByLabel executes all function by their type.
func ExecAllByLabel(c Container, label Label) ([][]any, error) {
	return c.ExecuteFunctionsByLabel(label)
}

func castSliceTo[T any](svcsAny []any) ([]T, error) {
	svcs := make([]T, 0, len(svcsAny))
	for _, svcAny := range svcsAny {
		svc, ok := svcAny.(T)
		if !ok {
			return nil, fmt.Errorf(`di: service is of wrong type; expected %s; got %s`, util.Signature(reflect.TypeFor[T]()), util.Signature(reflect.TypeOf(svcAny)))
		}
		svcs = append(svcs, svc)
	}
	return svcs, nil
}

func castTo[T any](svcAny any) (T, error) {
	svc, ok := svcAny.(T)
	if !ok {
		return util.Zero[T](), fmt.Errorf(`di: service is of wrong type; expected %s; got %s`, util.Signature(reflect.TypeFor[T]()), util.Signature(reflect.TypeOf(svcAny)))
	}

	return svc, nil
}
