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
`Container` above all — means regenerating.

### Seeing the graph work

```bash
go run ./examples/graph -format text
go run ./examples/graph | dot -Tsvg -o graph.svg
go run ./examples/graph -format html -open
go run ./examples/graph -format html -snapshot -open   # wiring as declared, before compilation

# The code-free path: a failed build writes JSON, the CLI renders it.
GODI_SNAPSHOT_ON_BUILD_ERR=true GODI_SNAPSHOT_PATH=/tmp go run ./your/failing/main
go run ./cmd/godi export text /tmp/godi-graph-*.json
go run ./cmd/godi view /tmp/godi-graph-*.json
```

### Viewer tests

Most of `graph/html`'s behaviour lives in the browser, so `TestViewerRegressions` writes two
pages, drives them with headless Chrome over CDP from `graph/html/testdata/viewer_test.mjs`,
and reports each JS assertion as a Go subtest. It needs `node` and a Chrome (`CHROME_PATH`
overrides the search) and **skips** when either is missing — a green `go test ./...` does not
mean the viewer was exercised. Run it deliberately after touching `assets/viewer.js`,
`assets/page.html`, `assets/viewer.css` or `payload.go`.

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
graph/                 model only — imports nothing of godi's, stdlib only. JSON lives here.
graph/dot|text|html    encoders, one package each
graph/serve            HTTP handler over a graph.Source — imports graph/html
graph/view             opens a rendered graph — the only os/exec in the library
graph/internal/render  short names, wrapping, per-format quoting
di/graph.go            extraction: reads unexported engine state, emits the model
cmd/godi/              the CLI — the only cobra in the module, and the only os/exec caller
```

The model must stay a leaf. The root `Container` interface names `graph.Config`, so
`di` → `graph` → `di` would be a cycle, and every godi binary would otherwise carry Graphviz
plumbing and embedded viewer assets. The split is enforced by `deps_test.go`, and by hand:

```bash
go list -deps github.com/michalkurzeja/godi/v2 |
  grep -E 'graph/(dot|html|serve|view)|os/exec|spf13'
# must print nothing — graph/text alone is expected, since Print delegates to it
```

The `os/exec` half of that rule is about capability, not size: it is two small stdlib packages,
and `os` and `syscall` are already linked. What it buys is that no binary linking godi can start
a process. Do not spend it.

`cmd/godi` lives in this module, so cobra is one of its requirements. It costs a consumer nothing
they link — only three `/go.mod` hashes in their `go.sum` and three lines in `go list -m all` —
but that holds only while nothing outside `cmd/godi` imports it, which is why the test checks.

JSON is the one format inside `graph` rather than beside it, and that is what the failure
snapshot rests on: `snapshot.go` writes a failed build's graph with nothing but the model, so
`GODI_SNAPSHOT_ON_BUILD_ERR` costs a plain godi binary no new dependency at all. Rendering is the
CLI's job. Anything that would put a renderer on the root's import path is the wrong shape.

Nothing display-related belongs in `di`: extraction stores full signatures, and short forms are
methods on the model backed by `graph/internal/render`, which Go's internal rule keeps reachable
only from `graph/…`.

Encoders implement `graph.Encoder` (`Format()` + `Encode(g, w)`) and each exposes
`New(opts...)` with its own option namespace, so `dot.Theme(...)` and `html.Theme(...)` coexist.
`graph/html` imports `graph/dot` — there is one DOT implementation, not two.

### Provenance: who wired what

The whole point of the graph is telling apart an argument you wrote, one godi autowired, and one
a compiler pass substituted — at runtime they are byte-identical. The mechanism is internal and
lives in the compiler:

- `Slot.Set`/`Append` (`di/arg.go`) and `NewInterfaceBinding` (`di/binding.go`) set a `dirty` flag.
- `Compiler.Run` clears the marks once before the pass loop — so anything filled at definition
  time stays `argOriginManual` — then attributes everything still dirty after each pass to that
  pass.
- A pass declares what its own edits mean via `withArgOrigin`/`withBindOrigin`. The two built-ins
  claim `argOriginAutowiring` and `bindOriginAutobinding`; everything else defaults to
  `...CompilerPass` plus the pass name.

Never identify a pass by name — names are neither unique nor stable, and a third-party pass may
be called "autowiring". Attribution is the pass's own declaration.

The model keeps this as two independent facets: `Origin`/`OriginPass` (who wired the argument)
and `Bindings` (which interface bindings it resolved through, each with its own origin). Do not
collapse them into one enum.

### Partial graphs

A graph can come from a built container, from `*ContainerBuilder` mid-compilation, from the root
`Builder` before `Build`, or from `extras.CaptureGraph`. Anything but the first carries a
`graph.Snapshot` saying when it was taken and which passes had run, and every encoder must pass
that on: a half-wired graph is indistinguishable from a finished one with dependencies missing.

Two consequences worth knowing before touching this area:

- A failed `Build` deliberately leaves `b.container` non-nil, so `graph.Extract(builder)` after a
  failure shows exactly where the compiler stopped. That is the main use of the feature.
- `Node.Incomplete` is gated on `Snapshot.Autowired`. Before autowiring runs every slot is empty,
  so marking them says nothing.

`Builder.prepare()` is memoised with per-slice cursors, not a single "done" flag: reading the
graph prepares what is registered so far, and services registered afterwards must still reach the
container.

### Extraction mirrors the resolver

`di/graph.go` walks arguments in parallel with `di/arg_resolver.go` rather than calling
`ResolveArgIDs`, which discards the mechanism that matched. The duplication is deliberate and is
held together by a parity test — if you change resolution order (especially the `flexibleSliceArg`
branch order), change both and expect that test to tell you.

Resolve against `def.EffectiveScope()`, never `def.Scope()`.

## Conventions

- Tests live in external `_test` packages; engine internals get `*_internal_test.go` in-package.
  testify `require` only, table-driven, `require.ErrorContains` with the full message.
- `Container.Print` and `di.Print` are deprecated and implemented over `graph/text`. New output
  work goes in the encoders.
- User-facing behaviour is documented in three places that must stay in step: `README.md`,
  `docs/v2/documentation.md`, and `plugins/godi/skills/godi-v2/SKILL.md`. Changing the skill also
  means bumping `version` in `plugins/godi/.claude-plugin/plugin.json`, or installed consumers
  never receive the update.
