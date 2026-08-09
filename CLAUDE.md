# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

godi v2 is a reflection-based dependency-injection container for Go, plus a dependency-graph
extractor, renderers for it, and a CLI that renders a graph handed over as JSON.

## Commands

```bash
go test ./...                              # what CI runs
go test ./graph/... -run TestName -count=1 # one test, no cache
go test -bench BenchmarkBuild -run '^$' .  # container compilation cost
go vet ./...
golangci-lint run                          # config in .golangci.yaml
task mockery                               # regenerate all mocks (go tool mockery)
```

Mocks are generated wholesale from `.mockery.yaml` (`all: true, recursive: true`) into a
`mocks/` package next to each interface. Adding or changing any exported interface — the root
`Container` above all — means regenerating. `di.Arg` is excluded: it is sealed, so a mock of it
cannot compile.

### Seeing the graph work

```bash
go run ./examples/graph -format text
go run ./examples/graph | dot -Tsvg -o graph.svg
go run ./examples/graph -format html -open
go run ./examples/graph -format html -snapshot -open   # the wiring partway through compilation

# The code-free path: a failed build writes JSON, the CLI renders it.
GODI_SNAPSHOT_ON_BUILD_ERR=true GODI_SNAPSHOT_PATH=/tmp go run ./your/failing/main
go run ./cmd/godi export text /tmp/godi-graph-*.json
go run ./cmd/godi view /tmp/godi-graph-*.json
```

### Viewer tests

Most of `graph/html`'s behaviour lives in the browser, so `TestViewerRegressions` writes three
pages, drives them with headless Chrome over CDP from `graph/html/testdata/viewer_test.mjs`,
and reports each JS assertion as a Go subtest. It needs `node` and a Chrome (`CHROME_PATH`
overrides the search) and **skips** when either is missing — a green `go test ./...` does not
mean the viewer was exercised. Run it deliberately after touching `assets/viewer.js`,
`assets/page.html`, `assets/viewer.css` or `payload.go`:

```bash
GODI_REQUIRE_VIEWER_TESTS=1 go test ./graph/html/... -run TestViewerRegressions
```

That variable turns both skips into failures, which is how the CI job asserts the suite ran.

Watch the escaping when editing the `.mjs`: expressions are sent to the browser inside Go-side
template literals, so a regex needs `/\\s+/g`, not `/\s+/g`.

## Architecture

### Two packages named `di`

The module root (`package di`, imported by users as `di "github.com/michalkurzeja/godi/v2"`) is
the public facade: fluent builders, generic accessors, the `Container` interface. The
subdirectory `di/` (also `package di`) is the engine: scopes, definitions, compiler, argument
resolution, instantiation. The root imports the engine unaliased. Extension authors import the
engine directly; ordinary users never should.

### The graph packages, and why they are split

```
di/                    the engine. Knows nothing about graphs.
graph/                 the model + JSON. A leaf: stdlib only.
graph/extract          reads a container, emits the model — the only package that needs both
graph/dot|text|html|json  encoders, one package each. Import graph only.
graph/serve            HTTP handler over a graph.Source — imports graph/html and graph/json
graph/internal/render  short names, wrapping, per-format quoting
<root>                 the facade. Imports di, graph, graph/extract.
cmd/godi/              the CLI — the only cobra in the module, and the only os/exec in it
```

**A graph is a view over the engine, not part of it.** The engine used to import the model so
that a container could describe itself, and the cycle that supposedly forced it existed only
because `Container.Graph` did. Extraction lives in `graph/extract` instead, and the model stays a
leaf so that anything consuming the *format* is free of the engine: `godi view graph.json` has no
container in it and must not link the DI engine (nor `dominikbraun/graph`, `uuid`, `orderedmap`,
`lo`) merely to render a file. Third-party encoders get the same deal.

The facade's way in is `di.Graph(c)` and `di.LiveGraph(c)` (`graph.go` at the root). `Build` hands
back the `Container` interface and `extract.From` reads the container itself, so without them every
user writes the assertion. They are at the root because the root is the only place that sees both
sides: `extract` cannot take the facade interface — the root imports `extract` for `snapshot.go`,
so that is a cycle — and the engine cannot grow a `Graph` method, which is the coupling this split
removed. A `Container` of the user's own standing in front of one godi built implements
`di.Unwrapper`, and `engineContainer` follows it down. That loop is unbounded: an `Unwrap` that
never gets closer to the container is the implementer's mistake to avoid, not godi's to police.

