package di

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

// Get returns a service from the container and ensures its type.
func Get[T any](c Container, opts ...OptionsFunc) (T, error) {
	opt := getOptions{id: FQN[T]()}
	for _, optFn := range opts {
		optFn(&opt)
	}

	svcAny, err := c.Get(opt.id)
	if err != nil {
		return zero[T](), err
	}

	svc, ok := svcAny.(T)
	if !ok {
		return zero[T](), fmt.Errorf(`di: service %s is of wrong type; expected %s; got %s`, opt.id, FQN[T](), fqn(reflect.TypeOf(svcAny)))
	}

	return svc, nil
}

// GetByTag returns all services from the container that have the given tag.
func GetByTag[T any](c Container, tag Tag) ([]T, error) {
	svcsAny, err := c.GetByTag(tag)
	if err != nil {
		return nil, err
	}

	var errs *multierror.Error
	svcs := lo.Map(svcsAny, func(svcAny any, _ int) T {
		svc, ok := svcAny.(T)
		if !ok {
			errs = multierror.Append(errs, fmt.Errorf(`di: service %s is of wrong type; expected %s; got %s`, tag, FQN[T](), fqn(reflect.TypeOf(svcAny))))
			return zero[T]()
		}
		return svc
	})

	if errs != nil {
		return nil, errs
	}
	return svcs, nil
}

// Has returns true if the container has a service with the given ID.
func Has[T any](c Container, opts ...OptionsFunc) bool {
	opt := getOptions{id: FQN[T]()}
	for _, optFn := range opts {
		optFn(&opt)
	}

	return c.Has(opt.id)
}

// Initialised returns true if the service has been initialised, i.e. if the containes
// currently holds the instance of that service.
func Initialised[T any](c Container, opts ...OptionsFunc) bool {
	opt := getOptions{id: FQN[T]()}
	for _, optFn := range opts {
		optFn(&opt)
	}

	return c.Initialised(opt.id)
}

// MustGet is like Get but panics if an error occurs.
func MustGet[T any](c Container, opts ...OptionsFunc) T {
	svc, err := Get[T](c, opts...)
	if err != nil {
		panic(err)
	}
	return svc
}

// MustGetByTag is like GetByTag but panics if an error occurs.
func MustGetByTag[T any](c Container, tag Tag) []T {
	svcs, err := GetByTag[T](c, tag)
	if err != nil {
		panic(err)
	}
	return svcs
}

type getOptions struct {
	id ID
}

type OptionsFunc func(opt *getOptions)

// WithID returns an OptionsFunc that sets the ID of the service to get.
func WithID(id ID) OptionsFunc {
	return func(opt *getOptions) {
		opt.id = id
	}
}
