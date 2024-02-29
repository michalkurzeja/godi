# godi

![ci workflow](https://github.com/michalkurzeja/godi/actions/workflows/build.yaml/badge.svg)

This library is an attempt to bring to Go a DI container that requires as little action as possible from the user.
You just need to define your services and the library will handle dependencies on its own, as much as possible.
Whenever there's any ambiguity, you'll get a clear error message and will have to resolve it yourself.

This library takes heavy inspiration from an excellent [DI component](https://github.com/symfony/dependency-injection) of PHP
Symfony framework.

## Features

- [x] Service definition
- [x] Automatic and manual dependency resolution
- [x] Post-construct method invocation
- [x] Service aliases
- [x] Lazy/Eager instantiation
- [x] Cached/uncached services
- [x] Public/private services
- [x] Service tagging
- [x] Dependency graph validation and helpful errors
- [x] Programmatic control and container automation through compiler passes
- [ ] Dependency graph visualization
- [ ] Test mode (dependency overrides)
- [ ] Service definitions from a config file

## Installation

```bash
go get -u github.com/michalkurzeja/godi
```

## Usage

Let's define some types that we'll want to wire using our DI container.

```go
package main

type Foo struct{}

func NewFoo() Foo { return Foo{} }

type Bar struct {
	Foo Foo
}

func NewBar(foo Foo) Bar { return Bar{Foo: foo} }

type Baz struct {
	param string
}

func NewBaz(param string) Baz { return Baz{param: param} }

```

Now we can set up the container:

```go
package main

import di "github.com/michalkurzeja/godi"

func main() {
	// Create a new container builder.
	builder := di.New()

	// Add services. Dependencies are resolved automatically.
	c, err = builder.Services(
		di.SvcT[Foo](NewFoo),
		di.SvcT[Bar](NewBar),
		// We need to manually provide the value of `Baz.param` because it's not in the container.
		di.SvcT[Baz](NewBaz).
		    Args(di.Val("my-string")),
	).Build()
	_ = err // If something is wrong, we will find out here!

	// We can now get our services from the container!
	foo := di.MustGet[Foo](c)
	bar := di.MustGet[Bar](c)
	baz := di.MustGet[Baz](c)
}

```

### Global container

If you only need a single instance of container, you can use the package `dig` instead,
which operates on a package-level container:

```go
package main

import "github.com/michalkurzeja/godi/dig"

func main() {
	// Add services right away. The container is already there!
	_ = dig.AddServices(
		di.SvcT[Foo](NewFoo),
	).Build()

	foo := dig.MustGet[Foo]()
}
```

### Inferred types

If you don't want to specify the types of your services, you can use the `di.Svc` function.
It will figure out the types of your services based on the type returned by the given provider function.

```go
package main

import "github.com/michalkurzeja/godi/dig"

func main() {
	_ = dig.AddServices(
		di.Svc(NewFoo),
	).Build()

	foo := dig.MustGet[Foo]()
}
```
