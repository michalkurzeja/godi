// Command graph wires a small, deliberately varied container and writes its
// dependency graph as Graphviz DOT.
//
// The wiring is chosen so that every kind of provenance the encoder can draw
// shows up somewhere in the picture: an argument you wrote by hand, one godi
// autowired, one resolved through a binding godi created for you, one resolved
// through a binding you declared, and one substituted by a compiler pass.
//
// Render it with Graphviz:
//
//	go run ./examples/graph | dot -Tsvg -o graph.svg
//	go run ./examples/graph -theme dark | dot -Tsvg -o graph-dark.svg
//
// Open the SVG in a browser to pan, zoom, and read the tooltips: every node
// carries its fully qualified type, factory and scope, and every edge carries
// how it resolved.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"time"

	di "github.com/michalkurzeja/godi/v2"
	dicore "github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/extras"
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/dot"
)

// An interface with a single implementation: godi binds it on its own, and the
// edges into it get a diamond arrowhead.

type Logger interface{ Log(string) }

type ConsoleLogger struct{}

func (ConsoleLogger) Log(string) {}

// An interface with two implementations: godi cannot choose, so the container
// declares a binding. Those edges get a hollow arrowhead, and the losing
// implementation shows up as unreachable.

type Clock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type FrozenClock struct{}

func (FrozenClock) Now() time.Time { return time.Time{} }

// An interface collected into a slice: every implementation is injected.

type Handler interface{ Handle() }

type UserHandler struct{}

func (UserHandler) Handle() {}

type OrderHandler struct{}

func (OrderHandler) Handle() {}

// An interface bound by a compiler pass rather than by godi or by the container.
// Unusual - most containers never do this - but the graph distinguishes it, so
// the example covers it.

type Reporter interface{ Report() }

type JSONReporter struct{}

func (JSONReporter) Report() {}

type TextReporter struct{}

func (TextReporter) Report() {}

type (
	Config    struct{}
	Repo      struct{}
	Cache     struct{}
	Metrics   struct{}
	Router    struct{}
	Server    struct{}
	Conn      struct{}
	Scheduler struct{}
	Auditor   struct{}
)

func NewConsoleLogger() ConsoleLogger { return ConsoleLogger{} }
func NewSystemClock() SystemClock     { return SystemClock{} }
func NewFrozenClock() FrozenClock     { return FrozenClock{} }
func NewJSONReporter() JSONReporter   { return JSONReporter{} }
func NewTextReporter() TextReporter   { return TextReporter{} }
func NewConfig(string) *Config        { return &Config{} }
func NewMetrics() *Metrics            { return &Metrics{} }
func NewConn() *Conn                  { return &Conn{} }

func NewScheduler(Clock) *Scheduler { return &Scheduler{} }
func NewAuditor(Reporter) *Auditor  { return &Auditor{} }

func NewRepo(Logger, *Config, string) *Repo { return &Repo{} }
func NewCache(Clock, time.Duration) *Cache  { return &Cache{} }

func NewUserHandler(*Repo, *Cache) UserHandler { return UserHandler{} }
func NewOrderHandler(*Repo) OrderHandler       { return OrderHandler{} }

func NewRouter(...Handler) *Router { return &Router{} }

func NewServer(*Router, Logger, *Conn, string) *Server { return &Server{} }

func (s *Server) SetTimeout(time.Duration) {}

// A function is an entry point, so everything it needs counts as reachable.
func migrate(*Repo, Logger, *Scheduler, *Auditor) error { return nil }

func main() {
	theme := flag.String("theme", "light", "colour palette: light or dark")
	flag.Parse()

	if err := run(*theme); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(theme string) error {
	c, err := build()
	if err != nil {
		return err
	}

	// Literal values are left out by default, because a constant is often a
	// connection string or a token. This example opts in, truncating to 28
	// characters, so that the fake DSN below shows up in the picture.
	g, err := graph.Extract(c, graph.WithLiteralValues(28))
	if err != nil {
		return err
	}

	palette := dot.Light
	if theme == "dark" {
		palette = dot.Dark
	}

	return g.Encode(os.Stdout, dot.New(dot.Theme(palette)))
}

// bindReporter is a compiler pass that declares an interface binding itself.
// godi cannot choose between the two reporters, and the container does not say,
// so this pass does - and the graph credits it by name, drawing those edges with
// a dot arrowhead.
//
// Reaching for the engine package like this is unusual; most containers only
// ever need the bindings the facade offers.
func bindReporter() *dicore.CompilerPass {
	return dicore.NewCompilerPass("bind reporter", dicore.PreAutomation, dicore.CompilerOpFunc(
		func(b *dicore.ContainerBuilder) error {
			root := b.RootScope()

			impls := root.GetServiceDefinitionsByType(reflect.TypeFor[JSONReporter]())
			if len(impls) == 0 {
				return errors.New("bind reporter: no JSON reporter registered")
			}

			ref, err := dicore.NewRefArg(impls[0])
			if err != nil {
				return err
			}
			binding, err := dicore.NewInterfaceBinding(reflect.TypeFor[Reporter](), ref)
			if err != nil {
				return err
			}

			root.AddBindings(binding)
			return nil
		},
	))
}

func build() (di.Container, error) {
	var (
		configRef    di.SvcReference
		serverRef    di.SvcReference
		schedulerRef di.SvcReference
	)

	return di.New().
		Services(
			di.Svc(NewConsoleLogger),

			// Two clocks, so the binding below has something to choose between.
			di.Svc(NewSystemClock),
			di.Svc(NewFrozenClock),

			di.Svc(NewConfig, "production").Bind(&configRef).Labels("config"),

			// The reference is explicit, so this dependency is drawn solid: you
			// wired it, godi did not work it out.
			di.Svc(NewRepo, &configRef, "postgres://user:hunter2@db:5432/app"),

			// A fresh instance on every injection, drawn with a dashed border.
			di.Svc(NewCache, 5*time.Minute).NotShared(),

			// The pass below rewires this one, so its clock is drawn bold.
			di.Svc(NewScheduler).Bind(&schedulerRef),

			// Two reporters, chosen by a compiler pass rather than here.
			di.Svc(NewJSONReporter),
			di.Svc(NewTextReporter),
			di.Svc(NewAuditor),

			di.Svc(NewUserHandler),
			di.Svc(NewOrderHandler),
			di.Svc(NewRouter),

			di.Svc(NewServer, "0.0.0.0:8080").
				Bind(&serverRef).
				Eager().
				MethodCall((*Server).SetTimeout, 30*time.Second).
				Children(di.Svc(NewConn)), // Private to the server.

			// Registered, wired to nothing, reached by nothing. Together with the
			// text reporter the binding above passes over, it is what the graph
			// flags as a candidate for dead wiring.
			di.Svc(NewMetrics),
		).
		Bindings(
			// You choose which clock. Without this the container would not build.
			di.BindType[Clock, SystemClock](),
		).
		Functions(
			di.Func(migrate).Labels("startup"),
		).
		CompilerPasses(
			// Substitutes a literal after the fact. A constant draws no edge, so
			// the pass is credited on the argument row instead.
			extras.OverrideSvcArg(serverRef, 3, "127.0.0.1:9090"),

			// Substitutes a dependency: the scheduler gets the frozen clock
			// instead of the one the binding above would have given it. That is
			// a real edge, drawn bold and credited to the pass.
			extras.OverrideSvcArg(schedulerRef, 0, di.Type[FrozenClock]()),

			bindReporter(),
		).
		Build()
}
