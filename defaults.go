package di

import (
	"github.com/michalkurzeja/godi/v2/di"
)

// What a service or function takes for a property its own registration did not
// choose. Pass them to New:
//
//	c, err := di.New(di.DefaultEager()).Services(...).Build()
//
// They belong to the container they are given to, so two containers in one
// process can disagree.

// DefaultLazy builds a service the first time something asks for it. The
// default.
func DefaultLazy() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Lazy = true }
}

// DefaultEager builds every service as the container is built.
func DefaultEager() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Lazy = false }
}

// DefaultShared hands the same instance to everything that asks. The default.
func DefaultShared() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Shared = true }
}

// DefaultNotShared builds a new instance for each request.
func DefaultNotShared() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Shared = false }
}

// DefaultAutowired fills in the arguments nobody wrote, by type. The default.
func DefaultAutowired() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Autowired = true }
}

// DefaultNotAutowired leaves every argument to whoever registered the service.
func DefaultNotAutowired() BuilderOption {
	return func(conf *di.Config) { conf.Defaults.Autowired = false }
}

// SetDefaultLazy sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultLazy()), which is per container. A
// package-level default means two containers in one process cannot disagree, a
// library that sets one changes its host's containers, and a test that flips
// one leaks into the next.
func SetDefaultLazy() {
	di.SetDefaultLazy(true) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}

// SetDefaultEager sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultEager()), which is per container.
func SetDefaultEager() {
	di.SetDefaultLazy(false) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}

// SetDefaultShared sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultShared()), which is per container.
func SetDefaultShared() {
	di.SetDefaultShared(true) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}

// SetDefaultNotShared sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultNotShared()), which is per container.
func SetDefaultNotShared() {
	di.SetDefaultShared(false) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}

// SetDefaultAutowired sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultAutowired()), which is per container.
func SetDefaultAutowired() {
	di.SetDefaultAutowired(true) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}

// SetDefaultNotAutowired sets the default for every container in the process.
//
// Deprecated: use di.New(di.DefaultNotAutowired()), which is per container.
func SetDefaultNotAutowired() {
	di.SetDefaultAutowired(false) //nolint:staticcheck // SA1019: the deprecated call is the whole of this deprecated function.
}