`deps_test.go` enforces three rules, and they are worth reading before moving anything:

- the library links no renderer — a godi binary carries no Graphviz plumbing or viewer assets;
- no library package links `os/exec` — this is now true by construction, since the only code that
  can start a process is in `cmd/godi`. Do not spend it;
- `graph` and the encoders link neither `di` nor the facade — the model stays a leaf.

`cmd/godi` lives in this module, so cobra is one of its requirements. It costs a consumer nothing
they link — only three `/go.mod` hashes in their `go.sum` and three lines in `go list -m all` —
but that holds only while nothing outside `cmd/godi` imports it, which is why the test checks.

Writing JSON is the one format on the model rather than beside it, and that is what the failure
snapshot rests on: `snapshot.go` writes a failed build's graph with nothing but the model, so
`GODI_SNAPSHOT_ON_BUILD_ERR` costs a plain godi binary no new dependency at all. `graph/json` is
the same writing as an `Encoder`. Rendering is the CLI's job.

Nothing display-related belongs in `di`: extraction stores full signatures, and short forms are
methods on the model backed by `graph/internal/render`, which Go's internal rule keeps reachable
only from `graph/…`. The one exception is the deprecated `di.Print`, which writes its own plain
outline rather than putting an encoder on every consumer's link path.

Encoders implement `graph.Encoder` (`Format()` + `Encode(g, w)`) and each exposes
`New(opts...)` with its own option namespace, so `dot.Theme(...)` and `html.Theme(...)` coexist.
`graph/html` imports `graph/dot` — there is one DOT implementation, not two.

**Display rules live on the model**, so that every format gives the same answer: `Node.Title`,
`Node.Subtitle`, `Node.KnownByName`, `Edge.DecidedBy`, `Edge.PassCredit`. The viewer renders what
the payload hands it; what it decides for itself is only which of its boxes have room for a
second line.

### One call, one instantiation context

`di/instantiation_context.go` is the rule that instantiation obeys: within one call into the
container, every factory runs before any method call. A public `Scope.GetService*`/
`ExecuteFunction*` opens an `instantiationContext`, `Scope.instantiate` hands it the built
service's method calls, and the entry point drains the queue afterwards. `NewEagerInitPass` opens
one context for a whole build, so a build wires the same way a later request would.

Two things rest on it, and both break if a method call is ever run inline again:

- Wiring that loops back through a method call gives one instance whichever end is asked for first.
  Running the call inline meant the loop re-entered a factory that had not published yet, and built
  a second instance with no error.
- `instantiationContext.svcDefStack` names the factories running now, so a factory cycle that
  reaches runtime is an error rather than a stack overflow.

The price is stated in the docs and pinned by a test: **a factory does not see the method calls of
its dependencies.** Do not add a special case to win it back — deciding per-service when to defer
would make construction depend on which service was asked for first, which is what this replaced.

`Arg.resolve` carries the context. That is only possible because `Arg` is sealed; the exported
`Factory/Method/Func.Execute` and `ResolveArg` keep their signatures and open a context of their
own.

### Concurrency: two locks, and one call builds at a time

A built container is safe to share. Two locks on `Container` carry that, and they have opposite
rules — the difference is the whole design:

- `buildMu` serialises construction: one call builds at a time, the rest wait. It **is** held
  across user code. That is what makes `shared` mean one construction, and it is why there is no
  in-flight bookkeeping, no cross-goroutine cycle detection and no wait-for graph to get right.
- `mu` guards every scope's `instances` map. It is **never** held across user code, so
  `extract.Live` and `Instantiated` never wait behind a running factory. That was stage 2's whole
  point and it still holds.

Lock order is always `buildMu → mu`.

A call `stage`s what it builds and `commit`s it once its method calls have run. Nothing outside a
call can see a service that is built but not yet configured —
`TestAServiceIsNotVisibleUntilItsMethodCallsHaveRun` pins that, and it fails if `instantiate`
publishes early as it used to.

`withInstantiationContext` defers `ic.release()`. Without it a panicking factory keeps `buildMu`
and every later call blocks forever; `TestAPanickingFactoryLeavesTheContainerUsable` is what
catches that. Anything staged and unpublished is dropped, so a failed or panicking factory leaves
the container as it found it and the next call retries.

