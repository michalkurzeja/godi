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
| `di.Val(v)` | the literal value `v` (a bare non-reference value is shorthand for this) |
| `di.Ref(&ref)` | the service bound to `ref` (a bare `&ref` is shorthand for this) |
| `di.Type[T]()` | the single service of type `T` (error if not exactly one) |
| `di.Type[T]("label")` | the single service of type `T` carrying that label |
| `di.SliceOf[T]()` | all services of type `T` as `[]T` |
| `di.SliceOf[T]("label")` | all services of type `T` with that label |
| `di.Compound[[]T](args...)` | combines several args into one `[]T` — niche, mostly for generic/extension code |

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

Register a method to run right after the service is constructed. Its args are injected too.
Pass the method as a **method expression** (`(*Service).Method`) — godi injects the receiver
as slot 0 automatically, so you only supply args for the parameters *after* the receiver.

```go
di.Svc(NewMySvc).MethodCall((*MySvc).SetStr, "hello")
```

Method calls are the standard workaround for **circular dependencies**: if A needs B and B
needs A, wire one direction through a factory and the other through a method call. (Cycle
validation only inspects factory args, not method-call args.)

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

Three settings, configurable per definition or globally:

- **Lazy / Eager** — lazy (default) builds on first request; eager builds all at end of
  `Build()`. `Lazy()`/`Eager()` per definition; `di.SetDefaultLazy()`/`di.SetDefaultEager()`.
- **Shared / NotShared** (services only) — shared (default) caches and reuses one instance;
  not-shared builds a fresh instance every injection/retrieval. `Shared()`/`NotShared()`;
  `di.SetDefaultShared()`/`di.SetDefaultNotShared()`.
- **Autowired / NotAutowired** — autowired (default) resolves omitted args by type;
  not-autowired means *you* must supply every arg. `Autowired()`/`NotAutowired()`;
  `di.SetDefaultAutowired()`/`di.SetDefaultNotAutowired()`.

⚠️ **Global defaults are applied when `di.Svc`/`di.Func` is called**, not at build time.
Change them once at startup *before* defining anything; changing them later won't affect
already-created definitions. They're plain package globals — **not concurrency-safe**.

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

## Seeing how a container is wired

A built container can hand you its dependency graph. This is the only way to recover *how*
each dependency got there: at runtime an argument you wired by hand, one godi autowired,
one resolved through a binding godi created, and one substituted by a compiler pass are
indistinguishable.

```go
import (
	"github.com/michalkurzeja/godi/v2/graph"
	"github.com/michalkurzeja/godi/v2/graph/text"
)

g, err := graph.Extract(c)
err = g.Encode(os.Stdout, text.New())
```

`graph` is the model only. Encoders live in their own packages, so a binary compiles the
formats it asks for: `graph/text` (an indented outline, nothing to install), `graph/dot`
(Graphviz, one port per argument row), `graph/html` (a self-contained interactive page —
no CDN, no server). `graph/view` opens the result via a temporary file:

```go
path, err := view.Open(c, html.New())
```

Filters work on the model, so every format gets them. Reach for them on any real container:
past a hundred nodes a whole-graph picture is unreadable.

```go
g, err := graph.Extract(c,
	graph.Focus(graph.ByType("*app.(*Server)"), graph.Downstream(3)),
	graph.ExcludeLabels("infrastructure"),
	graph.HideMethodCalls(),
)
```

Matchers are `ByType`, `ByName`, `ByLabel`, `ByID`, `ByFile`, plus `All`, `Any` and `Not`.
Patterns are globs (`*` = any run of characters), matched against the qualified name and
the short form alike, so `ByType("app.(*Server)")` and `ByType("github.com/acme/*")` both
work.

A **root** is a node nothing injects — an entry point, or wiring nothing uses. godi does not
guess which: a service fetched at runtime with `SvcByType` leaves no trace in the container.

`Container.Print(w)` and `di.Print(scope, w)` still work but are deprecated; they now render
through `graph/text`. Prefer `graph.Extract` in new code.

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
- **`di.Type[T](l1, l2)` only uses the last label** — passing multiple is silently ignored.

## Deep reference

For internals, edge-case resolution semantics, the full exported API surface, the `di/`
sub-package, arg-resolver behaviour, and documented quirks, read
`references/full-reference.md`. Consult it when you hit unusual autowiring/scope behaviour,
are writing a compiler pass or extension, or need exact error-message text.
