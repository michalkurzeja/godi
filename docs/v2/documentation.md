# godi v2 — Comprehensive Technical Documentation

> This document is intended for AI agents. It is exhaustive and precise. Every exported symbol is covered. Architectural subtleties and known quirks are called out explicitly.

---

## 1. Overview

**Module:** `github.com/michalkurzeja/godi/v2`  
**Go version requirement:** 1.24  
**Repository:** github.com/michalkurzeja/godi

`godi` is a reflection-based dependency injection (DI) container library for Go. Its purpose is to eliminate manual wiring of application components. The user declares *what* services exist and *how* they are created (via factory functions); `godi` figures out the dependency graph, validates it, and resolves values on demand.

### Main features

- **Factory-based service registration.** Any function that returns one or two values (`(T)` or `(T, error)`) is a valid factory.
- **Autowiring.** The compiler automatically matches unresolved factory/method/function arguments to registered services by type.
- **Interface resolution.** When a parameter type is an interface, `godi` finds all services that implement it and either autowires a single implementation or assembles a slice of implementations.
- **Child scopes.** Services can declare private child services that are invisible to the rest of the container but visible to their parent.
- **Method calls.** Post-construction method invocations that are also dependency-injected.
- **Functions.** Arbitrary dependency-injected callables that are not tied to any service. Useful in testing.
- **Compiler passes.** A staged, priority-ordered extension mechanism that can inspect and mutate the container configuration before it is finalised.
- **Interface bindings.** Explicit static mappings from an interface type to an argument expression, usable to resolve ambiguity when multiple implementations exist.
- **Shared / not-shared.** Services are cached by default; this can be turned off per service.
- **Lazy / eager.** Services and functions are instantiated/executed lazily by default; eager mode triggers them at the end of container build.
- **Cycle detection.** The compiler validates the service dependency graph for cycles using a DAG library.
- **`extras` package.** Built-in compiler passes for common mutation operations (override arg, remove service/function).

### Design goals

- Zero runtime overhead after `Build()` returns.
- All errors are surfaced at build time, not at service retrieval time (except factory errors that can only surface at instantiation).
- Unambiguous and deterministic autowiring — if a choice is ambiguous, it is an error, not a guess.
- Extensibility through compiler passes.
- A thin, friendly public facade in the root package that delegates to a richer but lower-level `di/` sub-package.

---

## 2. Architecture

### 2.1 Two-layer design

The library is split into two packages:

| Package | Role |
|---|---|
| `github.com/michalkurzeja/godi/v2` (root) | Public facade. All types a typical user needs. Fluent builders. Generic helper functions for container access. |
| `github.com/michalkurzeja/godi/v2/di` | Core implementation. All runtime logic: scope tree, definition registry, compiler, arg resolution, instantiation. |

Users interact almost exclusively with the root package. The `di/` package is consumed by the root package and by extension authors (compiler pass authors).

The root package re-exports `di.ID` and `di.Label` as type aliases so that users never need to import `di/` directly for everyday use.

### 2.2 Key components

#### Container (`di.Container`)

The runtime store of services and functions. After `Build()` it is immutable from the user's perspective. Internally it holds a tree of `*Scope` objects. The public `Container` interface (defined in `di.go`) is what user code receives; the concrete `*di.Container` struct implements it.

The root `Container` interface (in the root package) must be satisfied to call the retrieval helpers (`SvcByType`, `ExecByLabel`, etc.). A testify mock of this interface lives in `mocks/container.go`.

#### ContainerBuilder (`di.ContainerBuilder`)

Mutable pre-build state. Holds the root `*Scope` and the `*Compiler`. During the build phase compiler passes traverse it via `ServiceDefinitionsSeq()` / `FunctionDefinitionsSeq()` iterators (Go 1.23 `iter.Seq2` style). Once `Build()` is called the builder is locked.

#### Definitions

Two concrete definition types exist:

- `di.ServiceDefinition` — holds an ID (UUID), factory, method calls map, labels, scope reference, child scope reference, and the three flags: `lazy`, `shared`, `autowired`.
- `di.FunctionDefinition` — same pattern but no `shared` flag and no method calls; holds a `*Func` instead of a `*Factory`.

Both implement the `di.Definition` interface: `ID() ID`, `Type() reflect.Type`, `Labels() []Label`.

#### Factory, Method, Func (`di/function.go`)

- `Func` is the fundamental executable unit: it wraps a `reflect.Value`, an `*ArgList`, and calls `CallSlice` or `Call` depending on variadicity.
- `Factory` wraps a `Func` and adds validation that the function returns 1 or 2 values (the second must be `error`). It exposes `Creates() reflect.Type`.
- `Method` wraps a `Func` and injects the service instance as argument slot 0 (a `*SlottedArg`). It validates that the function returns at most one value (must be `error` if present).

#### ArgList and Slots (`di/arg.go`)

`ArgList` models the parameter list of a function as a slice of `*Slot` objects. Each `Slot` knows its position, type, whether it is the variadic slot, and whether it has been filled.

Filling a slot can happen in two ways:
- **Set** — assign an `Arg` whose type is directly assignable to the slot type.
- **Append** — for slice/variadic slots, accumulate element-typed `Arg` objects; they are later wrapped into a `compoundArg` on `Arg()` retrieval.

When `AddArgs` is called on a `Func`, slotted args are processed first (they target a specific index), then unslotted args fill the first matching free slot left-to-right.

#### Arg types (`di/arg.go`)

| Concrete type | Public constructor | Resolution behaviour |
|---|---|---|
| `literalArg` | `NewLiteralArg(v any)` | Returns `v` as-is |
| `refArg` | `NewRefArg(def *ServiceDefinition)` | Resolves the referenced definition's service |
| `typeArg` | `NewTypeArg(typ, slice bool)` | Looks up services by type in scope chain; if `slice` is true, collects all |
| `labelArg` | `NewLabelArg(label, typ, slice bool)` | Looks up services by label in scope chain |
| `flexibleSliceArg` | `NewFlexibleSliceArg(elemType, allowEmpty bool)` | First tries exact slice-type match, then element-type match; used internally by autowiring |
| `compoundArg` | `NewCompoundArg(typ, ...Arg)` | Resolves each sub-arg and builds a typed slice |
| `SlottedArg` | `NewSlottedArg(arg Arg, slot uint)` | Wrapper that pins an arg to a specific slot index |

All arg types implement the `Arg` interface: `Type() reflect.Type` and `String() string`.

#### ArgResolver (`di/arg_resolver.go`)

A package-level singleton `resolver` dispatches validation and resolution to type-specific sub-resolvers. The public surface is three package-level functions:
- `ValidateArg(scope, arg)` — called during the validation compiler pass.
- `ResolveArg(scope, arg)` — called at instantiation time.
- `ResolveArgIDs(scope, arg)` — called during cycle detection to enumerate dependency IDs without instantiation.

#### Scope (`di/scope.go`)

`Scope` is the unit of visibility. Each scope holds:
- `svcs *DefinitionRegistry[*ServiceDefinition]`
- `funs *DefinitionRegistry[*FunctionDefinition]`
- `bindings *orderedmap.OrderedMap[reflect.Type, *InterfaceBinding]` — explicit interface-to-arg mappings
- `instances map[ID]any` — the shared instance cache

Every scope has an optional parent. Lookups ending in `InChain` walk up the parent chain. The `Chain()` iterator yields from child to root.

The root scope is named `"root"`. Child scopes are named after the parent definition's ID string (e.g., `"abc123-..."`). This naming is set in `definition.go`'s `Build` method for both service and function children: `scope.NewChild(b.def.ID().String())`.

All scopes are also registered in `container.scopes` (an ordered map) so the compiler can iterate them via `ContainerBuilder.Scopes()`.

#### DefinitionRegistry

A generic ordered triple-index map (by ID, by type, by label) used inside each `Scope`. The ordering is insertion order. Both `*ServiceDefinition` and `*FunctionDefinition` satisfy the `Definition` constraint.

