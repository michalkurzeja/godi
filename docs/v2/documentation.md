# godi v2 — Technical Documentation

> For AI agents and for anyone extending godi. It covers what godi does, the whole of its public
> API, and the behaviour that is not obvious from the signatures. It deliberately does **not**
> document unexported internals: those change, and a document that names them goes stale without
> anyone noticing.

---

## 1. Overview

**Module:** `github.com/michalkurzeja/godi/v2`
**Go version:** 1.25
**Repository:** github.com/michalkurzeja/godi

`godi` is a reflection-based dependency-injection container for Go. You declare *what* services
exist and *how* they are created; godi works out the dependency graph, validates it at build time,
and resolves values on demand.

### Main features

- **Factory-based registration.** Any function returning one or two values (`(T)` or `(T, error)`)
  is a valid factory.
- **Autowiring.** Unresolved factory, method and function arguments are matched to registered
  services by type.
- **Interface resolution.** For an interface parameter, godi finds the implementations and either
  wires the single one or assembles a slice.
- **Child scopes.** A service can declare private services, invisible to the rest of the container.
- **Method calls.** Post-construction calls, dependency-injected like anything else. They run once
  every factory the request needed has returned.
- **Functions.** Injected callables not tied to a service.
- **Compiler passes.** A staged, priority-ordered extension mechanism that inspects and mutates
  the configuration before it is finalised.
- **Interface bindings.** Explicit mappings from an interface to an argument, for the cases where
  the choice is otherwise ambiguous.
- **Shared / not shared, lazy / eager**, per definition or per container.
- **Cycle detection** over the service dependency graph.
- **Dependency graph.** A container can be extracted as an inspectable model and encoded as text,
  Graphviz DOT, a self-contained HTML page, or JSON. Every edge records who wired the argument and
  how it resolved — which is not recoverable at runtime.

### Design goals

