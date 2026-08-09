---
name: godi-v2
description: >-
  Write and understand Go code that uses the godi v2 dependency-injection container
  (github.com/michalkurzeja/godi/v2). Use this whenever code wires services or
  dependencies with godi — calls like di.New(), di.Svc(), di.Func(), di.SvcByType,
  di.Type[...], di.Ref, di.SliceOf, di.BindType, MethodCall, Children, or CompilerPasses —
  or whenever the task is to set up a DI container, register services/factories,
  autowire dependencies, resolve interfaces, define child scopes, or add compiler passes
  in a Go project that imports godi. Trigger even when "godi" is not named explicitly but
  the code clearly uses this container's API.
---

# godi v2

godi is a reflection-based dependency-injection container for Go (requires Go 1.25).
You declare *what* services exist and *how* they are built (factory functions); godi
figures out the dependency graph, validates it at build time, and creates values on demand.

Import it aliased as `di` — this is the universal convention:

```go
import di "github.com/michalkurzeja/godi/v2"
```

The extras package (optional compiler passes) is `github.com/michalkurzeja/godi/v2/extras`.

## The mental model

1. **Build once, at startup.** Configure everything through the fluent builder, call
   `Build()`, and you get a read-only `Container`. All the reflection cost is paid here.
2. **Errors surface at build time, not retrieval time.** If `Build()` returns no error,
   every registered service *can* be instantiated and no dependency is missing. The only
   errors after build are factory errors that can only happen at instantiation.
3. **Autowiring is unambiguous or it fails.** godi never guesses. If a dependency matches
   zero or more-than-one candidate where exactly one is required, it's a build error with a
   clear message — not a silent choice.

## Core workflow

```go
c, err := di.New().
    Services(
        di.Svc(NewMySvc),                       // dependency autowired by type
        di.Svc(NewMyOtherSvc, "Hello, world!"), // string dependency provided manually
    ).
    Build()
if err != nil {
    // build/validation error — malformed config, missing/ambiguous dep, cycle, etc.
    panic(err)
}

svc, err := di.SvcByType[*MySvc](c) // instantiates lazily on first request
```

The builder has four registration methods, all optional and chainable in any order,
then `Build()`:

```go
di.New().
    Services(...).        // *ServiceDefinitionBuilder, from di.Svc / di.SvcVal
    Functions(...).       // *FunctionDefinitionBuilder, from di.Func
    Bindings(...).        // *InterfaceBindingBuilder, from di.BindType / BindArg / BindSlice
    CompilerPasses(...).  // *di.CompilerPass, e.g. from the extras package
    Build()
```

`di.New()` also accepts options; the only built-in one is `di.SkipCycleValidation()`
(a performance optimization for very large graphs).

## Factories

A factory is any function that returns **one value (the service), optionally followed by an
`error`**. Arguments can be any types, including variadic.

```go
func NewSvc() *Svc                               // valid
func NewSvc() (*Svc, error)                       // valid
func NewSvc(a A, b B) (Svc, error)                // valid
func NewSvc(a A, b B, opts ...Opt) (Svc, error)   // valid (variadic)

func NewSvc() (Svc, Other)          // INVALID: 2nd return must be error
func NewSvc() (Svc, Other, error)   // INVALID: too many returns
```

## Services

Register with `di.Svc(factory, args...)`. Args are optional — anything you omit gets
autowired. The builder is fluent; every option is optional:

```go
var ref di.SvcReference
di.Svc(NewService, "manual-arg").
    Bind(&ref).                                     // bind to a reference for later retrieval
    Labels("foo", "bar").                           // attach labels
    MethodCall((*Service).SomeMethod, "arg").       // call a method after construction
    Children(di.Svc(NewChildSvc)).                  // private child services (child scope)
    Lazy().                                          // or Eager()
    Shared().                                        // or NotShared()
    Autowired()                                      // or NotAutowired()
```

Most services need only a factory and maybe an arg: `di.Svc(NewService, "manual-arg")`.

To register an already-built instance, use `di.SvcVal`:

```go
di.SvcVal(myLogger) // equivalent to di.Svc(func() *Logger { return myLogger })
```

## Arguments and autowiring

When you leave an argument out, autowiring resolves it **by type** (this is on by default):

- **Non-slice arg:** finds exactly one service of the matching type. Zero or >1 is an error.
- **Slice / variadic arg:** first tries a matching slice-type service (e.g. `[]*Svc`); if
  none, collects all services of the element type (`*Svc`) into the slice.