#### Compiler and CompilerPasses (`di/compiler.go`, `di/compiler_ops.go`)

`Compiler` holds a `Passes` slice. When `Run` is called, passes are sorted (stable by stage then by priority, lower number = earlier) and executed in order.

**Stages** (in execution order):

| Constant | Value | When it runs |
|---|---|---|
| `PreAutomation` | 0 | Before any automatic wiring |
| `Automation` | 1 | Interface binding, autowiring |
| `PreValidation` | 2 | Before validation |
| `Validation` | 3 | Arg validation, cycle detection |
| `PreFinalization` | 4 | Before finalisation |
| `Finalization` | 5 | Eager init |
| `PostFinalization` | 6 | After finalisation |

Built-in passes (from `BasePasses`):
- `"interface binding"` — `Automation` stage — `InterfaceBindingPass`
- `"autowiring"` — `Automation` stage — `autowiringPass`
- `"argument validation"` — `Validation` stage — `argValidationPass`
- `"eager initialization"` — `Finalization` stage — eager init func
- `"cycle validation"` — `Validation` stage — cycle validation func (skippable via config)

`CompilerOp` is the interface that passes wrap:
```go
type CompilerOp interface {
    Run(builder *ContainerBuilder) error
}
```

`CompilerOpFunc` is a function type that implements `CompilerOp` by calling itself.

#### Interface bindings (`di/binding.go`)

`InterfaceBinding` maps a `reflect.Type` (must be an interface) to an `Arg`. Created via `NewInterfaceBinding(iface, boundTo)` — validates that the bound-to type implements the interface. Stored per-scope.

Two creation paths:
1. **Automatic** — `InterfaceBindingPass` discovers implementations and creates bindings on the fly during compilation.
2. **Manual** — the root package's `BindArg`, `BindType`, `BindSlice` helpers create `InterfaceBindingBuilder` objects that are resolved to bindings on the root scope.

#### Lifecycle

```
New() -> Builder
  .Services(...)
  .Functions(...)
  .Bindings(...)
  .CompilerPasses(...)
  .Build()
     |
     1. ParseFactory() for all services (determines type before args are parsed)
     2. Build(rootScope) for all services (builds ArgList, child scopes)
     3. Build(rootScope) for all functions
     4. Build(rootScope) for all bindings
     5. cb.Build() ->
          compiler.Run(builder)
            passes sorted by (stage, priority)
            for each pass: pass.Run(builder)
              - PreAutomation: user passes (e.g., OverrideSvcArg, RemoveSvc)
              - Automation:    InterfaceBindingPass, autowiringPass
              - Validation:    argValidationPass, cycleValidationPass
              - Finalization:  eagerInitPass
          -> *di.Container
```

After `Build()` the `ContainerBuilder` zeroes its internal `container` pointer and sets `built = true`. Calling `Build()` again returns an error.

---

## 3. Source Analysis

### go.mod

**Purpose:** Module manifest.

Module path: `github.com/michalkurzeja/godi/v2`. Go 1.25 required.

Key direct dependencies:
- `github.com/dominikbraun/graph v0.23.0` — DAG library used for cycle detection.
- `github.com/elliotchance/orderedmap/v2 v2.7.0` — insertion-order-preserving map used for definition registries and bindings.
- `github.com/google/uuid v1.6.0` — generates UUIDs for service/function IDs.
- `github.com/samber/lo v1.49.1` — generic helpers (map, filter, repeat-by, etc.).
- `github.com/stretchr/testify v1.10.0` — testing assertions and mocks.
- `golang.org/x/exp v0.0.0-...` — `constraints.Ordered` used in `util.SortedAsc`.
- `github.com/vektra/mockery/v3` — code generation tool (listed as `tool`, not a runtime dependency).

---

### README.md

**Purpose:** User-facing documentation and tutorial.

Describes all major concepts (service, factory, dependency, container, autowiring), shows usage examples, and explains all builder options. The README is accurate with respect to the source with one minor inconsistency noted in Section 4.

---

### di.go

**Package:** `di` (root)

**Purpose:** Defines the public `Container` interface and provides typed generic accessor functions over it.

#### Exported types

```go
type Container interface {
    HasService(id ID) bool
    GetService(id ID) (any, error)
    GetServices(ids ...ID) (svcs []any, err error)
    GetServicesIDsByType(typ reflect.Type) []ID
    GetServicesByType(typ reflect.Type) ([]any, error)
    GetServicesIDsByLabel(label Label) []ID
    GetServicesByLabel(label Label) ([]any, error)
    HasFunction(id ID) bool
    ExecuteFunction(id ID) ([]any, error)
    ExecuteFunctions(ids ...ID) (results [][]any, err error)
    GetFunctionsIDsByType(typ reflect.Type) []ID
    ExecuteFunctionsByType(typ reflect.Type) ([][]any, error)
    GetFunctionsIDsByLabel(label Label) []ID
    ExecuteFunctionsByLabel(label Label) ([][]any, error)
    Print(w io.Writer)
}
```

This interface is satisfied by `*di.Container`. It is also mockable (see `mocks/container.go`).

#### Exported functions (generic)

```go
func SvcByRef[T any](c Container, ref SvcReference) (T, error)
```
Returns the service pointed to by `ref`, type-asserting to `T`. Returns an error if the reference is empty, the service is not found (nil from container), or the type assertion fails. Note: checking for `nil` from `GetService` is the "not found" sentinel here because the inner `Scope.GetService` returns `(nil, nil)` when the ID is not registered in that scope. The container-level lookup only searches the root scope, so if a service was registered in a child scope only, `GetService` returns `nil, nil` and this function wraps it as "not found".

```go
func SvcByType[T any](c Container) (T, error)
```
Errors if 0 or more than 1 service of type T is found.

```go
func SvcsByType[T any](c Container) ([]T, error)
```
Returns all services of type T. Never returns an error for "none found" — returns an empty slice.

```go
func SvcByLabel[T any](c Container, label Label) (T, error)
```
Errors if 0 or more than 1 service with label is found.

```go
func SvcsByLabel[T any](c Container, label Label) ([]T, error)
```
Returns all services with label. No error for empty result.

```go
func ExecByRef(c Container, ref FuncReference) ([]any, error)
```
Executes the function pointed to by `ref`. Returns an error if reference is empty.

```go
func ExecByType[T any](c Container) ([]any, error)
```
Finds a function by the type of the underlying Go function (e.g., `func() *MySvc`). Errors if 0 or >1 found.

```go
func ExecAllByType[T any](c Container) ([][]any, error)
```
Executes all functions of type T.

```go
func ExecByLabel(c Container, label Label) ([]any, error)
```
Executes a single function by label. Errors if 0 or >1 found.

```go
func ExecAllByLabel(c Container, label Label) ([][]any, error)
```
Executes all functions with label.

#### Unexported helpers

```go
func castTo[T any](svcAny any) (T, error)
func castSliceTo[T any](svcsAny []any) ([]T, error)
```
Type assertion with error reporting using fully-qualified type names from `util.Signature`.

---

### arg.go

**Package:** `di` (root)

**Purpose:** Public argument builder. Provides the `Arg`, `Val`, `Ref`, `Type`, `SliceOf`, and `Compound` builder functions.

#### Exported types

```go
type ArgBuilder struct {
    newArg  func() (di.Arg, error)
    slot    uint
    slotSet bool
}
```

`ArgBuilder` is the user-facing argument configurator. It lazily constructs a `di.Arg` when `Build()` is called.

```go
func (b *ArgBuilder) Slot(n uint) *ArgBuilder
func (b *ArgBuilder) Build() (di.Arg, error)
```

`Slot(n)` pins this argument to position `n` in the function signature. When built, if `slotSet` is true, the resulting `di.Arg` is wrapped in a `di.SlottedArg`.

#### Exported functions

