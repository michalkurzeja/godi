# godi
![ci workflow](https://github.com/michalkurzeja/godi/actions/workflows/build.yaml/badge.svg)

This library is an attempt to bring to Go a DI container requires as little action as possible from the user.
You just need to define your services and the library will handle dependencies on its own, as much as possible.
Whenever there's any ambiguity, you'll have to resolve it yourself.

This library takes inspiration from https://github.com/samber/do and an excellent
[DI component](https://github.com/symfony/dependency-injection) of PHP Symfony framework.

## Features

- [x] Service definition
- [x] Automatic and manual dependency resolution
- [x] Cyclic dependencies through deferred injection
- [x] Lazy loading
- [x] Dependency graph validation and helpful errors
- [x] Dependency graph visualization
- [x] Test mode (dependency overrides)
- [ ] Service definitions from a config file
- [ ] Tagged services

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
	// Create a new container.
	c := di.New()

	// Register services. Dependencies are resolved automatically.
	_ = di.Register(c,
		di.SvcT[Foo](NewFoo),
		di.SvcT[Bar](NewBar),
		// We need to manually provide the value of `Baz.param` because it's not in the container.
		di.SvcT[Baz](NewBaz).With(
			di.Val("my-string"),
		),
	)

	// We can now get the services from the container!
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
	// Register services right away. The container is already there!
	_ = dig.Register(
		di.SvcT[Foo](NewFoo),
	)

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
	// Register services right away. The container is already there!
	_ = dig.Register(
		di.Svc(NewFoo),
	)

	foo := dig.MustGet[Foo]()
}
```
