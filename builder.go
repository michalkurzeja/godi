package di

import (
	"errors"

	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// New creates a new Builder.
// This is the recommended entrypoint to the godi library.
func New(opts ...BuilderOption) *Builder {
	return &Builder{cb: di.NewContainerBuilder(newConfig(opts))}
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

	// prepared counts how many of each have been turned into definitions
	// already, so that reading the graph and then registering more still
	// builds the lot.
	prepared struct{ services, functions, bindings int }
	prepErr  error
}

// A Builder is a graph.Source too, so the wiring can be read before it is
// compiled.
var _ graph.Source = (*Builder)(nil)

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

	for _, pass := range b.passes {
		b.cb.Compiler().AddPass(pass)
	}

	container, err := b.cb.Build()
	err = errors.Join(prepErr, err)
	if err != nil {
		b.reportFailedBuild(container)
	}
	return container, err
}

// Graph returns the dependency graph of the container as it is configured so
// far, before any of it is compiled: arguments you wrote are there, and the
// ones godi would autowire are not yet. The graph says so - see graph.Snapshot.
//
// It is how you see what you have declared without building it, and the
// counterpart to taking a graph from a compiler pass, which shows a later
// moment. Building afterwards is unaffected.
func (b *Builder) Graph(cfg graph.Config) *graph.Graph {
	_ = b.prepare() // Whatever failed to build is missing from the graph, and Build reports it.
	return b.cb.Graph(cfg)
}

// prepare turns the definition builders registered since last time into
// definitions in the container builder, collecting everything that went wrong
// rather than stopping at the first.
//
// Each builder is prepared exactly once: the definitions it produces are the
// same objects Build compiles, so preparing one twice would fill its arguments
// twice. Anything registered after a graph was read is still waiting here.
func (b *Builder) prepare() error {
	services := b.services[b.prepared.services:]
	b.prepared.services = len(b.services)

	for _, builder := range services {
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