```go
func Arg(v any) *ArgBuilder
```
Smart constructor: if `v` is already an `*ArgBuilder`, returns it; if it is a `*SvcReference`, returns `Ref(v)`; otherwise returns `Val(v)`. This is the converter used by `buildArgs()` when processing raw variadic args passed to `Svc()` and `Func()`.

```go
func Ref(ref *SvcReference) *ArgBuilder
```
Creates an arg builder that will produce a `di.refArg` pointing to `ref.def`. The reference is captured by pointer at builder construction time — the actual `def` pointer inside the reference may be nil when the builder is created (deferred binding). The `def` is only accessed when `Build()` is called, which happens during `Builder.Build()`. Since `ParseFactory` runs for all services first, and `Build` runs second, forward references where `Bind(&ref)` is called after `Ref(&ref)` work correctly.

```go
func Val(v any) *ArgBuilder
```
Produces a `di.literalArg` wrapping `v`.

```go
func Type[T any](label ...Label) *ArgBuilder
```
If a label is provided (only the last one is used if multiple), produces a `di.labelArg` for that label with `slice=false`. Otherwise produces a `di.typeArg` for `T` with `slice=false`.

```go
func SliceOf[T any](label ...Label) *ArgBuilder
```
Same as `Type` but with `slice=true`. Used to collect multiple services of type T into a `[]T`.

```go
func Compound[T any](builders ...*ArgBuilder) *ArgBuilder
```
Resolves each sub-builder and creates a `di.compoundArg` of type `T`. If no builders are provided, `NewCompoundArg` returns `nil, nil` (no-op). All sub-arg types must be assignable to `T`.

---

### binding.go

**Package:** `di` (root)

**Purpose:** Public interface binding builders.

#### Exported types

```go
type InterfaceBindingBuilder struct {
    typ    reflect.Type
    bindTo *ArgBuilder
}
func (b *InterfaceBindingBuilder) Build(scope *di.Scope) error
```

`Build` creates a `di.InterfaceBinding` and adds it to the given scope. It will fail at build time if `bindTo`'s type does not implement the interface.

#### Exported functions

```go
func BindArg[Iface any](bindTo *ArgBuilder) *InterfaceBindingBuilder
```
Generic binding from interface `Iface` to any argument expression.

```go
func BindType[Iface, To any]() *InterfaceBindingBuilder
```
Sugar: binds `Iface` to the service of type `To` (uses `Type[To]()`).

```go
func BindSlice[Iface, To any]() *InterfaceBindingBuilder
```
Binds `Iface` (or `[]Iface`) to a `Compound[Iface](Type[To]())`. This is useful when `Iface` needs to resolve as a slice of `To`. Note: the compound wraps a single `Type[To]()` arg; this means it resolves to a single-element compound of `To`-typed services. When used to inject a `[]Iface` parameter, this effectively collects all services of type `To` into a slice (because `Type[To]()` resolves all matching services when the slot is a slice). The compound's element type is `Iface` so the resulting slice type is `[]Iface`.

---

### builder.go

**Package:** `di` (root)

**Purpose:** Top-level entry point. `New()` and `Builder`.

#### Exported types and functions

```go
func New(opts ...BuilderOption) *Builder
```
Creates a `Builder` wrapping a fresh `di.ContainerBuilder`. The recommended entry point to the library.

```go
type Builder struct { ... }
func (b *Builder) Services(services ...*ServiceDefinitionBuilder) *Builder
func (b *Builder) Functions(functions ...*FunctionDefinitionBuilder) *Builder
func (b *Builder) Bindings(bindings ...*InterfaceBindingBuilder) *Builder
func (b *Builder) CompilerPasses(passes ...*di.CompilerPass) *Builder
func (b *Builder) Build() (Container, error)
```

The `Build()` method:
1. Calls `ParseFactory()` on all services (two-pass: first parse, then build — this allows forward references to be resolved since definitions are created before args are processed).
2. Calls `Build(rootScope)` on all services.
3. Calls `Build(rootScope)` on all functions.
4. Calls `Build(rootScope)` on all bindings.
5. Adds compiler passes to the compiler.
6. Calls `cb.Build()`.

Errors from steps 1–4 are joined and combined with any compiler error from step 6. The function returns a non-nil `Container` even if there is a partial build error in steps 1–4 (from the `cb.Build()` call returning `nil` on compiler error). Actually: if `cb.Build()` errors, the container returned is nil. If only steps 1–4 error but compilation succeeds, the container is non-nil. In practice, if definitions are malformed (step 1–4 errors), the compiler will also fail (arg validation), so a nil container is the typical outcome.

```go
type BuilderOption func(*di.Config)
func SkipCycleValidation() BuilderOption
```
The only built-in `BuilderOption`. Sets `Config.CompilerConfig.SkipCycleValidation = true`. Cycle validation uses a DAG and is `O(V + E)` — skipping it is a performance optimisation for containers with many services.

---

### defaults.go

**Package:** `di` (root)

**Purpose:** Package-level functions to change the global defaults that are applied when `di.NewServiceDefinition` or `di.NewFunctionDefinition` is called.

```go
func SetDefaultLazy()        // di.SetDefaultLazy(true)
func SetDefaultEager()       // di.SetDefaultLazy(false)
func SetDefaultShared()      // di.SetDefaultShared(true)
func SetDefaultNotShared()   // di.SetDefaultShared(false)
func SetDefaultAutowired()   // di.SetDefaultAutowired(true)
func SetDefaultNotAutowired()// di.SetDefaultAutowired(false)
```

These functions modify package-level variables in `di/definition.go`:
```go
var defaultLazy      = true
var defaultShared    = true
var defaultAutowired = true
```

**Critical behaviour:** The defaults are applied at the moment `di.NewServiceDefinition` / `di.NewFunctionDefinition` is called — which happens inside `Svc()` / `Func()`. If you call `SetDefaultEager()` after some `Svc()` calls, those already-created definitions retain their previous defaults. The README warns about this explicitly.

These variables are package-level mutable globals and are **not safe for concurrent use**. They should only be changed once at program startup, before any `Svc()`/`Func()` calls.

---

### definition.go

**Package:** `di` (root)

**Purpose:** Defines public-facing `SvcReference`, `FuncReference`, `ServiceDefinitionBuilder`, and `FunctionDefinitionBuilder`.

#### Exported types

```go
type ID = di.ID       // type alias
type Label = di.Label // type alias
```

```go
type SvcReference struct {
    def *di.ServiceDefinition
}
func (r SvcReference) SvcID() ID
func (r SvcReference) IsEmpty() bool
func (r SvcReference) String() string
```

A `SvcReference` is a zero-value type (zero value is an empty reference with `def == nil`). Calling `Bind(&ref)` on a builder sets `ref.def` to the underlying definition. The reference is usable as an `Arg` shorthand (`Ref(&ref)`) or directly passed as a raw arg to `Svc()` (the `Arg()` function recognises `*SvcReference`).

```go
type FuncReference struct {
    def *di.FunctionDefinition
}
func (r FuncReference) FuncID() ID
func (r FuncReference) IsEmpty() bool
func (r FuncReference) String() string
```

Same pattern as `SvcReference` but for functions.

```go
type ServiceDefinitionBuilder struct { ... }
```

Fluent builder for `di.ServiceDefinition`.

```go
func Svc(factory any, args ...any) *ServiceDefinitionBuilder
```
Creates a builder wrapping a fresh `di.NewServiceDefinition(nil)`. The factory and args are stored but not parsed until `ParseFactory()` is called.

```go
func SvcVal[T any](svc T) *ServiceDefinitionBuilder
```
Sugar: wraps `svc` in a zero-arg factory `func() T { return svc }` and delegates to `Svc`.

