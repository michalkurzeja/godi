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
- `Node.Incomplete` and `Graph.WiringDiagnostics` are gated on `Snapshot.Autowired`. Before
  autowiring runs every slot is empty, so marking them says nothing.
- There is no route from the facade `Builder` to a graph. Adding one means an accessor that
  prepares as it hands over, or the graph misses everything registered since.

`Builder.prepare()` is memoised with per-slice cursors, not a single "done" flag, and it is where
compiler passes are registered too: "prepared" means everything the facade knows is in the engine.
Its deferred registration is load-bearing — the order and number of `Services`/`Functions`/
`Bindings` calls must stay irrelevant, and forward references must keep working across calls.

### Diagnostics: two kinds, one of them derived

`Graph.GraphDiagnostics` are stored and are about the graph or the file it came from — a scope no
definition owns, a schema this build does not know. `Graph.WiringDiagnostics()` is derived from
the parameters, so what a reader is told is wrong and what is drawn as wrong cannot disagree.
Encoders render `AllDiagnostics()`. Do not store a wiring fault: set `Param.Unresolved` and
`Param.Note`, and it will be reported.

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
  `docs/v2/documentation.md`, and `plugins/godi/skills/godi-v2/SKILL.md`. Changing the skill also
  means bumping `version` in `plugins/godi/.claude-plugin/plugin.json`, or installed consumers
  never receive the update.