- Nothing to pay after `Build()` returns.
- Errors at build time, not at retrieval time (except a factory's own errors).
- Autowiring that is unambiguous and deterministic: an ambiguous choice is an error, not a guess.
- Extensibility through compiler passes.
- A thin, friendly facade over a fuller engine.

---

## 2. Architecture

### 2.1 The two packages named `di`

| Package | Role |
|---|---|
| `github.com/michalkurzeja/godi/v2` (root, `package di`) | The facade. Everything a typical user needs: fluent builders, generic accessors, the `Container` interface. |
| `github.com/michalkurzeja/godi/v2/di` (`package di`) | The engine. Scopes, definitions, compiler, argument resolution, instantiation. |

Ordinary users import only the root. Extension authors — anyone writing a compiler pass or driving
the builders directly — import the engine. Its breadth is deliberate: a low call-site count inside
godi says nothing about what an extension needs.

The root re-exports `di.ID` and `di.Label` as aliases, so everyday code never imports the engine.

### 2.2 The graph packages

```
di/                       the engine. Knows nothing about graphs.
graph/                    the model + reading and writing JSON. A leaf: stdlib only.
graph/extract             reads a container, emits the model. The only package needing both sides.
graph/dot|text|html|json  encoders, one package each.
graph/serve               an HTTP handler over a graph.Source.
<root>                    the facade. Imports di, graph and graph/extract.
cmd/godi                  the CLI. The only place in the module that can start a process.
```

A graph is a **view over** the engine, not a part of it. The model stays a leaf so that anything
consuming the *format* is free of the container: `godi view graph.json` links no DI engine to
render a file, and neither does a third-party encoder. `deps_test.go` holds this to it.

Writing JSON is the one format on the model itself, because a build that failed must be able to
write its graph without linking an encoder. `graph/json` is the same writing, as an `Encoder`.

### 2.3 Lifecycle

```
di.New(opts...) -> *Builder
  .Services(...) .Functions(...) .Bindings(...) .CompilerPasses(...)
  .Build()
     |
     1. prepare(): container defaults are applied to everything registered since last time,
        then services are parsed (factory types first, then arguments), then functions,
        then bindings, then the compiler passes are registered.
     2. ContainerBuilder.Build() -> Compiler.Run:
          passes sorted by (stage, priority, when they were added)
            PreAutomation    your passes
            Automation       interface binding, autowiring
            Validation       argument validation, cycle validation
            Finalization     eager initialisation
     3. -> di.Container
```

`prepare()` is memoised per slice, so registering more after a graph has been read still builds
the lot, and the order and number of `Services`/`Functions`/`Bindings` calls never matters — a
service may refer to one registered by a later call.

A successful `Build` hands the container over and leaves the builder spent. A failed one leaves it
standing, which is what makes the graph of where the compiler stopped available.

---

## 3. The facade

### 3.1 Building a container

```go
c, err := di.New(di.DefaultEager()).
	Services(
		di.Svc(NewServer, di.Val(":8080")).Eager(),
		di.Svc(NewRepo).Labels("storage").Children(di.Svc(NewConn)),
		di.SvcVal(time.Local),
	).
	Functions(di.Func(migrate).Labels("startup")).
	Bindings(di.BindType[Logger, *ConsoleLogger]()).
	CompilerPasses(myPass).
	Build()
```

| Call | What it does |
|---|---|
| `New(opts ...BuilderOption) *Builder` | A builder. Options: `SkipCycleValidation()`, `DefaultLazy/DefaultEager`, `DefaultShared/DefaultNotShared`, `DefaultAutowired/DefaultNotAutowired`. |
| `Svc(factory any, args ...any)` | A service built by a factory. |
| `SvcVal[T](v T)` | A service that *is* the value handed over. |
| `Func(fn any, args ...any)` | An injected callable. |
| `Builder.Build() (Container, error)` | Prepares, compiles, returns the container. |

Definition builders: `.Bind(&ref)`, `.Labels(...)`, `.MethodCall(method, args...)`,
`.Children(...)`, `.Lazy()`, `.Eager()`, `.Shared()`, `.NotShared()`, `.Autowired()`,
`.NotAutowired()`.

### 3.2 Arguments

Positional arguments to `Svc`/`Func`/`MethodCall` fill the first free slot they fit, left to
right. Anything that is not an `*ArgBuilder` or a `*SvcReference` is taken as a literal.

| Builder | Argument |
|---|---|
| `Val(v)` | A literal value. Not `nil`: an untyped nil has no type to be slotted by. |
| `Ref(&ref)` | Whatever the reference is bound to. |
| `Type[T]()` | The service of type `T`. `Type[T](label)` narrows to a label. |
| `SliceOf[T]()` | Every service of type `T`, as `[]T`. `SliceOf[T](label)` narrows to a label. |
| `Compound[T](a, b, …)` | A `[]T` assembled from the given arguments. |
| `SpreadSlice(a)` | Inside a `Compound[T]`, the elements of the `[]T` that `a` resolves to, rather than that slice as one element. |
| `Arg(v)` | Whichever of the above suits `v`. |
| `.Slot(n)` | Pins an argument to slot `n` rather than the next free one. |

`Type[T](label ...Label)` uses the **last** label given and silently ignores the rest.

**A nil literal is refused; a nil service is not.** `Val(nil)` — and `Val(iface)` where `iface` is
a nil interface, which is the same `nil` once boxed in an `any` — carries no type, so nothing says
which slot it fills. Write a typed nil, `Val((*Logger)(nil))`. A factory *returning* nil is
ordinary Go and is injected as nil: the parameter type says what the nil means, since the value
itself no longer carries one.

Every slot must be filled, by you or by autowiring, and validation reports "argument N is not
set" otherwise. A variadic slot is the exception: no arguments is a valid call, so leaving it out
passes an empty slice. That holds whether or not the definition is autowired — an autowired one
finding no services of the element type comes to the same thing.

### 3.3 Bindings

```go
di.BindType[Logger, *ConsoleLogger]()   // Logger resolves to *ConsoleLogger
di.BindSlice[Handler, *HTTPHandler]()   // Handler, or []Handler, resolves to every *HTTPHandler
di.BindArg[Clock](di.Ref(&fixedClock))  // Clock resolves to whatever the reference names
```

A binding maps an interface to an *argument*, in a scope. It does not fill any slot: autowiring
does that, and resolution follows the binding when it reaches the interface. With
`NotAutowired()`, nothing fills the slot and validation reports "argument N is not set" — the
binding cannot help, so pass the argument yourself. A variadic slot is left empty instead, with
no error, so a `BindSlice` you expected to arrive silently does not.

### 3.4 Retrieval

```go
svc, err := di.SvcByType[*Server](c)
svcs, err := di.SvcsByType[Handler](c)
svc, err := di.SvcByLabel[*Repo](c, "storage")
svcs, err := di.SvcsByLabel[Handler](c, "http")
svc, err := di.SvcByRef[*Server](c, ref)

out, err := di.ExecByRef(c, funcRef)
out, err := di.ExecByType[func() error](c)
out, err := di.ExecByLabel(c, "startup")
all, err := di.ExecAllByType[func() error](c)
all, err := di.ExecAllByLabel(c, "startup")
```

The singular forms error when there is not exactly one match. The plural forms return everything
and do not error on an empty result. `T` may be any interface the concrete service implements: the
cast is an ordinary type assertion.

Child services are private. A reference to one does not resolve through the root container.

One call runs every factory it needs before any method call. What you are handed back is fully
configured; what a factory was handed on the way is not. See [4.2](#42-scopes).

### 3.5 Defaults

Three settings, each per definition or per container:

| Setting | Default | Per definition | Per container |
|---|---|---|---|
| Lazy / eager | lazy | `.Lazy()` / `.Eager()` | `DefaultLazy()` / `DefaultEager()` |
| Shared / not shared | shared | `.Shared()` / `.NotShared()` | `DefaultShared()` / `DefaultNotShared()` |
| Autowired / not | autowired | `.Autowired()` / `.NotAutowired()` | `DefaultAutowired()` / `DefaultNotAutowired()` |

What a definition asks for wins; the container's default fills in the rest, as the definition is
registered.

`SetDefaultLazy`, `SetDefaultEager`, `SetDefaultShared`, `SetDefaultNotShared`,
`SetDefaultAutowired` and `SetDefaultNotAutowired` set the same three for **every container in the
process**. They still work and are **deprecated**: two containers cannot disagree, a library that
sets one changes its host's containers, a test that flips one leaks into the next, and they are
not safe for concurrent use.

### 3.6 The failure snapshot

| Variable | Meaning |
|---|---|
| `GODI_SNAPSHOT_ON_BUILD_ERR` | Write the graph when `Build` returns an error. Off unless set to something `strconv.ParseBool` reads as true. |
| `GODI_SNAPSHOT_PATH` | A directory to write into, or the file to write. Defaults to the temporary directory. |

It writes JSON and nothing else, so a godi binary carries no renderer for a debugging aid it will
almost never use. `godi view <path>` turns it into something to look at. Whatever happens, the
error `Build` returns is untouched.

---

## 4. The extension API

Everything here is in `github.com/michalkurzeja/godi/v2/di`.

### 4.1 Driving the builders

```go
b := di.NewContainerBuilder(di.NewConfig())
scope := b.RootScope()

f, err := di.NewFactory(NewServer)
scope.AddServiceDefinitions(di.NewServiceDefinition(f).SetScope(scope).SetLazy(false))

c, err := b.Build()
```

`ContainerBuilder`: `RootScope()`, `Scope(name)`, `Scopes()`, `ServiceDefinitionsSeq()`,
`FunctionDefinitionsSeq()`, `Container()`, `Compiler()`, `Build()`.

`Container()` is the container being built, and nil once `Build` has handed it over. A failed
`Build` keeps it.

`Container` mirrors those readers: `Root()`, `Scope(name)`, `Scopes()`,
`ServiceDefinitionsSeq()`, `FunctionDefinitionsSeq()`, alongside the retrieval methods.

The facade's `ServiceDefinitionBuilder.ParseAndBuild(scope)` is the one-call path for registering
a definition builder into a scope programmatically.

### 4.2 Scopes

A `Scope` is the unit of visibility: definitions, bindings and the instances built from them.
Every scope but the root has a parent, and `Chain()` yields from a scope up to the root.

Chain-wide lookups come in two forms: a sequence (`ServicesIDsByTypeInChainSeq`,
`ServiceDefinitionsByLabelInChainSeq`, `BindingsInChainSeq`, …) and the slice collected from it
(`GetServicesIDsByTypeInChain`, …). Reach for the sequence when the first match will do.

`Scope.NewChild(name)` makes a scope belonging to nobody. `Definition.NewChildScope(parent)` makes
one that belongs to the definition: named after it and back-linked, which is what everything
reporting on the container reads. A second scope registered under a name already taken is renamed
(`plugins`, `plugins#2`) rather than replacing the first.

**One call into the container is one `instantiationContext`.** Every public
`GetService*`/`ExecuteFunction*` starts one, and so does the eager-init pass for a whole build.
Within it, every factory runs before any method call: a service is built, published if it is
shared, and its method calls go to the back of a queue that is drained once the factory chain is
done. A queued call may build services of its own, and their calls join the back of the queue in
turn.

That rule is what makes wiring through a method call work from either end. `A`'s factory takes `B`,
`B` is configured with `MethodCall((*B).SetA)`: `B.SetA` cannot run while `A`'s factory is still
running, so it finds the published `A` rather than building a second one. It costs the other
guarantee — **a factory does not see the method calls of its dependencies**, because they have not
run yet.

A factory whose argument resolves to a definition already being built is a genuine cycle, and the
context reports it as one instead of overflowing the stack. Compile-time cycle validation normally
catches this first; `SkipCycleValidation` is what leaves it to runtime.

**A built container is safe to share across goroutines.** `shared` means one construction, however
many goroutines ask at once, and a caller is never handed a service whose method calls have not
run: a call stages what it builds and publishes it only once it has configured it.

**Construction serialises; retrieval does not.** One call builds at a time and the rest wait, so
two goroutines each building a different service for the first time go one after the other. Once a
service is built, resolving it takes a read lock and nothing else, so the warm path — which is
where the traffic is — runs fully in parallel.

Graph extraction never waits for a factory. The lock that construction holds across user code is
not the one guarding instances, so `di.LiveGraph` and `Scope.Instantiated` are unaffected by a
build in progress.

**A factory, method call or function must not resolve from the container that is building it** — it
would block on a lock its own caller holds. Declare the dependency as an argument.

Calling `GetService` directly returns an error: godi finds the factory's own frame on the stack.
Starting a goroutine to call it and waiting for that goroutine hangs instead. The new goroutine has
a stack of its own, carrying no sign of the factory, so godi cannot tell it from an unrelated
caller.

`ContainerBuilder.Build` itself is single-threaded (below), and a built container is read-only
apart from the instances it builds: registration is a build-time activity.

### 4.3 Definitions

`ServiceDefinition` and `FunctionDefinition` share an identity, labels, scopes and properties;
every fluent setter returns the concrete type.

Common: `ID()`, `Scope()`/`SetScope`, `ChildScope()`/`SetChildScope`/`NewChildScope`,
`EffectiveScope()`, `Labels()`/`SetLabels`/`AddLabels`/`RemoveLabels`, `IsLazy()`/`SetLazy`,
`IsAutowired()`/`SetAutowired`, `RegisteredAt()`.

Service-only: `Type()`, `Factory()`/`SetFactory`, `MethodCalls()`/`SetMethodCalls`/
`AddMethodCalls`/`RemoveMethodCalls`, `IsShared()`/`SetShared`, `Val()`/`SetVal`, `FactoryName()`,
`DeclaredAt()`.

Function-only: `Type()`, `Func()`/`SetFunc`, `DeclaredAt()`.

**Resolve against `EffectiveScope()`, never `Scope()`** — a definition with a child scope resolves
its own dependencies from inside it.

`FactoryName()`, `Factory().Type()` and `DeclaredAt()` describe the factory. A service registered
as a value is described by `Val()` instead, and deciding which of the two a reader should be shown
belongs to whoever is doing the showing — `graph/extract` makes that call, not the engine.

### 4.4 Arguments

`Arg` is **sealed**: its behaviour is unexported, so the kinds are godi's. An argument the compiler
passes and the graph would not understand is worse than no argument at all.

| Constructor | Resolves to |
|---|---|
| `NewLiteralArg(v)` | `v` itself. |
| `NewZeroArg(typ)` | The zero value of `typ`. |
| `NewRefArg(def)` | The service the definition builds. |
| `NewTypeArg(typ, slice)` | Services of the type, in the scope chain. |
| `NewLabelArg(label, typ, slice)` | Services carrying the label. |
| `NewFlexibleSliceArg(elem, allowEmpty)` | The slice type if something provides it, otherwise every service of the element type. What autowiring fills a slice slot with. |
| `NewCompoundArg(typ, args...)` | A `[]typ` from the given arguments — `typ` is the element type, and `Type()` reports `[]typ`. Returns `(nil, nil)` when given none. |
| `NewSpreadSliceArg(arg)` | `arg`, whose elements a compound takes one by one. Errors unless `arg` is a slice. |
| `NewSlottedArg(arg, n)` | `arg`, pinned to slot `n`. |

`ArgList` and `Slot` model a function's parameters. A slot is filled by `Set` (assignable to the
slot type) or `Append` (element-typed, for a slice or variadic slot; the elements become a compound
when read). A compound is a slice, so it `Set`s a slice slot rather than being appended to one.

Three entry points act on an argument: `ValidateArg(scope, arg)`, `ResolveArg(scope, arg)` and
`ResolveArgIDs(scope, arg)`. `ArgResolver` is the same three under an older name.

`TraceArg(scope, arg)` returns an `ArgTrace`: what the argument matched, by which mechanism
(`Resolution`), through which bindings (`BindingHop`), and why it matched nothing (`ArgFault`).
That is what resolution discards, and it is what the graph is built from.

### 4.5 Compiler passes

```go
pass := di.NewCompilerPass("my pass", di.PreAutomation, di.CompilerOpFunc(
	func(b *di.ContainerBuilder) error { ... },
)).WithPriority(-10)
```

| Stage | Runs |
|---|---|
| `PreAutomation` | Before anything automatic. Where your passes usually belong. |
| `Automation` | Interface binding, then autowiring. |
| `PreValidation` | After godi has wired, before it checks. |
| `Validation` | Argument validation, cycle validation. |
| `PreFinalization` | — |
| `Finalization` | Eager initialisation. |
| `PostFinalization` | After everything. |

Within a stage, lower priority runs first; passes of the same priority run in the order they were
added, and that is a guarantee rather than an accident of the sort.

A pass may register another pass: it joins the queue as soon as the pass adding it returns. One
whose place has already gone by is a build error naming both — running it now would run the stages
out of order, and running it later is not the place it asked for.

#### Reporting what a pass objects to

A pass that only returns an error says what went wrong and not what it went wrong about, so the
dependency graph of the failed build has nowhere to draw it. Report it instead, against the thing
it is about:

```go
b.ReportError(di.AtServiceArg(def, slot), err, "could not override argument %d of %s", i, def)
b.Report(di.Diagnostic{Severity: di.SeverityWarning, Site: di.AtService(def), Message: "looks expensive"})
```

| Site | Shown on |
|---|---|
| `AtContainer()` | The graph itself. Where a fault belongs to nothing narrower. |
| `AtScope(scope)` | The scope. |
| `AtService(def)` / `AtFunction(def)` | The node. |
| `AtServiceArg(def, slot)` / `AtFunctionArg(def, slot)` | The argument. Factory and method arguments alike. |

An error-severity diagnostic stops compilation once the reporting pass returns, and `Build` fails
with its `Err`; a pass that reports one should return `nil` rather than the same error twice. A
warning or an info note never fails a build, and stays on the container it produced.

`ReportError` splits the two readers: the message is the bare fault, shown beside an element that
already says where it is, and the wrapped error is what stands alone in a terminal. A pass that
returns an error rather than reporting still reaches the graph, at container level, because the
compiler records one for it.

The pass name is filled in by the compiler. A pass reports what it found; naming itself is not
its job.

`Compiler`: `AddPass(pass)`, `Passes()`, `Progress()`, `Run(builder)`.
`CompilerPass`: `Name()`, `Stage()`, `Priority()`, `WithPriority(n)`, `Run(builder)`.
`ContainerBuilder`: `Report(d)`, `ReportError(site, err, format, ...)`.
`Container`: `Diagnostics()`.
`BasePasses(skipCycleValidation)` returns godi's own five, and each is exported on its own as an
example of the shape: `NewInterfaceBindingPass()`, `NewAutowiringPass()`, `NewArgValidationPass()`,
`NewCycleValidationPass()` and `NewEagerInitPass()`. The pipeline is readable, not replaceable:
turning behaviour off is expressed per definition (`NotAutowired()`, `Lazy()`) and per container
(`SkipCycleValidation`).

### 4.6 Provenance

An argument you wrote, one godi autowired and one a pass substituted are byte-identical at
runtime. The compiler records which is which:

```go
origin, pass := slot.Origin()     // ArgOriginManual | ArgOriginAutowiring | ArgOriginCompilerPass | ArgOriginNone
origin, pass := binding.Origin()  // BindOriginManual | BindOriginAutobinding | BindOriginCompilerPass
```

Reading it is open — an override pass that replaces only what autowiring chose needs nothing else.
Declaring it is not: what a pass's edits mean is godi's own, so a pass is credited with its work
rather than claiming godi's.

A pass is credited with what it wired even when it also reported an error. The graph of a build that
stopped is where the difference matters most, and the wiring the failing pass did is still its own.

**Never identify a pass by name.** Names are neither unique nor stable, and a third-party pass may
be called "autowiring".

### 4.7 Where a definition came from

`RegisteredAt()` walks the captured stack past godi's own packages to the caller. A library that
wraps `godi.Svc` on its users' behalf is in the same position and can say so:

```go
func init() { di.MarkWiringPackage("github.com/acme/wiring") }
```

The path is matched exactly, never as a prefix.

### 4.8 The extras package

```go
extras.OverrideSvcArg(ref, slot, "replacement")           // replace an argument
extras.RemoveSvc(&ref)                                    // drop a service
extras.CaptureGraph(di.PreValidation, capture, opts...)   // the graph, partway through
```

---

## 5. The dependency graph

### 5.1 Getting one

```go
g, err := di.Graph(c)                             // a built container
src := di.LiveGraph(c)                            // a graph.Source, re-read on every call
src := graph.Static(g)                            // a graph you already have

g, err := extract.From(container)                 // the container itself, for extensions
g, err := extract.FromBuilder(b)                  // inside a pass, or after a failed build
```

`Build` hands back the `Container` interface and extraction reads the container underneath, so
`di.Graph` and `di.LiveGraph` find it for you. A container of your own standing in front of one
godi built keeps working if it implements `di.Unwrapper` — one method, `Unwrap() Container`.

`graph.Source` is an interface with one method, `Graph(Config) (*Graph, error)`; wrap a plain
function in `graph.SourceFunc`. `graph/serve` takes a Source, which is how the same handler serves
a file and a running container.

Extraction options: `WithLiteralValues(maxRunes)`, `WithRedactor(fn)`, `WithoutLiterals()`.
Literal values are left out by default: a literal is routinely a DSN or a token, and graphs get
committed and pasted into issues.

Diagnostic messages take the same care and a knob of their own, because they answer a different
question: `WithDiagnosticMarks()` keeps what is broken and drops what was said about it,
`WithoutDiagnostics()` drops both, and `WithDiagnosticRedactor(fn)` filters the text. Messages are
included by default — a graph of a failed build that will not say what failed is no use — and a
message from a factory carries **whatever that factory put in its error**.

### 5.2 The model

`Graph` holds `Scopes`, `Nodes`, `Edges`, `Bindings`, `Diagnostics`, an optional `Snapshot`
and a `SourceRoot`. It is plain data.

- **Node** — a service or a function: its type, name, signature, labels, flags, where it was
  registered and where it is defined, and a `Param` per argument.
- **Param** — one argument: what was asked for, what filled it (`Origin`, `OriginPass`), any
  literals, and whatever is wrong with it.
- **Edge** — one dependency: from a node, through a param, to a node, with the `Resolution` that
  matched and the `Bindings` it traversed.
- **Scope** — a box, with the definition that owns it when one does.

Display rules live on the model, so every format gives the same answer: `Node.Title`,
`Node.Subtitle`, `Node.KnownByName`, `Node.Anonymous`, `Param.Position`, `Edge.DecidedBy`,
`Edge.PassCredit`.

### 5.3 Diagnostics

**A diagnostic is stored on the element it is about.** `Graph`, `Scope`, `Node` and `Param` each
carry their own, and the graph itself takes the ones nothing narrower fits: a pass that could not
be scheduled, a definition that never made it into the container, a file written against a schema
this build does not know.

`AllDiagnostics()` walks them in a stable order — the container, then each scope, then each node
followed by its arguments — and pairs each with the ids of what it came from, which is what the
encoders render. `Where(g)` names that place in words. `Node.Faulty()` and `Param.Faulty()` ask
whether something carries an error rather than storing a second answer to the same question, so
what a reader is told is wrong cannot drift from what is drawn as wrong.

`Severity` is `info`, `warning` or `error`. Where a filter cut a diagnostic away with the element
it was about, `Graph.ElidedDiagnostics` counts it, so a narrowed graph does not read as a
container with nothing wrong with it.

### 5.4 Narrowing

```go
g = g.Select(
	graph.Focus(graph.ByType("*app.(*Server)"), graph.Dependencies(3)),
	graph.ExcludeLabels("infrastructure"),
	graph.HideMethodCalls(),
)
```

Matchers: `ByType`, `ByName`, `ByLabel`, `ByID`, `ByFile`, combined with `All`, `Any`, `Not`.
Patterns are globs matched against the qualified name and the short form alike. `Focus` reaches
outwards, limited by `Dependencies(n)` and `Consumers(n)`, and never turns a corner. Where a limit
cut the graph, the nodes left at the edge carry `Elided`.

Narrowing is a separate call from extraction, which is how one extraction answers several
questions — and a `Filter` is its own type, so an extraction option cannot be passed to `Select`.

### 5.5 Partial graphs

A graph taken before the container finished carries a `Snapshot`: the stage and pass in progress,
the passes already done, whether autowiring has run, and the pass that failed if one did. Every
encoder prints it, because a half-wired graph otherwise reads as a finished one with dependencies
missing. Saying that an argument is unwired is gated on `Snapshot.Autowired`: before autowiring,
every slot is empty and marking them says nothing.

Every format draws an edge feeding an argument that will not resolve, and one that closes a
cycle, as wrong. How it says so differs, because only one of them has room for both facts: the
HTML page puts a red glow behind the line, so the line still says who chose the dependency,
while Graphviz gives an edge one colour and no second layer, so there it turns red outright.

### 5.6 Formats and the CLI

| Package | Output |
|---|---|
| `graph/text` | An indented outline. Nothing to install. |
| `graph/dot` | Graphviz DOT, one port per argument row. |
| `graph/html` | A self-contained interactive page. No CDN, no server. |
| `graph/json` | The interchange format. `Graph.WriteJSON` writes it without an encoder; `graph.ReadJSON` reads it back with its `Metadata`. |

A schema `ReadJSON` does not recognise is a warning, not a failure: the model grows by adding
fields, and half a graph beats none when a build has already gone wrong.

```shell
go install github.com/michalkurzeja/godi/v2/cmd/godi@latest

godi view graph.json                     # serve on 127.0.0.1:7777 and open a browser
godi export text graph.json
godi export dot graph.json | dot -Tsvg -o graph.svg
```

---

## 6. Behaviour worth knowing

**A binding does not fill a slot.** It maps an interface to an argument in a scope; autowiring
fills the slot, and resolution follows the binding. With autowiring off, nothing fills it.

**Cycle detection covers factory arguments only.** Method calls are the documented way out of a
circular dependency, so they are not checked. A factory cycle that reaches runtime — which needs
`SkipCycleValidation` — is reported as an error naming the chain.

**Method calls run after every factory the call needed.** So a factory is handed dependencies whose
method calls have not run. Store them; do not use them while constructing. See [4.2](#42-scopes).

**A method call's receiver is slot 0.** `MethodCall((*Service).Method, args...)` passes a method
expression, whose first parameter is the receiver; godi fills it. Your arguments start at slot 1.

**Registering the same method twice keeps the last.** Method calls are keyed by name.

**`BindSlice[Iface, To]()` resolves to `[]Iface`.** It spreads `SliceOf[To]()` into a
`Compound[Iface]`, so the element type is the interface and every implementation is converted to
it.

**A binding is fitted to the argument that resolves through it.** `BindType` names one
implementation and `BindSlice` collects every one, while the argument asks for a single value or a
slice. A single value reaching a slice argument is wrapped in a one-element slice; a slice reaching
a single argument is a build error, since there is nothing sensible to pick.

**Child services are private.** They live in a scope of their own, so nothing outside can retrieve
one, and a reference to one does not resolve through the container.

**An eagerly executed function's return values are dropped.** Nothing is there to receive them.

**`ContainerBuilder.Build` is not safe for concurrent use.** Nothing guards the "already built"
flag.

**A graph cannot be taken from the facade `Builder`.** The routes are a compiler pass, a failed
build's snapshot, and the built container.

---

## 7. Documentation that must stay in step

`README.md`, this file, and `plugins/godi/skills/godi-v2/SKILL.md` describe the same behaviour to
three audiences. Changing one means checking the others; changing the skill also means bumping
`version` in `plugins/godi/.claude-plugin/plugin.json`, or installed consumers never receive the
update.