Builder methods:
```go
func (b *ServiceDefinitionBuilder) Bind(ref *SvcReference) *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) MethodCall(method any, args ...any) *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Labels(labels ...Label) *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Lazy() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Eager() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Shared() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) NotShared() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Autowired() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) NotAutowired() *ServiceDefinitionBuilder
func (b *ServiceDefinitionBuilder) Children(services ...*ServiceDefinitionBuilder) *ServiceDefinitionBuilder
```

```go
func (b *ServiceDefinitionBuilder) ParseFactory() (joinedErrs error)
```
Parses the factory function to determine `Creates()` type. Must be called before `Build`. Also recursively calls `ParseFactory` on children. Sets `factoryParsed = true`.

```go
func (b *ServiceDefinitionBuilder) Build(scope *di.Scope) (joinedErrs error)
```
Builds the arg list, creates a child scope if children are present, recursively builds children into the child scope, sets the definition's scope, and registers it in the scope. The child scope name is `b.def.ID().String()`.

```go
func (b *ServiceDefinitionBuilder) ParseAndBuild(scope *di.Scope) error
```
Convenience: calls `ParseFactory` then `Build`.

**Key subtlety about `MethodCall`:** When `Build` processes method calls, the receiver is explicitly set as slot 0 via `NewRefArg(b.def)` + `NewSlottedArg(receiver, 0)`. The method itself is parsed as a function that takes the receiver type as first parameter (Go method values are receiver-parameterised). This means the `Slot` at index 0 of a method's `ArgList` is always the receiver.

**Duplicate method calls:** If the same method is called multiple times with different args, only the last one survives. The `methodCalls` map is keyed by method name (via `call.Name()`), so `AddMethodCalls` silently overwrites an existing entry. The test "duplicated method call replaces the previous one" confirms this.

```go
type FunctionDefinitionBuilder struct { ... }
func Func(fn any, args ...any) *FunctionDefinitionBuilder
```
Creates a builder for a `di.FunctionDefinition`. Unlike service definitions, `Func` builds the arg list lazily inside `setFunc` closure (called by `Build`). There is no `ParseFactory` step.

Builder methods:
```go
func (b *FunctionDefinitionBuilder) Bind(ref *FuncReference) *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) Labels(labels ...Label) *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) Lazy() *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) Eager() *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) Autowired() *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) NotAutowired() *FunctionDefinitionBuilder
func (b *FunctionDefinitionBuilder) Children(services ...*ServiceDefinitionBuilder) *FunctionDefinitionBuilder
```

Note: `FunctionDefinitionBuilder.Children` accepts service builders (not function builders) because only services can be children.

The child scope name for function children is `b.def.ID().String()`, the same as for service children.

---

### di/arg.go

**Package:** `di`

**Purpose:** Core argument model: `Arg` interface, all concrete arg types, `Slot`, `Slots`, `ArgList`.

#### Exported interface

```go
type Arg interface {
    fmt.Stringer
    Type() reflect.Type
}
```

#### Exported arg constructors

```go
func NewLiteralArg(v any) Arg
func NewZeroArg(typ reflect.Type) Arg   // creates literalArg with reflect.Zero value; used for unfilled variadic slots
func NewRefArg(def *ServiceDefinition) (Arg, error)  // errors if def is nil
func NewTypeArg(typ reflect.Type, slice bool) Arg
func NewLabelArg(label Label, typ reflect.Type, slice bool) Arg
func NewFlexibleSliceArg(elemType reflect.Type, allowEmpty bool) Arg
func NewCompoundArg(typ reflect.Type, args ...Arg) (Arg, error) // nil, nil if no args; errors if any sub-arg is not assignable to typ
func NewSlottedArg(arg Arg, slot uint) *SlottedArg
```

#### Exported types

```go
type SlottedArg struct {
    Arg        // embedded
    slot uint
}
func (a *SlottedArg) Slot() uint
```

```go
type Slots []*Slot
func (slots Slots) Args() []Arg  // extracts Arg from each Slot
```

```go
type Slot struct { ... }
func NewSlot(typ reflect.Type, i uint, variadic bool) *Slot
func (s *Slot) IsSlice() bool
func (s *Slot) IsVariadicSlice() bool
func (s *Slot) Type() reflect.Type
func (s *Slot) ElemType() reflect.Type  // panics if not a slice slot
func (s *Slot) FillableBy(arg Arg) bool // SettableBy || AppendableBy
func (s *Slot) SettableBy(arg Arg) bool // arg.Type().AssignableTo(slot.Type())
func (s *Slot) AppendableBy(arg Arg) bool // slot.IsSlice() && arg.Type().AssignableTo(slot.ElemType())
func (s *Slot) Fill(arg Arg) error
func (s *Slot) Set(arg Arg) error
func (s *Slot) Append(args ...Arg) error
func (s *Slot) Arg() Arg   // returns compound arg for slice slots with appended elements; returns s.arg otherwise
func (s *Slot) IsFilled() bool  // IsSet() || IsAppended()
func (s *Slot) IsSet() bool     // s.arg != nil
func (s *Slot) IsAppended() bool // len(s.args) > 0
func (s *Slot) Index() uint
```

**Key subtlety:** A variadic slot can be filled via `Append`, which accumulates individual element args. When `Arg()` is called on such a slot, a `compoundArg` is synthesised from the accumulated elements. This compound arg is then resolved at instantiation time by `compoundArgResolver`, which calls each sub-arg's resolver and builds a typed slice.

```go
type ArgList struct { ... }
func NewArgList(fnType reflect.Type) *ArgList
func (l *ArgList) Slots() Slots
func (l *ArgList) Assign(arg Arg) error             // routes slotted args first, then positional
func (l *ArgList) ValidateAndCollect() ([]Arg, error) // validates all required slots are filled; fills empty variadic with zero arg
func (l *ArgList) Validate() error
func (l *ArgList) IsVariadic() bool
func (l *ArgList) FillSlot(arg *SlottedArg) error   // Fill (set or append)
func (l *ArgList) SetSlot(arg *SlottedArg) error    // Set only
func (l *ArgList) AppendSlot(arg *SlottedArg) error // Append only
```

`Assign` processes in two phases: slotted args first (order matters for slot collision avoidance), then unslotted. For unslotted, it iterates left-to-right over slots and fills the first `FillableBy` and not-yet-`IsSet` slot. For slice slots it uses `Fill` which may `Append` rather than `Set`, allowing multiple elements to accumulate.

`ValidateAndCollect` requires all non-variadic slots to be filled. For the variadic slot, if it has not been set, a `NewZeroArg` for the variadic slice type is used (this represents `nil` / empty slice, enabling valid `CallSlice` invocation).

---

### di/arg_resolver.go

**Package:** `di`

**Purpose:** Implements runtime resolution and compile-time validation of all `Arg` types.

#### Package-level API

```go
var resolver = NewArgResolver()

func ValidateArg(scope *Scope, arg Arg) error
func ResolveArg(scope *Scope, arg Arg) (any, error)
func ResolveArgIDs(scope *Scope, arg Arg) []ID
```

#### ArgResolver struct

```go
type ArgResolver struct {
    literalArgResolver       *literalArgResolver
    refArgResolver           *refArgResolver
    typeArgResolver          *typeArgResolver
    labelArgResolver         *labelArgResolver
    flexibleSliceArgResolver *flexibleSliceArgResolver
    compoundArgResolver      *compoundArgResolver
}
```

Each sub-resolver is a zero-field struct except `typeArgResolver`, `labelArgResolver`, `flexibleSliceArgResolver`, and `compoundArgResolver` which hold a back-pointer to `*ArgResolver` for recursive resolution.

The `ArgResolver` interface (mocked in `di/mocks/arg_resolver.go`) has three methods: `Validate(scope, arg)`, `Resolve(scope, arg)`, `ResolveIDs(scope, arg)`.

**Resolution rules per arg type:**

