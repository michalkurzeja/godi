// Package dig provides a default, package-level container and functions that interact with it.
//
// It's meant to be a quick and easy way to use the di package,
// sufficient for most cases where only 1 container is needed.
//
// Why `dig`? It stands for "DI Global". Also, it's short and easy to type.
package dig

import (
	"io"

	di "github.com/michalkurzeja/godi"
)

var c = di.New()

func Container() di.Container {
	return roContainer{c: c}
}

func Test() *di.TestContainer {
	tc := di.NewTestContainer()
	c = tc
	return tc
}

func Register(services ...*di.ServiceBuilder) error {
	return di.Register(c, services...)
}

func Get[T any]() (T, error) {
	return di.Get[T](c)
}

func MustGet[T any]() T {
	return di.MustGet[T](c)
}

func Export(w io.Writer) error {
	return di.Export(c, w)
}
