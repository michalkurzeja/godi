# godi ![ci workflow](https://github.com/michalkurzeja/godi/actions/workflows/build.yaml/badge.svg)

<img src="assets/logo.png" height="150" align="right"/>

Godi brings to Go a robust, feature-rich DI container that takes care for you of all the manual and boring work of combining your application components together.

You can focus on writing your business logic and let Godi handle the rest!

## ⚡ Requirements

- Go 1.23+

## 🔧 Installation

```shell
go get github.com/michalkurzeja/godi/v2
```

## 📋 Key concepts

### Service

Any value managed by the container is called a service.
It can be anything: a primitive value, a struct, a function, an interface, etc.

Services are usually long-lived: they are created once and reused multiple times in the lifespan of your app.

Typically, most services are structs with methods that implement business logic of your application.

An example of a service could be a database connection, a repository, or a logger.

### Service definition

A service definition is a configuration object that tells the container about a service.
You can think of it as a recipe for creating a service.

The majority of your interactions with this library will be through service definitions - your job, as a user, is to create and configure them.

Don't worry, usually creating a service definition is a one-liner, and odi will take care of the rest!
The simplest service definition consists of just a factory function.

### Factory

A factory is a function that creates a service - the closest thing to a constructor in Go.

Godi places only a single restriction on factories: they must have a single return value, which is the service they create, and an optional error.
Aside from that, any function will work.
It can have any number of arguments of any types and even works with variadic arguments.

When you request a service from the container, Godi will call its factory function to instantiate it.

Here are some examples of factories:

```go
// The following functions are valid factories:
func NewService() *Service
func NewService() (*Service, error)
func NewService(Arg, AnotherArg) (Service, error)
func NewService(Arg, AnotherArg, ...VariadicArg) (Service, error)
// The following functions are invalid factories:
func NewService() (Service, SomethingElse) // The second return value must be an error!
func NewService() (Service, SomethingElse, error) // Too many return values! 
```

> 💡 Godi also supports other types of functions: methods and "loose" functions.
> While there are differences in their purpose and restrictions,
> they are all handled in a similar way.

### Dependency

Simply put, a dependency is an argument of a function.
It can be another service, or a literal value provided by you.

For example, let's say you have a repository service that requires a database connection (it's an argument of the repository's factory).
That database connection is a dependency of the repository service.
The connection itself also has a factory, and it requires connection parameters, such as a hostname and credentials.
Those parameters are dependencies too.

### Container

The container is the central piece of Godi.
It's a read-only registry of services and their definitions,
responsible for creating and caching services with their dependency trees.

It is created by the container builder - a fluent API that you use to define all services.
The build process performs some automated tasks and validation to ensure that the configuration is complete and valid.

Any issues are collected and returned to the user as clear error messages to help you fix them.
If the container is built without errors, you can be certain that every single service **can** be instantiated and that no dependencies are missing.

By default, the container lazily instantiates services when you request them.
This will also recursively instantiate all their dependencies. Services are also cached and re-used.

You can change the defaults globally or per-service.

### Autowiring

Godi can automatically resolve dependencies for you. This is called autowiring.

When you define a service, godi inspects the factory function to determine the service's type, as well as all its dependencies.
Then it's able to match the dependencies with other services in the container by their types.
It's smart enough to also find matching interface implementations!

> 💡 If godi is unable to unambiguously resolve a dependency (e.g. when there are multiple services of a matching type),
> it returns a clear error and leaves it to you to resolve the situation.
>
> This way, it avoids making any opinionated decisions and keeps you in control.

## 🚀 Quick start

Here's a simple example to get you started:

```go
package main

import (
	di "github.com/michalkurzeja/godi/v2"
)

func main() {
	c, err := di.New().
		Services(
			di.Svc(NewMySvc),                       // The dependency will be autowired!
			di.Svc(NewMyOtherSvc, "Hello, world!"), // Here, we provide the dependency (string) manually.
		).Build()
	if err != nil {
		panic(err)
	}

	mySvc, err := di.SvcByType[*MySvc](c)
	if err != nil {
		// Either the container encountered an error
		// (e.g. the service of this type does not exist)
		// of the factory returned an error.
		panic(err)
	}
}

// Some dummy service implementations to demonstrate the concept

type MySvc struct {
	other *MyOtherSvc
}

func NewMySvc(other *MyOtherSvc) (*MySvc, error) {
	return &MySvc{other: other}, nil
}

type MyOtherSvc struct {
	s string
}

func NewMyOtherSvc(s string) *MyOtherSvc {
	return &MyOtherSvc{s: s}
}

```