- **literalArg:** Always valid. Returns `a.v`.
- **refArg:** Valid if `scope.HasServiceInChain(a.def.ID())`. Resolves by `scope.GetServiceInChain(a.def.ID())`.
- **typeArg:** Checks for an interface binding first (`scope.GetBoundArgInChain(a.typ)`). If found, delegates to the bound arg's resolver. Otherwise looks up services by type in chain. For `slice=false`, errors if count != 1. For `slice=true`, builds a typed slice.
- **labelArg:** Same pattern but uses `GetServicesIDsByLabelInChain`. For `slice=false`, also validates that the resolved value's type matches `a.Type()`.
- **flexibleSliceArg:** First tries exact slice-type match (including binding check). If nothing found, tries element-type match. If `allowEmpty=true`, an empty result is valid (returns an empty slice). This is used by autowiring for variadic slots.
- **compoundArg:** Validates/resolves each sub-arg and assembles a typed slice via `convertSlice`.

**`ResolveIDs`:** Returns the IDs of services that this arg depends on. Used by cycle detection (no instantiation). For `literalArg` returns nil. For `refArg` returns `[def.ID()]`. For type/label/flexible args returns the IDs found by the chain lookup.

---

### di/binding.go

**Package:** `di`

**Purpose:** `InterfaceBinding` type.

```go
type InterfaceBinding struct {
    ifaceTyp reflect.Type
    boundTo  Arg
}
func NewInterfaceBinding(iface reflect.Type, boundTo Arg) (*InterfaceBinding, error)
func (b *InterfaceBinding) Interface() reflect.Type
func (b *InterfaceBinding) BoundTo() Arg
```

Validation at creation: `iface.Kind()` must be `reflect.Interface`; `boundTo.Type().Implements(iface)` must be true. This means attempting to create a binding where the arg type does not implement the interface fails immediately.

---

### di/compiler.go

**Package:** `di`

**Purpose:** Compiler infrastructure: stages, pass ordering, runner.

#### Exported types

```go
type CompilerPass struct { ... }
func NewCompilerPass(name string, stage CompilerStage, op CompilerOp) *CompilerPass
func (p *CompilerPass) WithPriority(priority int) *CompilerPass
func (p *CompilerPass) Run(builder *ContainerBuilder) error
func (p *CompilerPass) String() string  // returns the name
```

Default priority is 0. Within a stage, lower priority value runs first. If two passes have the same stage and priority, insertion order is preserved (the sort is not stable by spec, but since `cmp.Compare` returns 0 for equal priorities, the sort does not swap them; however, `slices.SortFunc` is not guaranteed stable in Go, so equal-priority same-stage passes may execute in arbitrary order).

```go
type CompilerOp interface {
    Run(builder *ContainerBuilder) error
}

type CompilerOpFunc func(builder *ContainerBuilder) error
func (fn CompilerOpFunc) Run(builder *ContainerBuilder) error
```

`CompilerOpFunc` satisfies `CompilerOp` directly — no wrapping needed. It is used by `NewCycleValidationPass` and `NewEagerInitPass`.

```go
type CompilerStage uint8
const (
    PreAutomation   CompilerStage = iota  // 0
    Automation                            // 1
    PreValidation                         // 2
    Validation                            // 3
    PreFinalization                        // 4
    Finalization                          // 5
    PostFinalization                       // 6
    compilerPassStageCount                // 7 (unexported sentinel)
)
```

```go
type Passes []*CompilerPass
func BasePasses(skipCycleValidation bool) Passes
func (passes Passes) sort()  // unexported; called by Compiler.Run
```

```go
type Compiler struct { ... }
func NewCompiler(conf CompilerConfig) *Compiler
func (c *Compiler) AddPass(pass *CompilerPass)
func (c *Compiler) Run(builder *ContainerBuilder) error
```

```go
type CompilerConfig struct {
    SkipCycleValidation bool
}
func NewCompilerConfig() CompilerConfig
```

---

### di/compiler_ops.go

**Package:** `di`

**Purpose:** All built-in compiler pass implementations.

#### InterfaceBindingPass (Automation stage)

```go
type InterfaceBindingPass struct{}
func NewInterfaceBindingPass() CompilerOp
func (p *InterfaceBindingPass) Run(builder *ContainerBuilder) error
```

Iterates all service definitions (factories + method calls) and all function definitions, checking each unfilled slot. For each slot whose type is an interface (or whose element type is an interface for slice slots), it:
1. Skips if the slot is already filled.
2. Skips if there is an existing binding for the interface in the scope chain.
3. Calls `findImplementations` to find all services in the scope chain that implement the interface and are not the owning service itself.
4. If no implementations: skips (leaves for autowiring or validation to handle).
5. If the slot is a slice: creates a `compoundArg` of `refArg`s pointing to all implementations.
6. If the slot is not a slice and there are multiple implementations: returns an error.
7. If the slot is not a slice and there is one implementation: creates a `refArg`.
8. Registers a new `InterfaceBinding` on the scope.

**Important:** The binding is added to the scope (not the slot). The slot remains unfilled — the binding is consulted by `ArgResolver` during resolution and validation. This means the interface binding mechanism bypasses the slot's fill state; the slot appears unfilled to the `argValidationPass` but the `typeArgResolver`/`flexibleSliceArgResolver` find the binding and validate/resolve through it.

Actually — re-reading: the `argValidationPass` calls `ValidateArg(scope, slot.Arg())`. For a slot that has been filled by autowiring (with a `typeArg` or `flexibleSliceArg`), the validator will find the binding and validate through it. For a slot that remains unfilled after both interface binding and autowiring passes, `slot.IsFilled()` returns false and `argValidationPass` emits "argument N is not set". So the flow is: InterfaceBindingPass creates the binding but does NOT fill the slot; the autowiringPass then fills the slot with a `typeArg` / `flexibleSliceArg`; then the validationPass validates by checking the binding.

Wait — that does not match the code. Let's be precise: `InterfaceBindingPass` creates a `di.InterfaceBinding` on the scope but does NOT call `slot.Set(...)` or `slot.Append(...)`. The slot remains unfilled. If autowiring is enabled, `autowiringPass` then runs: for the same interface-typed slot it calls `slot.Fill(NewTypeArg(slot.Type(), false))` (or `FlexibleSliceArg` for slices). The resulting `typeArg` is stored in the slot. When the `typeArgResolver` resolves this `typeArg`, it calls `scope.GetBoundArgInChain(a.typ)`, finds the binding, and resolves through it. For a non-autowired definition, the slot is never filled; the `argValidationPass` reports it as not set.

If autowiring is disabled, manual args must fully cover all params including interface params, or the build fails at validation.

#### autowiringPass (Automation stage)

```go
type autowiringPass struct{}
func NewAutowiringPass() CompilerOp
```

For each autowired service definition, iterates factory slots and each method call's slots. For each unfilled slot:
- If it is a slice (including variadic): fills with `NewFlexibleSliceArg(slot.ElemType(), slot.IsVariadicSlice())`. `allowEmpty=true` for variadic slots, `allowEmpty=false` for non-variadic slice slots.
- Otherwise: fills with `NewTypeArg(slot.Type(), false)`.

Does the same for autowired function definitions.

#### argValidationPass (Validation stage)

```go
type argValidationPass struct{}
func NewArgValidationPass() CompilerOp
```

For each service definition: validates factory args and each method call's args. For each function definition: validates its args.

Validation per arg list: for each slot, if not filled → error "argument N is not set". If filled → call `ValidateArg(scope, slot.Arg())`.

#### NewCycleValidationPass (Validation stage)

Returns a `CompilerOpFunc`. Builds a directed graph (using `github.com/dominikbraun/graph` with `PreventCycles()`) with all service definitions as vertices. Adds edges for each arg's resolved IDs (via `ResolveArgIDs`). If an edge creates a cycle, records an error naming both the dependent and the dependency.

Only factory args are checked (not method calls). This means method-call circular dependencies are not detected — they are the documented workaround for cycles.

#### NewEagerInitPass (Finalization stage)