- **Interface arg:** first tries an exact type match; otherwise finds implementations
  (see Interfaces below).

**Arg order doesn't matter** — godi matches provided args to parameters by type, filling
the first free matching slot left-to-right. `di.Svc(NewService, "a", 1, "b")` for
`NewService(a, b string, c int)` calls `NewService("a", "b", 1)`.

### Explicit arg types

Use these when autowiring can't (or shouldn't) decide. Pass them positionally to
`di.Svc` / `di.Func` / `MethodCall`:

| Builder | Resolves to |
|---|---|
| `di.Val(v)` | the literal value `v` (a bare non-reference value is shorthand for this); `v` may not be `nil` |
| `di.Ref(&ref)` | the service bound to `ref` (a bare `&ref` is shorthand for this) |
| `di.Type[T]()` | the single service of type `T` (error if not exactly one) |
| `di.Type[T]("label")` | the single service of type `T` carrying that label |
| `di.SliceOf[T]()` | all services of type `T` as `[]T` |
| `di.SliceOf[T]("label")` | all services of type `T` with that label |
| `di.Compound[T](args...)` | combines several args into one `[]T` — niche, mostly for generic/extension code |
| `di.SpreadSlice(arg)` | inside `di.Compound[T]`, takes the elements of the `[]T` that `arg` resolves to instead of the slice itself |

`di.Val(nil)` is an error — an untyped nil (and a nil interface, which is the same once boxed in an
`any`) has no type to be slotted by. Write `di.Val((*Logger)(nil))`. A service that *is* nil is
fine and is injected as nil.

Shorthands: a plain value → `di.Val`; a `*di.SvcReference` → `di.Ref`. So these are equal:

```go
di.Svc(NewService, "literal", &ref)
di.Svc(NewService, di.Val("literal"), di.Ref(&ref))
```

Pin an arg to a specific parameter index with `.Slot(n)` on any arg builder — rarely needed,
since type-matching usually suffices:

```go
di.Svc(NewService, di.Val("x").Slot(1))
```

## Interfaces

If a factory arg is an interface type, godi resolves it automatically:

- **Single (non-slice) interface arg:** succeeds only if exactly one registered service
  implements it. Multiple implementations → build error (ambiguous).
- **Slice/variadic interface arg:** collects all implementations, even of different types.

```go
// One implementer of `any` (int) -> autowired into func(any) string:
di.New().Services(di.SvcVal(42), di.Svc(func(v any) string { return fmt.Sprint(v) }))
```

When multiple implementations exist and you need a single one, resolve the ambiguity with a
**binding** (in `.Bindings(...)`):

```go
di.BindType[Iface, Impl]()  // bind interface Iface to the service of concrete type Impl
di.BindArg[Iface](di.Ref(&ref))   // bind Iface to any arg expression
di.BindSlice[Iface, Impl]()       // bind a []Iface arg to all services of type Impl
```

A binding is fitted to the argument resolving through it: `di.BindType` on a `[]Iface` argument
gives a one-element slice, while `di.BindSlice` on a single `Iface` argument is a build error.

Example — two `any` implementers, bind the single-arg case to the int:

```go
c, _ := di.New().
    Services(di.SvcVal(42), di.SvcVal(false), di.Svc(func(v any) string { return fmt.Sprint(v) })).
    Bindings(di.BindType[any, int]()).
    Build()
```

Without the binding, that config fails: `multiple implementations of interface interface {} found: [int bool]`.

## Functions

Functions are dependency-injected callables not tied to any service. Their args autowire
exactly like factory args. They're especially handy in tests (inject mocks as services,
configure expectations in a function).

```go
var ref di.FuncReference
di.Func(myFunc, "manual-arg").
    Bind(&ref).
    Labels("foo").
    Children(di.Svc(NewChildSvc)).  // functions can have child *services*
    Lazy().                          // or Eager()
    Autowired()                      // or NotAutowired()
```

Functions have no `Shared`/`NotShared` (they re-execute) and no method calls. Execute them
via the retrieval API (see below). Eagerly-executed functions can't return values to you.

## Method calls

Register a method to run once the service is constructed. Its args are injected too.
Pass the method as a **method expression** (`(*Service).Method`) — godi injects the receiver
as slot 0 automatically, so you only supply args for the parameters *after* the receiver.

```go
di.Svc(NewMySvc).MethodCall((*MySvc).SetStr, "hello")
```

One call into the container runs **every factory it needs before any method call**. Method calls
are then run oldest service first; one that pulls in a new service builds it there and then, and
that service's own calls join the back of the queue.