**A factory, method call or function must not resolve from the container building it.** It would
block on the lock its own frame holds. `insideUserCode` (`di/reentrancy.go`) catches the
same-goroutine case by looking for the `callUserCode` frame on the stack — legitimate, since
`runtime.Callers` describes the calling goroutine and needs no goroutine identity. Keep
`callUserCode` the only route into user code, or the check goes blind.

A factory that starts a goroutine to resolve and waits for it makes the same mistake and hangs.
That goroutine has a stack of its own with no `callUserCode` frame on it, so the check cannot see
it, and no check that avoids goroutine identity could. It is forbidden all the same.

The cost, measured: a warm resolve is ~14ns slower, a *parallel* warm resolve is ~16% faster than
the old exclusive mutex, and two goroutines building two different services for the first time go
one after the other. `BenchmarkResolve`/`BenchmarkResolveParallel` exist to keep that honest.

### Defaults are applied at registration

`Scope.AddServiceDefinitions` and `AddFunctionDefinitions` fill in the properties a definition's
registration did not choose, from the container's `Defaults`. Every route in goes through them — the
facade builders, `ParseAndBuild`, a pass reaching in via `builder.RootScope()` — so a compiler pass
needs to know nothing about defaults to get them right. `ContainerBuilder.Defaults()` is for reading
them, not for applying them.

The definition remembers whether its registration chose each property (`property`,
`di/definition_base.go`). That bookkeeping used to sit on the facade's `ServiceDefinitionBuilder`,
which is why `di.New(di.DefaultEager())` did nothing for a definition built inside a pass. Do not
move it back.

The deprecated process-wide `SetDefault*` globals have one reader left, `NewDefaults`. A definition
gets its properties from the container it is registered in, never from the process. Do not seed them
in a constructor.

### Provenance: who wired what

The whole point of the graph is telling apart an argument you wrote, one godi autowired, and one
a compiler pass substituted — at runtime they are byte-identical. The mechanism lives in the
compiler:

- `Slot.Set`/`Append` (`di/arg.go`) and `NewInterfaceBinding` (`di/binding.go`) set a `dirty` flag.
- `Compiler.Run` clears the marks once before the pass loop — so anything filled at definition
  time stays `ArgOriginManual` — then attributes everything still dirty after each pass to that
  pass.
- A pass declares what its own edits mean via `withArgOrigin`/`withBindOrigin`, which stay
  unexported: reading provenance is open to a pass (`Slot.Origin`, `InterfaceBinding.Origin`),
  claiming godi's own is not. The two built-ins claim `ArgOriginAutowiring` and
  `BindOriginAutobinding`; everything else defaults to `...CompilerPass` plus the pass name.

Never identify a pass by name — names are neither unique nor stable, and a third-party pass may
be called "autowiring". Attribution is the pass's own declaration.

The engine's `ArgOrigin`/`BindOrigin` are integers for passes to read; `graph`'s are strings and
**are the wire format**, versioned by `Schema`. `graph/extract` translates. Do not couple them.

The model keeps this as two independent facets: `Origin`/`OriginPass` (who wired the argument)
and `Bindings` (which interface bindings it resolved through, each with its own origin). Do not
collapse them into one enum.

### Partial graphs

A graph can come from a built container (`extract.From`), from a `*di.ContainerBuilder`
mid-compilation or after a failed build (`extract.FromBuilder`), or from `extras.CaptureGraph`.
Anything but the first carries a `graph.Snapshot` saying when it was taken and which passes had
run, and every encoder must pass that on: a half-wired graph is indistinguishable from a finished
one with dependencies missing.

Three consequences worth knowing before touching this area:

- A failed `Build` deliberately leaves `b.container` non-nil, so `extract.FromBuilder` after a
  failure shows exactly where the compiler stopped. That is the main use of the feature; a
  successfully built builder has handed its container over and says so.
- Saying that an argument is unwired (`markUnwired`) is gated on `Snapshot.Autowired`. Before
  autowiring runs every slot is empty, so marking them says nothing. What a pass objected to is
  not gated: a build that stopped in the Automation stage is exactly when that is all there is.
- There is no route from the facade `Builder` to a graph, and `di.Graph` is not one: it takes a
  built container, where nothing is pending. Adding one means an accessor that prepares as it
  hands over, or the graph misses everything registered since.