Returns a `CompilerOpFunc`. For each non-lazy service definition: calls `scope.GetService(def.ID())` to trigger instantiation. For each non-lazy function definition: calls `scope.ExecuteFunction(def.ID())`.

---

### di/config.go

**Package:** `di`

**Purpose:** Top-level `Config` struct and constructor.

```go
type Config struct {
    CompilerConfig
}
func NewConfig() Config
```

`Config` embeds `CompilerConfig`. Currently the only field is `SkipCycleValidation`. The root package's `newConfig` helper applies `BuilderOption` functions to a `Config` value.

---

### di/container.go

**Package:** `di`

**Purpose:** The concrete `*Container` type and the `RootScope` constant.

```go
const RootScope = "root"

type Container struct {
    root   *Scope
    scopes *orderedmap.OrderedMap[string, *Scope]
}
func NewContainer() *Container
```

All public `Container` methods delegate to `c.root`. The `scopes` map is used by `ContainerBuilder.Scopes()` to iterate all scopes for compiler passes.

`GetBindingFor(typ reflect.Type) (Arg, bool)` is an additional method on `*di.Container` not present on the `Container` interface defined in the root package — it is available to consumers that type-assert to `*di.Container` or that use the concrete type in internal code. The root package's `Container` interface does not expose this.

`Print(w io.Writer)` delegates to the `Print` function in `di/print.go`.

---

### di/container_builder.go

**Package:** `di`

**Purpose:** Pre-build mutable state. Provides iterators for compiler passes.

```go
type ContainerBuilder struct {
    container *Container
    compiler  *Compiler
    built     bool
}
func NewContainerBuilder(conf Config) *ContainerBuilder
func (b *ContainerBuilder) RootScope() *Scope
func (b *ContainerBuilder) Scope(name string) (*Scope, bool)
func (b *ContainerBuilder) Scopes() iter.Seq[*Scope]
func (b *ContainerBuilder) ServiceDefinitionsSeq() iter.Seq2[*Scope, *ServiceDefinition]
func (b *ContainerBuilder) FunctionDefinitionsSeq() iter.Seq2[*Scope, *FunctionDefinition]
func (b *ContainerBuilder) Compiler() *Compiler
func (b *ContainerBuilder) Build() (*Container, error)
```

`ServiceDefinitionsSeq` and `FunctionDefinitionsSeq` yield `(*Scope, *Definition)` pairs. The scope yielded is the scope in which the definition resides (not the effective scope). Compiler passes that need the effective scope call `def.EffectiveScope()`.

`Build()` panics (implicit nil pointer dereference) or returns an error if called more than once. Actually: the second call returns `errors.New("container already built")` before touching `c.container`, so it is safe.

After a successful build, `b.container` is set to `nil` to release the reference. The returned `*Container` is the only owner.

---

### di/definition.go

**Package:** `di`

**Purpose:** Core definition types, ID/Label types, and global defaults.

#### ID and Label

```go
type ID string
func NewID() ID           // generates UUID v4 string
func (id ID) String() string

type Label string
func (l Label) String() string
```

IDs are UUID v4 strings. They are generated once per definition at creation time and never change.

#### Global defaults

```go
var defaultLazy      = true
var defaultShared    = true
var defaultAutowired = true

func SetDefaultLazy(b bool)
func SetDefaultShared(b bool)
func SetDefaultAutowired(b bool)
```

These are package-level mutable variables, not concurrency-safe.

#### ServiceDefinition

```go
type ServiceDefinition struct {
    id          ID
    labels      []Label
    factory     *Factory
    methodCalls map[string]*Method   // keyed by method name
    scope       *Scope
    childScope  *Scope
    lazy        bool
    shared      bool
    autowired   bool
}
func NewServiceDefinition(factory *Factory) *ServiceDefinition
```

```go
func (d *ServiceDefinition) ID() ID
func (d *ServiceDefinition) Type() reflect.Type          // delegates to d.factory.Creates()
func (d *ServiceDefinition) Scope() *Scope
func (d *ServiceDefinition) SetScope(scope *Scope) *ServiceDefinition
func (d *ServiceDefinition) ChildScope() *Scope
func (d *ServiceDefinition) SetChildScope(scope *Scope) *ServiceDefinition
func (d *ServiceDefinition) EffectiveScope() *Scope      // childScope if set, else scope
func (d *ServiceDefinition) Factory() *Factory
func (d *ServiceDefinition) SetFactory(factory *Factory) *ServiceDefinition
func (d *ServiceDefinition) MethodCalls() []*Method      // sorted by method name (ascending)
func (d *ServiceDefinition) SetMethodCalls(methodCalls ...*Method) *ServiceDefinition
func (d *ServiceDefinition) AddMethodCalls(methodCalls ...*Method) *ServiceDefinition  // overwrites on name collision
func (d *ServiceDefinition) RemoveMethodCalls(names ...string) *ServiceDefinition
func (d *ServiceDefinition) Labels() []Label
func (d *ServiceDefinition) SetLabels(labels ...Label) *ServiceDefinition
func (d *ServiceDefinition) AddLabels(labels ...Label) *ServiceDefinition
func (d *ServiceDefinition) RemoveLabels(labels ...Label) *ServiceDefinition
func (d *ServiceDefinition) IsLazy() bool
func (d *ServiceDefinition) SetLazy(lazy bool) *ServiceDefinition
func (d *ServiceDefinition) IsShared() bool
func (d *ServiceDefinition) SetShared(shared bool) *ServiceDefinition
func (d *ServiceDefinition) IsAutowired() bool
func (d *ServiceDefinition) SetAutowired(autowired bool) *ServiceDefinition
func (d *ServiceDefinition) FactoryName() string
func (d *ServiceDefinition) String() string   // "type (label1, label2)" or "service" if no factory
```

The `String()` method is used in error messages throughout the library. It uses `util.Signature(d.Type())` which produces fully-qualified type paths like `github.com/myapp.(*MySvc)`.

`EffectiveScope()` is the key to understanding child scope resolution. When a service has a child scope, all arg resolution for that service's factory and method calls happens inside the child scope, which has visibility to both the child services and all parent scope services.

#### FunctionDefinition

```go
type FunctionDefinition struct {
    id         ID
    function   *Func
    labels     []Label
    scope      *Scope
    childScope *Scope
    lazy       bool
    autowired  bool  // no "shared" — functions are always re-executed
}
func NewFunctionDefinition(function *Func) *FunctionDefinition
```

Methods mirror `ServiceDefinition` except there is no `IsShared` / `SetShared` and no `MethodCalls`. The `Type()` method returns the Go function type (`reflect.Func`), not a return value type.

---

### di/function.go

**Package:** `di`

**Purpose:** `Factory`, `Method`, and `Func` — the executable units of the DI system.

#### Func

```go
type Func struct {
    fn      reflect.Value
    args    *ArgList
    returns []reflect.Type
    name    string
}
func NewFunc(fn reflect.Value, args ...Arg) (*Func, error)
func (f *Func) Execute(scope *Scope) ([]reflect.Value, error)
func (f *Func) Args() *ArgList
func (f *Func) AddArgs(args ...Arg) error
func (f *Func) Type() reflect.Type    // fn.Type() — the reflect.Func type
func (f *Func) Name() string          // fully qualified function name via util.FuncName
func (f *Func) String() string
```

`Execute` resolves all args from the scope, then calls either `fn.Call(resolvedArgs)` or `fn.CallSlice(resolvedArgs)` depending on `args.IsVariadic()`. The last resolved arg for a variadic function must be a slice — this is guaranteed because variadic slots are filled with `flexibleSliceArg` or `compoundArg`, both of which resolve to slice values.

`AddArgs` processes slotted args first, then unslotted. This two-pass logic ensures that explicit slot assignments take priority over positional assignments.

The `name` field is set by `util.FuncName(fn)` which uses `runtime.FuncForPC`. Anonymous functions get names like `main.TestFoo.func1`.

#### Factory