Method calls are the standard workaround for **circular dependencies**: if A needs B and B
needs A, wire one direction through a factory and the other through a method call. Either can
be asked for first — both get the same pair of instances. (Cycle validation only inspects
factory args, not method-call args.)

⚠️ A factory does **not** see the method calls of the services it is handed — they have not run
yet. Store a dependency; don't use it while constructing.

⚠️ Registering the same method twice on one service **silently replaces** the first — only
the last `MethodCall` for a given method name takes effect.

## Child services (scopes)

Children are private services visible only to their parent and siblings. They can't be
retrieved from the container or used as dependencies elsewhere — but they *can* pull
dependencies from outer scopes.

```go
di.Svc(NewStringsCollector).Children(
    di.SvcVal("child-str"), // only NewStringsCollector (and its siblings) can see this
)
```

Children can nest arbitrarily deep. Note: `extras.OverrideSvcArg` / `RemoveSvc` only reach
root-scope definitions, not child-scope ones.

## Retrieving from the container

Three lookup keys — reference, type, label — each with single vs. multiple variants:

```go
// Services
svc,  err := di.SvcByRef[T](c, ref)      // ref is a di.SvcReference (value, not pointer)
svc,  err := di.SvcByType[T](c)          // error unless exactly one
svcs, err := di.SvcsByType[T](c)         // all; never errors on "none" (empty slice)
svc,  err := di.SvcByLabel[T](c, "lbl")  // error unless exactly one
svcs, err := di.SvcsByLabel[T](c, "lbl") // all; no error on none

// Functions (return values as []any per call)
res,  err := di.ExecByRef(c, &ref)       // ref is a *di.FuncReference here
res,  err := di.ExecByType[FnType](c)    // FnType is the Go func type, e.g. func() *Svc
ress, err := di.ExecAllByType[FnType](c) // [][]any
res,  err := di.ExecByLabel(c, "lbl")
ress, err := di.ExecAllByLabel(c, "lbl") // [][]any
```

**The single vs. multiple rule:** "single" variants (`SvcByType`, `SvcByLabel`,
`ExecByType`, `ExecByLabel`) error if there isn't *exactly one* match. "Multiple" variants
(`Svcs...`, `ExecAll...`) return everything and never error on an empty result.

`SvcByRef[T]` / `SvcByType[T]` do a Go type assertion to `T`, so `T` may be an interface the
concrete service implements (e.g. `di.SvcByRef[fmt.Stringer](c, ref)`).

## Behaviour flags

Three settings, configurable per definition or per container:

- **Lazy / Eager** — lazy (default) builds on first request; eager builds all at end of
  `Build()`. `Lazy()`/`Eager()` per definition; `di.New(di.DefaultLazy())`/`di.New(di.DefaultEager())`
  per container.
- **Shared / NotShared** (services only) — shared (default) caches and reuses one instance;
  not-shared builds a fresh instance every injection/retrieval. `Shared()`/`NotShared()`;
  `di.DefaultShared()`/`di.DefaultNotShared()`.
- **Autowired / NotAutowired** — autowired (default) resolves omitted args by type;
  not-autowired means *you* must supply every arg, except a variadic one, which may be
  left out and is then called with none. `Autowired()`/`NotAutowired()`;
  `di.DefaultAutowired()`/`di.DefaultNotAutowired()`.

What a definition asks for wins; the container's default fills in the rest, as the definition
is registered. That includes a definition a compiler pass registers.

⚠️ The `di.SetDefault*` functions set the same three for **every container in the process**.
They still work and are deprecated — a test that flips one leaks into the next, and they are
not concurrency-safe.

## Compiler passes and the extras package

Compiler passes inspect and mutate the config during `Build()`. The `extras` package
provides ready-made ones (run at the `PreAutomation` stage, before autowiring):

```go
import "github.com/michalkurzeja/godi/v2/extras"

var ref di.SvcReference
di.New().
    Services(di.Svc(strconv.Itoa, 0).Bind(&ref)).
    CompilerPasses(
        extras.OverrideSvcArg(ref, 0, 42),   // replace arg at slot 0 (value ref)
        // extras.OverrideFuncArg(funcRef, 0, x)
        // extras.RemoveSvc(&ref)             // takes a *pointer*
        // extras.RemoveFunc(&funcRef)
    ).
    Build()
```

Note the API asymmetry: `OverrideSvcArg` takes the reference by **value**, `RemoveSvc`
takes it by **pointer**. Both only affect root-scope definitions.

