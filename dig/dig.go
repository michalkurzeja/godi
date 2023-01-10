// Package dig provides a default, package-level container and functions that interact with it.
//
// It's meant to be a quick and easy way to use the di package,
// sufficient for most cases where only 1 container is needed.
//
// Why `dig`? It stands for "DI Global". Also, it's short and easy to type.
package dig

import (
	di "github.com/michalkurzeja/godi"
)

var (
	b = &Builder{di.New()}
	c di.Container
)

func Container() di.Container {
	return c
}

func Aliases(aliases ...di.Alias) *Builder {
	b.Aliases(aliases...)
	return b
}

func Services(services ...*di.DefinitionBuilder) *Builder {
	b.Services(services...)
	return b
}

func CompilerPass(stage di.CompilerPassStage, priority int, pass di.CompilerPass) *Builder {
	b.CompilerPass(stage, priority, pass)
	return b
}

func Build() error {
	return b.Build()
}

func Get[T any](opts ...di.OptionsFunc) (T, error) {
	return di.Get[T](c, opts...)
}

func MustGet[T any](opts ...di.OptionsFunc) T {
	return di.MustGet[T](c, opts...)
}

func GetByTag[T any](tag di.TagID) ([]T, error) {
	return di.GetByTag[T](c, tag)
}

func MustGetByTag[T any](tag di.TagID) []T {
	return di.MustGetByTag[T](c, tag)
}
