// Package graph turns a godi container into an inspectable dependency graph:
// what depends on what, what is passed where, and how each dependency got there.
//
// # Getting a graph
//
//	c, err := di.New().Services(...).Build()
//
//	g, err := extract.From(c.(*di.Container))
//	err = g.Encode(os.Stdout, dot.New())
//
// Reading a container is graph/extract's job, not this package's: the model is
// a leaf, and stays one, so that anything holding a graph - an encoder, a tool
// reading a file - carries none of the container engine.
//
// Encoders live in their own packages - graph/dot, graph/text, graph/html - so
// that a program only compiles the formats it asks for. The model itself is
// plain data, so a third party can add a format by implementing Encoder.
//
// # Wiring that is not finished yet
//
// A graph can also be taken from a container that is still being built: with
// extract.FromBuilder, from inside a compiler pass or after a build that
// failed. What a later pass would have wired is simply not there, so those
// graphs carry a Snapshot saying when they were taken and which passes had run,
// and every encoder passes that on. Without it a half-wired graph reads as a
// finished one with dependencies missing.
//
// # Provenance
//
// Every edge records two independent facts. Origin says who wired the argument:
// you (ArgOriginManual), godi's autowiring (ArgOriginAutowiring), or a compiler
// pass (ArgOriginCompilerPass, with OriginPass naming it). Bindings says how the
// argument resolved to this particular dependency, listing the interface
// bindings traversed and, for each, whether you declared it
// (BindOriginManual) or godi created it for you (BindOriginAutobinding).
//
// The two are independent: a hand-written di.Type[Iface]() resolves through
// whatever binding exists, including one godi created because a different
// argument needed it.
//
// # Narrowing it down
//
// Past a hundred or so nodes no format produces a picture worth reading, so
// readability comes from asking a narrower question rather than from a better
// renderer. Filters work on the model, so every format gets them:
//
//	g = g.Select(
//		graph.Focus(graph.ByType("*app.(*Server)"), graph.Dependencies(3)),
//		graph.HideMethodCalls(),
//	)
//
// Narrowing is a separate call from Extract, which is how one extraction
// becomes several views - and why a Filter is its own type: an extraction
// Option cannot be passed to Select, where it would have nothing to do.
//
// Where a limit rather than a name cut the graph, the nodes left at the edge
// carry Node.Elided: how many of their neighbours went. A picture that stops
// without saying so reads as the whole story.
//
// # Roots
//
// Node.Root marks a node nothing injects: the top of a dependency tree. Those
// are the entry points of the container, together with any wiring nothing uses,
// and nothing here tries to tell the two apart - a service fetched at runtime
// with SvcByType, SvcByLabel or SvcByRef leaves no trace in the container, so
// only the reader knows which of their roots are deliberate.
package graph