Write a custom pass with `di.NewCompilerPass(name, stage, op)`, wrapping a
`di.CompilerOpFunc`. Stages run in this order: `PreAutomation`, `Automation`,
`PreValidation`, `Validation`, `PreFinalization`, `Finalization`, `PostFinalization`.
Within a stage, lower `.WithPriority(n)` runs first.

```go
pass := di.NewCompilerPass("my pass", di.PreAutomation, di.CompilerOpFunc(
    func(b *di.ContainerBuilder) error {
        for scope, def := range b.ServiceDefinitionsSeq() {
            _ = scope; _ = def // inspect / mutate definitions
        }
        return nil
    },
))
```

A definition a pass registers with `svcBuilder.ParseAndBuild(b.RootScope())` takes the
container's defaults for the properties it did not set itself. `b.Defaults()` reads them.

## Seeing how a container is wired

A built container can hand you its dependency graph. This is the only way to recover *how*
each dependency got there: at runtime an argument you wired by hand, one godi autowired,
one resolved through a binding godi created, and one substituted by a compiler pass are
indistinguishable.

```go
import (
	di "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/graph/text"
)

g, err := di.Graph(c)                     // di.LiveGraph(c) for a graph.Source, re-read on every call
err = g.Encode(os.Stdout, text.New())
```

Reading a container is `graph/extract`'s job; `graph` is the model only, and links none of the
container engine. Encoders live in their own packages, so a binary compiles the formats it asks
for: `graph/text` (an indented outline, nothing to install), `graph/dot` (Graphviz, one port per
argument row), `graph/html` (a self-contained interactive page — no CDN, no server), `graph/json`
(the interchange format).

Writing JSON is also on the model itself, because it is what the library writes when it cannot
afford to depend on an encoder:

```go
err = g.WriteJSON(w)                      // or g.Encode(w, json.New(json.Indent("  ")))
g, md, err := graph.ReadJSON(r)           // md.Schema, md.WrittenAt, md.GodiVersion
src := graph.Static(g)                    // a graph read from a file, as a graph.Source
```

A schema `ReadJSON` does not recognise is a warning on the graph, not an error — the graph is
decoded anyway.

An edge feeding an argument that will not resolve is drawn as wrong, and so is one that
closes a cycle. The HTML page glows it red behind the line, keeping the line's own colour for
who chose it; Graphviz has no second layer, so there the edge turns red outright.

An argument a compiler pass gave up on has no wiring to draw, because the build stops before
anything fills the slot. The implementations it could not choose between are drawn instead,
each as a `Candidate` edge. An ambiguous interface argument is two red edges out of an argument
that resolved to neither.

**A diagnostic is stored on the element it is about.** `Graph`, `Scope`, `Node` and `Param`
each carry their own `Diagnostics`, and the graph itself takes what fits nothing narrower.
`AllDiagnostics()` walks them and is what the encoders print; `Node.Faulty()` and
`Param.Faulty()` ask whether one carries an error, so what a reader is told is wrong and what
is drawn as wrong cannot disagree.

Filters work on the model, so every format gets them. Reach for them on any real container:
past a hundred nodes a whole-graph picture is unreadable.

```go
g, err := di.Graph(c)

g = g.Select(
	graph.Focus(graph.ByType("*app.(*Server)"), graph.Dependencies(3)),
	graph.ExcludeLabels("infrastructure"),
	graph.HideMethodCalls(),
)
```

`Graph` takes extraction `Option`s (`WithLiteralValues`, `WithRedactor`, `WithoutLiterals`,
and `WithDiagnosticMarks`/`WithoutDiagnostics`/`WithDiagnosticRedactor` for the same care over
diagnostic text — a message from a factory carries whatever that factory put in its error);
`Select` takes `Filter`s. They are different types on purpose - neither compiles in the
other's place. `Focus` limits its reach with `Dependencies(n)` and `Consumers(n)`.

Matchers are `ByType`, `ByName`, `ByLabel`, `ByID`, `ByFile`, plus `All`, `Any` and `Not`.
Patterns are globs (`*` = any run of characters), matched against the qualified name and
the short form alike, so `ByType("app.(*Server)")` and `ByType("github.com/acme/*")` both
work.

Wiring can be read partway through the build, from a compiler pass. Such a graph carries a
`graph.Snapshot` saying when it was taken and which passes had run; every format prints it,
because a half-wired graph otherwise reads as a finished one with dependencies missing.