`Builder.prepare()` is memoised with per-slice cursors, not a single "done" flag, and it is where
compiler passes are registered too: "prepared" means everything the facade knows is in the engine.
Its deferred registration is load-bearing — the order and number of `Services`/`Functions`/
`Bindings` calls must stay irrelevant, and forward references must keep working across calls.

### Diagnostics: stored on the thing they are about

`Graph`, `Scope`, `Node` and `Param` each carry their own `Diagnostics`, and the graph itself takes
what fits nothing narrower — a pass that could not be scheduled, a definition that never made it
into the container, a schema this build does not know. `AllDiagnostics()` is a walk over them, in a
stable order, pairing each with the ids of what it came from; that is what the encoders render.

One record, many readers, so nothing can disagree: `Node.Faulty()` and `Param.Faulty()` are
questions rather than stored flags, and `Select` cuts a diagnostic away with the element it was
about, counting it into `Graph.ElidedDiagnostics`. **Do not add a list beside the elements**, and do
not copy a param's diagnostic onto its node — "is this node faulty" is a question that looks at the
node and its arguments.

A compiler pass is what puts most of them there. `ContainerBuilder.Report` takes a `di.Diagnostic`
whose `Site` names what it is about, and the sites are exactly the levels the model stores at:
`AtContainer`, `AtScope`, `AtService`, `AtFunction`, `AtServiceArg`, `AtFunctionArg`. There is no
edge site, and there should not be: the engine has no edge to point at, since an edge is
manufactured by extraction out of `ArgTrace.Matches` and its ordinal is a position in a list that
depends on what happens to be registered. If a fault ever needs to be narrower than a slot, the way
in is a sub-argument site, which the engine does have.

`di.Diagnostic` carries `Message` and `Err`, and they are two things rather than one twice: the
message is what a reader is told and all the graph carries, and the error is what `Build` returns,
wrapped however the pass words its failures. That split is what let every built-in pass move onto
reporting without changing a single released error string. `ReportError` builds both from one
error, so they cannot drift.

Compilation stops when a pass returns an error **or** reports an error-severity diagnostic; a pass
that reports one returns nil rather than the same error twice. A returned error is recorded as a
container-level diagnostic, so a third-party pass that never calls `Report` still reaches the graph
— coarsely, but never silently.

A diagnostic message is arbitrary user text: an eager-init failure carries whatever a factory put
in its error. `graph.Config` gates it — `DiagnosticMessages` (the default), `DiagnosticMarks`,
`DiagnosticNone`, plus a redactor — the same care `LiteralMode` takes over values.

**Where a compiler diagnostic names an argument, it replaces what extraction guessed about it**
(`graph/extract/diagnostics.go`). The pass saw more. This loses nothing because the validation pass
is exhaustive over slots and `compoundArg.validate` joins every sub-argument's error, so its
diagnostic for a faulty argument already contains everything the trace could have added. Extraction
cannot instead ask "has validation run": a pass may not be identified by name.

`ArgTrace.Fault` is not obsolete and must stay. It is what the graph can say when no pass has
objected *yet* — `extras.CaptureGraph`, `extract.LiveBuilder` mid-build, a builder nobody compiled,
a build that failed at an earlier pass — and `ArgFaultCircularBinding` has no compiler counterpart
at all.

### Extraction asks the arguments

An `Arg` answers everything about itself behind a sealed interface (`validate`, `resolve`,
`resolveIDs`, `trace`). `trace` is what extraction reads: it says what matched, by which
mechanism, and through which bindings — the mechanism resolution itself discards. There is no
second walk over the argument kinds to keep in step, and a new kind means touching one place.

Resolve against `def.EffectiveScope()`, never `def.Scope()`.

## Conventions

- Tests live in external `_test` packages; engine internals get `*_internal_test.go` in-package.
  testify `require` only, table-driven, `require.ErrorContains` with the full message. Test names
  are sentences: `TestAPassCanAddAPass`, not `TestDI_Passes`.
- `Container.Print` and `di.Print` are deprecated and frozen. New output work goes in the encoders.
- User-facing behaviour is documented in three places that must stay in step: `README.md`,
  `docs/v2/documentation.md`, and `plugins/godi/skills/godi-v2/SKILL.md`.
- `version` in `plugins/godi/.claude-plugin/plugin.json` is bumped **once per release**, not once
  per change: it is what tells an installed consumer to update, and nobody installs a branch. On a
  branch, raise it one step above what `main` has and leave it there however many times the skill
  changes before the merge.