```go
type Factory struct {
    fn           *Func
    returnedType reflect.Type
    returnsErr   bool
}
func NewFactory(fn any, args ...Arg) (*Factory, error)
func (f *Factory) Execute(scope *Scope) (any, error)
func (f *Factory) Args() *ArgList
func (f *Factory) AddArgs(args ...Arg) error
func (f *Factory) Creates() reflect.Type   // first return value type
func (f *Factory) Name() string
func (f *Factory) String() string
```

Validation in `NewFactory`:
- Must be a func.
- Must have 1 or 2 return values.
- If 2 return values, the second must be assignable to `error`.
- Function kind check is `reflect.Func`.

`Execute` calls the underlying `Func`, then checks if the second return value (if present) is non-nil error. It returns the first return value as `any`.

#### Method

```go
type Method struct {
    fn         *Func
    returnsErr bool
}
func NewMethod(fn any, receiver Arg, args ...Arg) (*Method, error)
func (m *Method) Execute(scope *Scope) error
func (m *Method) Args() *ArgList
func (m *Method) AddArgs(args ...Arg) error
func (m *Method) Name() string
func (m *Method) String() string
```

`NewMethod` validates:
- The short name of `fn` must exist as a method on `receiver.Type()` (via `receiver.Type().MethodByName`).
- At most 1 return value (must be `error` if present).

The receiver arg is injected as slot 0 via `NewSlottedArg(receiver, 0)`. This means `m.Args().Slots()[0]` is always the receiver.

In `MethodCall` (the builder method in `definition.go`), the receiver is `di.NewRefArg(b.def)`, which resolves to the already-instantiated service. The method is executed after the factory, so the service instance is available in the scope's instance cache.

---

### di/print.go

**Package:** `di`

**Purpose:** Debug printer for the container.

```go
func Print(s *Scope, w io.Writer)
```

Outputs a human-readable representation of a scope's bindings, services (with factory, flags, args), and functions. Only prints the given scope, not the chain.

Slots in method calls: the receiver slot (index 0) is skipped in the output — it iterates `method.Args().Slots()[1:]`.

Interface binding in output: for each slot arg, it checks `s.GetBoundArg(arg.Type())` and if a binding exists, prints the bound arg's string representation instead of the slot's.

This function is exposed on the public `Container` interface as `Print(w io.Writer)`.

---

### di/scope.go

**Package:** `di`

**Purpose:** `Scope`, `DefinitionRegistry`, and the `Definition` interface.

#### Scope

```go
func NewScope(name string, container *Container, parent *Scope) *Scope
```

Creates a scope and immediately registers it in `container.scopes`. This means that all scopes (root and all child scopes) are globally enumerable via the container.

```go
type Scope struct {
    name      string
    container *Container
    parent    *Scope
    svcs      *DefinitionRegistry[*ServiceDefinition]
    funs      *DefinitionRegistry[*FunctionDefinition]
    bindings  *orderedmap.OrderedMap[reflect.Type, *InterfaceBinding]
    instances map[ID]any
}
```

The `instances` map caches service instances. It is keyed by definition ID, not by type. Only shared services are cached here (see `instantiate`).

Service instantiation (`getServiceInstance` → `instantiate`):
1. Check `instances[def.ID()]` — return cached if found.
2. Call `def.factory.Execute(def.EffectiveScope())`.
3. If `def.shared`, store result in `instances`.
4. Execute all method calls via `method.Execute(def.EffectiveScope())`.
5. Return the service.

Note: the instance is stored before method calls are executed. This means if a method call triggers resolution of the same service (possible in pathological configurations), it would get the partially-constructed instance. This is the intended use for breaking circular dependencies via method calls.

```go
func (s *Scope) Chain() iter.Seq[*Scope]  // from this scope up to root
func (s *Scope) NewChild(name string) *Scope
func (s *Scope) Parent() *Scope
```

Methods follow a pattern: `HasX`, `GetX`, `HasXInChain`, `GetXInChain` for services, functions, bindings. The `InChain` variants walk the chain until a match is found.

Mutation methods: `AddServiceDefinitions`, `RemoveServiceDefinitions`, `ClearServiceDefinitions`, `AddFunctionDefinitions`, `RemoveFunctionDefinitions`, `AddBindings`, `SetBindings`, `RemoveBindings`.

Important export for compiler pass authors:
```go
func (s *Scope) GetServiceDefinitions() []*ServiceDefinition
func (s *Scope) GetServiceDefinitionsInChain() []*ServiceDefinition
func (s *Scope) GetServiceDefinitionsByType(typ reflect.Type) []*ServiceDefinition
func (s *Scope) GetServiceDefinitionsByTypeInChain(typ reflect.Type) []*ServiceDefinition
func (s *Scope) GetServiceDefinitionsByLabel(label Label) []*ServiceDefinition
func (s *Scope) GetServiceDefinitionsByLabelInChain(label Label) []*ServiceDefinition
func (s *Scope) GetServiceDefinition(id ID) (*ServiceDefinition, bool)
func (s *Scope) GetServiceDefinitionInChain(id ID) (*ServiceDefinition, bool)
// same pattern for FunctionDefinition
```

#### Definition interface

```go
type Definition interface {
    ID() ID
    Type() reflect.Type
    Labels() []Label
}
```

#### DefinitionRegistry

```go
type DefinitionRegistry[Def Definition] struct {
    byID    *orderedmap.OrderedMap[ID, Def]
    byType  *orderedmap.OrderedMap[reflect.Type, []Def]
    byLabel *orderedmap.OrderedMap[Label, []Def]
}
func NewDefinitionRegistry[Def Definition]() *DefinitionRegistry[Def]
func (r *DefinitionRegistry[Def]) Add(defs ...Def)
func (r *DefinitionRegistry[Def]) Remove(ids ...ID)
func (r *DefinitionRegistry[Def]) Clear()
func (r *DefinitionRegistry[Def]) Contains(id ID) bool
func (r *DefinitionRegistry[Def]) Get(id ID) (Def, bool)
func (r *DefinitionRegistry[Def]) GetByIDs(ids []ID) []Def
func (r *DefinitionRegistry[Def]) GetIDsByType(typ reflect.Type) []ID
func (r *DefinitionRegistry[Def]) GetByType(typ reflect.Type) []Def
func (r *DefinitionRegistry[Def]) GetIDsByLabel(label Label) []ID
func (r *DefinitionRegistry[Def]) GetByLabel(label Label) []Def
func (r *DefinitionRegistry[Def]) Seq() iter.Seq[Def]
func (r *DefinitionRegistry[Def]) GetAll() []Def
func (r *DefinitionRegistry[Def]) Len() int
```

`Remove` correctly cleans up all three indices. The `byType` and `byLabel` slices use `slices.DeleteFunc` to remove the specific definition while preserving others of the same type/label.

`Add` appends to the by-type and by-label slices, so registration order is preserved within a type group or label group.

---

### extras/override_arg.go

**Package:** `extras`

**Purpose:** Provides compiler passes for overriding factory/function arguments.

```go
func OverrideSvcArg(ref godi.SvcReference, slotIdx uint, arg any) *di.CompilerPass
func OverrideFuncArg(ref godi.FuncReference, slotIdx uint, arg any) *di.CompilerPass
```

Both run at `PreAutomation` stage, so they execute before autowiring. This means they can:
- Override a manually-set arg from the original `Svc()`/`Func()` call.
- Set an arg that would otherwise be autowired (the override fills the slot before autowiring, which skips already-filled slots).

Both use `def.Factory().Args().SetSlot(di.NewSlottedArg(a, slotIdx))` (or `def.Func().Args().SetSlot(...)`) which calls the `Set` operation on the slot, not `Fill`. The difference: `Set` will fail if the arg's type is not directly assignable to the slot type, whereas `Fill` would also try `Append` for slices.

The `arg` parameter is processed through `godi.Arg(arg).Build()`, so it accepts raw values, `*godi.ArgBuilder`, or `*godi.SvcReference`.