```go
extras.CaptureGraph(engine.PreAutomation, func(g *graph.Graph) error { ... })  // as declared
extras.CaptureGraph(engine.PreValidation, func(g *graph.Graph) error { ... })  // after autowiring
```

A failed `Build` leaves the builder standing, which is what the failure snapshot is written
from - and what `extract.FromBuilder(b)` reads inside a pass or after a failure.
`Snapshot.Failed` names the pass that failed, and what that pass objected to is on the
argument or the service it objected to, in the words the build failed with. Every faulty node
is drawn with a red border in the viewer.

A compiler pass of your own reaches the same place. Report what you object to against the
thing it is about, rather than only returning an error, and the graph of the failed build
shows it there:

```go
b.ReportError(di.AtServiceArg(def, slot), err, "could not override argument %d of %s", i, def)
b.Report(di.Diagnostic{Severity: di.SeverityWarning, Site: di.AtService(def), Message: "looks expensive"})
```

The sites are `AtContainer()`, `AtScope(scope)`, `AtService(def)`, `AtFunction(def)`,
`AtServiceArg(def, slot)` and `AtFunctionArg(def, slot)`. A fault about several elements names
the rest in `Related`, as sites of their own. The graph draws a candidate edge from the argument
to each of them. An error-severity diagnostic fails the build once the pass returns, so report it
and return `nil`; a warning does not, and stays on the container the build produced.

No code is needed for the common case. Set `GODI_SNAPSHOT_ON_BUILD_ERR=true` and a failed
`Build` writes its graph as JSON, printing the path and the command to run on stderr;
`GODI_SNAPSHOT_PATH` chooses a directory or a file, defaulting to the temporary directory.
The error `Build` returns is unchanged either way.

Rendering lives in a separate binary, so that no godi binary carries Graphviz or the means
to start a browser:

```shell
go install github.com/michalkurzeja/godi/v2/cmd/godi@latest

godi view graph.json                  # serve on 127.0.0.1:7777 and open a browser
godi export text graph.json           # an outline, in the terminal
godi export dot graph.json | dot -Tsvg -o graph.svg
```

`graph/serve` is the handler behind `godi view`, and takes a `graph.Source`, so it serves a
live container as readily as a file: `serve.Listen("127.0.0.1:0", di.LiveGraph(c))`.

A **root** is a node nothing injects — an entry point, or wiring nothing uses. godi does not
guess which: a service fetched at runtime with `SvcByType` leaves no trace in the container.

`Container.Print(w)` and `di.Print(scope, w)` still work but are deprecated: they write a plain
outline of their own, so that no godi binary links an encoder for them. Prefer `di.Graph`
with `graph/text` in new code.

## Concurrency

A built container is safe to share across goroutines. A shared service is built once no matter
how many goroutines ask at once, and none of them sees it before its method calls have run.
Construction serialises (one call builds at a time); resolving an already-built service does not.
Registration is build-time only — treat a built container as read-only.

⚠️ A factory, method call or function must **never** resolve from the container that is building
it: it blocks on a lock its own caller holds. Declare the dependency as an argument.

Calling the container directly returns an error. Starting a goroutine to call it and waiting for
that goroutine hangs instead — the new goroutine looks like any other caller, so godi cannot tell
it apart.

## Common pitfalls

- **Ambiguous autowiring.** Two services of the same type (or two interface implementers)
  where a single one is needed → build error. Fix with a label + `di.Type[T]("label")`, a
  `di.Ref`, or an interface binding.
- **Forgetting the second return value must be `error`.** Any other 2nd return type is an
  invalid factory.
- **Method expression vs. method value.** Use `(*T).Method` (expression, receiver as first
  param), not `instance.Method`. godi supplies the receiver.
- **Child services aren't retrievable.** If you need to `SvcByType` it, it can't be a child.
- **Changing global defaults after defining services** has no effect on those definitions.
- **Circular factory deps** are rejected. Break the cycle with a method call.
- **Using a dependency inside a factory** sees it before its method calls have run.
- **Resolving from the container inside a factory** errors, or hangs if done from a goroutine the
  factory waits on. Pass the dependency in instead.
- **`di.Type[T](l1, l2)` only uses the last label** — passing multiple is silently ignored.

## Deep reference

For internals, edge-case resolution semantics, the full exported API surface, the `di/`
sub-package, arg-resolver behaviour, and documented quirks, read
`references/full-reference.md`. Consult it when you hit unusual autowiring/scope behaviour,
are writing a compiler pass or extension, or need exact error-message text.
