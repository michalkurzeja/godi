// Package dig provides a default, package-level container and functions that interact with it.
//
// It's meant to be a quick and easy way to use the di package,
// sufficient for most cases where only 1 container is needed.
//
// Why `dig`? It stands for "DI Global". Also, it's short and easy to type.
package dig

import "github.com/michalkurzeja/godi/v2/di"

var (
	b = di.New()
	c di.Container
)

func Container() di.Container {
	return c
}

func AddServices(services ...*di.DefinitionBuilder) {
	b.Services(services...)
}

func AddAliases(aliases ...di.Alias) {
	b.Aliases(aliases...)
}

func Build() (err error) {
	c, err = b.Build()
	return err
}

func Get[T any](opts ...di.OptionsFunc) (T, error) {
	return di.Get[T](c, opts...)
}

func MustGet[T any](opts ...di.OptionsFunc) T {
	return di.MustGet[T](c, opts...)
}

func GetByTag[T any](tag di.Tag) ([]T, error) {
	return di.GetByTag[T](c, tag)
}

func MustGetByTag[T any](tag di.Tag) []T {
	return di.MustGetByTag[T](c, tag)
}