Both functions look up the definition only in `builder.RootScope()`, not in child scopes. Services/functions registered in child scopes (i.e., children of other services) cannot be modified via these helpers.

---

### extras/remove.go

**Package:** `extras`

**Purpose:** Provides compiler passes for removing services and functions.

```go
func RemoveSvc(ref *godi.SvcReference) *di.CompilerPass
func RemoveFunc(ref *godi.FuncReference) *di.CompilerPass
```

Note: both take a *pointer* to the reference (unlike `OverrideSvcArg` which takes a value). This is because the reference may be populated lazily.

Both run at `PreAutomation` and call `builder.RootScope().RemoveServiceDefinitions(ref.SvcID())` or `RemoveFunctionDefinitions(ref.FuncID())`.

Only removes from the root scope. Child-scope services cannot be removed this way.

After removal, the definition is no longer enumerable by compiler passes. Compilation continues without it.

---

### internal/errorsx/errors.go

**Package:** `errorsx`

**Purpose:** Internal error wrapping utility.

```go
func Wrap(err error, msg string) error
func Wrapf(err error, format string, a ...any) error
```

Creates a `*wrappedError` that implements `Error() string` as `msg + ": " + err.Error()` and `Unwrap() error`. This is the standard contextual wrapping used throughout the library to build hierarchical error messages like:
```
compilation failed: compiler pass (argument validation) returned an error: invalid service ...: invalid factory ...: argument 0 is not set
```

---

### internal/iterx/iterx.go

**Package:** `iterx`

**Purpose:** Utilities for the Go 1.23 `iter` package.

```go
func Values[K, V any](seq iter.Seq2[K, V]) iter.Seq[V]
func Collect[V any](seq iter.Seq[V]) []V
```

`Values` drops the keys from a key-value iterator. Used to extract scope values from `orderedmap.Iterator()`.

`Collect` materialises an iterator into a slice.

---

### internal/util/util.go

**Package:** `util`

**Purpose:** Reflection utilities and generic sorting.

```go
func Signature(typ reflect.Type) string
```
Returns a fully-qualified type name. For pointer types, produces `pkg/path.(*TypeName)`. For types without a package path (built-in types, anonymous types), falls back to `typ.String()`. For `nil`, returns `"<nil>"`. There is commented-out code that would produce function signatures — currently disabled.

```go
func FuncName(val reflect.Value) string
func FuncNameShort(val reflect.Value) string
```
`FuncName` uses `runtime.FuncForPC(val.Pointer()).Name()` for the fully qualified name. `FuncNameShort` extracts the last `.`-separated segment. For anonymous functions, the short name is something like `func1`.

```go
func Zero[T any]() T
```
Returns the zero value of type `T`. Used in error return paths of the generic retrieval functions.

```go
func SortedAsc[T any, O constraints.Ordered](s []T, by func(v T) O) []T
```
In-place sorts a slice by a key function ascending. Used in `ServiceDefinition.MethodCalls()` to return method calls in deterministic order (alphabetical by name).

---

## 4. Additional Remarks

### 4.1 Known quirks and edge cases

**Duplicate method call replacement.** Calling `MethodCall(method, args1...)` and `MethodCall(method, args2...)` on the same `ServiceDefinitionBuilder` silently drops the first. This is by design (the `methodCalls` map is keyed by name) but can surprise users who expect both calls to happen.

**Interface binding creates scope-level binding, not slot-level fill.** The `InterfaceBindingPass` does NOT fill the slot; it adds a binding to the scope. The slot is then filled by the autowiring pass. If autowiring is disabled (`NotAutowired()`), the slot is never filled and validation fails with "argument N is not set" — not with a helpful message about the interface. Users must manually pass the interface arg when autowiring is disabled.

**`compoundArg` with zero sub-args returns nil.** `NewCompoundArg(typ)` with no args returns `(nil, nil)`. If this nil arg is stored in a slot, `slot.IsFilled()` returns false (because `IsSet()` is false and `IsAppended()` is false). This is the documented "empty compound means no-op" behaviour and is used when `Compound` is called with no builders. But in the `Arg()` method of a slice slot, if both `s.arg` and `s.args` are empty/nil, `Arg()` calls `NewCompoundArg(s.ElemType())` which returns `(nil, nil)`, and then the slot returns nil. This nil arg would then fail in `ResolveArg` with "unsupported arg type <nil>". In practice, `ValidateAndCollect` uses `NewZeroArg` for empty variadic slots instead, avoiding this path.

**`SvcByRef` nil check.** The `SvcByRef` function checks `if svc == nil` after `GetService`. This nil check is for the case where `GetService` returns `(nil, nil)` — which happens when the service ID is not found in the root scope. However, `GetService` only searches the root scope, not child scopes. A service defined only in a child scope (because it's a child of another service) is not retrievable via `SvcByRef` or any other public method. This is by design (child services are private) but means that attempting to use a child service's reference across scopes silently returns "not found" rather than an explanatory error.

**`ExecByRef` uses value receiver for `FuncReference`.** The function signature is `func ExecByRef(c Container, ref FuncReference)` — `ref` is a value type. The README example shows `di.ExecByRef(c, fooRef)` correctly, but binding must happen before calling `ExecByRef`. The test confirms this.

**`SvcByRef` uses value receiver for `SvcReference`.** Same pattern.

**`OverrideSvcArg` takes a value, `RemoveSvc` takes a pointer.** The inconsistency is because `OverrideSvcArg` only reads the reference (calls `ref.SvcID()`), while `RemoveSvc` takes `*godi.SvcReference` even though it only reads it too. This is a minor API inconsistency.

**Cycle detection only covers factory args, not method call args.** The `NewCycleValidationPass` only iterates `def.Factory().Args().Slots()`. Method call dependencies are not checked. This is intentional — method calls are the documented workaround for circular dependencies.

**`ContainerBuilder.Build()` returns an error on second call but does not protect against concurrent calls.** No mutex guards `b.built`. Concurrent `Build()` calls could produce a race condition.

**`ExecByRef` signature note.** The function returns `([]any, error)` — not `([][]any, error)`. This is because it executes a single function and returns its return values as a flat slice. `ExecAllByType` and `ExecAllByLabel` return `([][]any, error)` because they return results for multiple functions.

### 4.2 Discrepancies between README and source

**Method call receiver is slot 0.** The README does not mention that method calls have the receiver injected as slot 0. The `MethodCall` builder takes a method value (e.g., `(*Service).SomeMethod`) which Go's method expression already includes the receiver as the first argument. Users providing manual args to `MethodCall` should only provide args for parameters after the receiver.

**`BindSlice` docs.** The README says `BindSlice[Iface, To]()` binds `Iface` or `[]Iface` to a slice `[]To`. The implementation actually wraps `Type[To]()` inside a `Compound[Iface](...)`. The resolved type is `Iface` (the compound's type), not `To`. The slice element type is `Iface`. The compound resolves all `To`-typed services and assembles a `[]Iface`. This works because `To` services are assignable to `Iface`. The docs are functionally accurate but the implementation detail differs from what a reader might expect.

**`Type[T](label)` only uses the last label.** The `Type[T any](label ...Label)` function uses `label[len(label)-1]` if any labels are provided. Passing multiple labels is silently ignored — only the last one is used. This is a potential footgun. The README does not document multi-label behaviour for `Type`.

**README example `SvcByRef[fmt.Stringer](c, ref)`.** This suggests that `SvcByRef` performs a type conversion, not just a type assertion. Looking at the source, `castTo[T]` does a plain Go type assertion `svcAny.(T)`. A concrete type like `MySvc` is not type-assertable to `fmt.Stringer` unless it implements `fmt.Stringer`. The example works because `MySvc` has a `String() string` method. If it did not, the assertion would panic. This is normal Go behaviour but worth noting: `SvcByRef[fmt.Stringer]` succeeds only if the concrete stored type implements `fmt.Stringer`.