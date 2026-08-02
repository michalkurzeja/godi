package di

import (
	"errors"

	"github.com/michalkurzeja/godi/v2/di"
)

// New creates a new Builder.
// This is the recommended entrypoint to the godi library.
func New(opts ...BuilderOption) *Builder {
	conf := newConfig(opts)
	return &Builder{cb: di.NewContainerBuilder(conf), defaults: conf.Defaults}
}

// Builder is a helper for building a container.
// It offers a fluent interface that incorporates other helpers to make
// the process of setting up the container easy and convenient for the user.
// This is the recommended way of building a container.
type Builder struct {
	cb *di.ContainerBuilder

	services  []*ServiceDefinitionBuilder
	functions []*FunctionDefinitionBuilder
	bindings  []*InterfaceBindingBuilder
	passes    []*di.CompilerPass

	// defaults are what a definition takes for the properties its registration
	// did not set.
	defaults di.Defaults

	// prepared counts how many of each have been handed to the container builder
	// already, so that registering more after a prepare still builds the lot.
	prepared struct{ services, functions, bindings, passes int }
	prepErr  error
}

func (b *Builder) Services(services ...*ServiceDefinitionBuilder) *Builder {
	b.services = append(b.services, services...)
	return b
}

func (b *Builder) Functions(functions ...*FunctionDefinitionBuilder) *Builder {
	b.functions = append(b.functions, functions...)
	return b
}

func (b *Builder) Bindings(bindings ...*InterfaceBindingBuilder) *Builder {
	b.bindings = append(b.bindings, bindings...)
	return b
}

func (b *Builder) CompilerPasses(passes ...*di.CompilerPass) *Builder {
	b.passes = append(b.passes, passes...)
	return b
}

func (b *Builder) Build() (Container, error) {
	prepErr := b.prepare()
	b.reportPrepErr(prepErr)

	container, err := b.cb.Build()
	err = errors.Join(prepErr, err)
	if err != nil {
		b.reportFailedBuild(container)
	}
	return container, err
}

// reportPrepErr puts what went wrong before compilation into the container, so
// that the graph of a failed build shows it. A definition that would not parse
// never became one, so the container is the only thing left to attach it to.
//
// Each error is reported on its own. They arrive joined, and one line per fault
// is what a reader wants.
func (b *Builder) reportPrepErr(err error) {
	if err == nil {
		return
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			b.reportPrepErr(e)
		}
		return
	}
	b.cb.Report(di.Diagnostic{Severity: di.SeverityError, Site: di.AtContainer(), Message: err.Error(), Err: err})
}

// prepare hands everything registered since last time to the container builder:
// the definition builders as definitions, and the compiler passes to the
// compiler. It collects every error rather than stopping at the first.
//
// Each builder is prepared exactly once. The definitions it produces are the
// same objects Build compiles, so preparing one twice would fill its arguments
// twice.
//
// The compiler passes go in here too. A prepared builder still missing them
// would compile into a different container than the one it describes.
func (b *Builder) prepare() error {
	services := b.services[b.prepared.services:]
	b.prepared.services = len(b.services)

	for _, builder := range services {
		builder.applyDefaults(b.defaults)
		if err := builder.ParseFactory(); err != nil {
			b.prepErr = errors.Join(b.prepErr, err)
			continue
		}
	}

	for _, builder := range services {
		if err := builder.Build(b.cb.RootScope()); err != nil {
			b.prepErr = errors.Join(b.prepErr, err)
			continue
		}
	}

	for _, builder := range b.functions[b.prepared.functions:] {
		builder.applyDefaults(b.defaults)
		if err := builder.Build(b.cb.RootScope()); err != nil {
			b.prepErr = errors.Join(b.prepErr, err)
			continue
		}
	}
	b.prepared.functions = len(b.functions)

	for _, builder := range b.bindings[b.prepared.bindings:] {
		if err := builder.Build(b.cb.RootScope()); err != nil {
			b.prepErr = errors.Join(b.prepErr, err)
			continue
		}
	}
	b.prepared.bindings = len(b.bindings)

	for _, pass := range b.passes[b.prepared.passes:] {
		b.cb.Compiler().AddPass(pass)
	}
	b.prepared.passes = len(b.passes)

	return b.prepErr
}

func newConfig(opts []BuilderOption) di.Config {
	conf := di.NewConfig()
	for _, opt := range opts {
		opt(&conf)
	}
	return conf
}

type BuilderOption func(*di.Config)

func SkipCycleValidation() BuilderOption {
	return func(b *di.Config) {
		b.SkipCycleValidation = true
	}
}
