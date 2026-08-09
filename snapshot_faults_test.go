package di_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/graph"
)

// Fixtures for the faults below. The ones above are wired to build; these exist
// to be rejected.

type Reporter interface{ Report() }

type FileReporter struct{}

func (*FileReporter) Report() {}

type JSONReporter struct{}

func (*JSONReporter) Report() {}

type Audit struct{}

func NewFileReporter() *FileReporter { return &FileReporter{} }
func NewJSONReporter() *JSONReporter { return &JSONReporter{} }

// NewAudit takes a single Reporter, so a binding that collects every one of them
// resolves to a []Reporter this cannot be handed.
func NewAudit(r Reporter) *Audit { return &Audit{} }

func NewAuditOfMany(rs []Reporter) *Audit { return &Audit{} }

type Broken struct{}

func NewBroken() (*Broken, error) { return nil, errors.New("connection refused") }

type GreeterHolder struct{}

func NewGreeterHolder(g Greeter) *GreeterHolder { return &GreeterHolder{} }

// Every build failure is something the compiler objected to, and the snapshot is
// where a reader looks for it. A fault that never reaches the graph leaves a
// picture of a container that appears to be fine.
//
// One case per way a build can fail. What is asserted is the same for all of
// them: the element is marked, the diagnostic is listed, and it repeats what the
// compiler said.
func TestAFailedBuildShowsWhatTheCompilerObjectedTo(t *testing.T) {
	tests := []struct {
		name string
		// build is the container that will not build.
		build func() *godi.Builder
		// node is the service the fault belongs to, and arg its argument. An arg
		// of -1 means the fault is the node's own.
		node string
		arg  int
		// says is what the diagnostic has to repeat of the compiler's words.
		says string
	}{
		{
			// The binding collects every implementation, and the argument takes one.
			// Resolution follows the binding and finds a real service at the end of
			// it, so only validation can tell that it does not fit.
			name: "a binding that resolves to the wrong shape",
			build: func() *godi.Builder {
				return godi.New().
					Services(godi.Svc(NewFileReporter), godi.Svc(NewAudit)).
					Bindings(godi.BindSlice[Reporter, *FileReporter]())
			},
			node: "v2_test.(*Audit)", arg: 0,
			says: "which cannot fill an argument of type",
		},
		{
			// Autobinding cannot choose, and the build stops before autowiring. Every
			// argument in the container is unwired at that point, so nothing but the
			// pass itself knows which one it gave up on.
			name: "an interface with more than one implementation",
			build: func() *godi.Builder {
				return godi.New().Services(
					godi.Svc(NewFileReporter), godi.Svc(NewJSONReporter), godi.Svc(NewAudit),
				)
			},
			node: "v2_test.(*Audit)", arg: 0,
			says: "multiple implementations of interface",
		},
		{
			name: "more services of a type than an argument can take",
			build: func() *godi.Builder {
				return godi.New().Services(
					godi.Svc(NewStore), godi.Svc(NewStore),
					godi.Svc(NewServer, "localhost:8080"), godi.Svc(NewEnGreeter),
				)
			},
			node: "v2_test.(*Server)", arg: 1,
			says: "multiple services found for type",
		},
		{
			// A slice argument matches the slice type first, and two services of it
			// are one too many. Both are found, so the trace has nothing to object to.
			name: "more slice services than a slice argument can take",
			build: func() *godi.Builder {
				reporters := func() []Reporter { return nil }
				return godi.New().Services(
					godi.Svc(reporters), godi.Svc(reporters), godi.Svc(NewAuditOfMany),
				)
			},
			node: "v2_test.(*Audit)", arg: 0,
			says: "multiple services found for type",
		},
		{
			name: "more services with a label than an argument can take",
			build: func() *godi.Builder {
				return godi.New().Services(
					godi.Svc(NewStore).Labels("store"),
					godi.Svc(NewStore).Labels("store"),
					godi.Svc(NewServer, "localhost:8080", godi.Type[*Store]("store")).NotAutowired(),
					godi.Svc(NewEnGreeter),
				).Bindings(godi.BindType[Greeter, EnGreeter]())
			},
			node: "v2_test.(*Server)", arg: 1,
			says: "multiple services found with label",
		},
		{
			// A factory that fails is a runtime fault with no static counterpart.
			// Nothing about the wiring is wrong, so nothing about the wiring says so.
			name: "a factory that returns an error",
			build: func() *godi.Builder {
				return godi.New().Services(godi.Svc(NewBroken).Eager())
			},
			node: "v2_test.(*Broken)", arg: -1,
			says: "connection refused",
		},
		{
			// A label says which service to take and nothing about its type, so the
			// mismatch only shows up when the container goes to pass it. It is still
			// the argument that is wrong, and it is reported there.
			name: "a labelled service of the wrong type",
			build: func() *godi.Builder {
				return godi.New().Services(
					godi.Svc(NewStore).Labels("greeter"),
					godi.Svc(NewGreeterHolder, godi.Type[Greeter]("greeter")).NotAutowired().Eager(),
				)
			},
			node: "v2_test.(*GreeterHolder)", arg: 0,
			says: "should be of type",
		},
		{
			name: "an argument nothing filled",
			build: func() *godi.Builder {
				return godi.New().Services(
					godi.Svc(NewServer, godi.Type[Greeter](), godi.Type[*Store]()).NotAutowired(),
					godi.Svc(NewEnGreeter), godi.Svc(NewStore),
				).Bindings(godi.BindType[Greeter, EnGreeter]())
			},
			node: "v2_test.(*Server)", arg: 2,
			says: "argument 2 is not set",
		},
		{
			name: "a dependency nothing provides",
			build: func() *godi.Builder {
				return godi.New().Services(godi.Svc(NewServer, "localhost:8080"), godi.Svc(NewEnGreeter))
			},
			node: "v2_test.(*Server)", arg: 1,
			says: "no services found for type",
		},
		{
			// Reported against the argument that closes the circle, which is the one
			// to change to break it.
			name: "a circular dependency",
			build: func() *godi.Builder {
				storeOfServer := func(s *Server) *Store { return &Store{} }
				return godi.New().Services(
					godi.Svc(NewServer, "localhost:8080"), godi.Svc(NewEnGreeter), godi.Svc(storeOfServer),
				)
			},
			node: "v2_test.(*Store)", arg: 0,
			says: "circular dependency",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := graphOfFailedBuild(t, test.build())

			node := nodeOf(t, g, test.node)
			require.True(t, node.Faulty(), "the reader has to be able to find %s", test.node)

			var diagnostics []graph.Diagnostic
			if test.arg < 0 {
				diagnostics = node.Diagnostics
			} else {
				diagnostics = paramOf(t, g, test.node, test.arg).Diagnostics
			}

			require.NotEmpty(t, diagnostics, "the fault is not on the thing it is about")
			require.Contains(t, messagesOf(diagnostics), test.says,
				"the graph does not repeat what the compiler objected to")
			require.Equal(t, graph.SeverityError, diagnostics[0].Severity)
			require.NotEmpty(t, diagnostics[0].Pass, "a compiler pass reported it, and the graph says which")
		})
	}
}

// A pass is credited with what it wired even when it also objected to something.
// The failure snapshot is the main reader of a partial graph, and this is where the
// difference matters most: the interface binding pass could not choose between two
// implementations, but it had already bound every other interface it handles, and
// none of that is wiring the user did.
func TestAFailingPassStillOwnsTheWiringItDid(t *testing.T) {
	g := graphOfFailedBuild(t, godi.New().Services(
		// Two Reporters, so the pass objects to the argument that takes one.
		godi.Svc(NewFileReporter), godi.Svc(NewJSONReporter), godi.Svc(NewAudit),
		// One Greeter, which the same pass binds before it gives up.
		godi.Svc(NewEnGreeter), godi.Svc(NewGreeterHolder),
	))

	require.Equal(t, "interface binding", g.Snapshot.Failed)
	require.NotContains(t, g.Snapshot.Done, "interface binding")
	require.False(t, g.Snapshot.Autowired, "the build stopped before autowiring")

	require.Len(t, g.Bindings, 1, "the pass bound the interface it could")
	binding := bindingOf(t, g, "github.com/michalkurzeja/godi/v2_test.Greeter")
	require.Equal(t, graph.BindOriginAutobinding, binding.Origin,
		"a failing pass's own binding must not read as one the user declared")
	require.Equal(t, "interface binding", binding.OriginPass)
}

// An argument the interface binding pass gave up on has no wiring of its own to
// draw: the build stops there, so nothing ever fills the slot and there is no
// argument to trace. Without the implementations the pass reported, the picture
// is a container of roots with no dependency in it at all, and the ambiguity is
// the one thing the reader came to see.
func TestAnArgumentThatCouldNotChooseShowsTheCandidates(t *testing.T) {
	g := graphOfFailedBuild(t, godi.New().Services(
		godi.Svc(NewFileReporter), godi.Svc(NewJSONReporter), godi.Svc(NewAudit),
	))

	param := paramOf(t, g, "v2_test.(*Audit)", 0)
	require.True(t, param.Unwired(), "the build stopped before anything filled it")

	edges := edgesOf(g, param)
	require.Len(t, edges, 2, "both implementations are worth looking at")

	targets := make([]graph.NodeID, 0, len(edges))
	for _, e := range edges {
		require.True(t, e.Candidate, "the container chose neither of them")
		require.True(t, g.EdgeFaulty(e), "an argument that cannot choose resolves to neither")
		targets = append(targets, e.To)
	}
	require.ElementsMatch(t, []graph.NodeID{
		nodeOf(t, g, "v2_test.(*FileReporter)").ID,
		nodeOf(t, g, "v2_test.(*JSONReporter)").ID,
	}, targets)

	require.Equal(t, 2, nodeOf(t, g, "v2_test.(*Audit)").OutDegree)
	require.False(t, nodeOf(t, g, "v2_test.(*FileReporter)").Root,
		"an implementation something could have taken is not the top of a tree")
}

// A definition that will not parse never becomes a node, so there is nothing
// narrower than the container to say it on. Without this the snapshot leaves the
// service out and says nothing at all.
func TestABuildThatFailedBeforeCompilationSaysSoOnTheContainer(t *testing.T) {
	g := graphOfFailedBuild(t, godi.New().Services(godi.Svc("not a function")))

	require.NotEmpty(t, g.Diagnostics)
	require.Contains(t, messagesOf(g.Diagnostics), "factory kind must be func")
}

// A pass is entitled to say something is worth knowing without stopping the
// build. The container it produced still carries it.
func TestAWarningFromAPassSurvivesASuccessfulBuild(t *testing.T) {
	pass := reportsOnEveryService(di.Diagnostic{Severity: di.SeverityWarning, Message: "looks expensive"})

	c, err := godi.New().Services(godi.Svc(NewStore)).CompilerPasses(pass).Build()
	require.NoError(t, err, "a warning does not fail a build")

	g := graphOf(t, c)
	node := nodeOf(t, g, "v2_test.(*Store)")

	require.False(t, node.Faulty(), "worth knowing is not the same as wrong")
	require.Equal(t, []graph.Diagnostic{
		{Severity: graph.SeverityWarning, Message: "looks expensive", Pass: "nosy"},
	}, node.Diagnostics)
}

// A message carries whatever the code that failed put in its error, and a graph
// travels: into a file, and over HTTP. What is in it has to be the caller's
// choice.
func TestDiagnosticMessagesCanBeHeldBack(t *testing.T) {
	t.Parallel()

	c, err := godi.New().Services(godi.Svc(NewStore)).CompilerPasses(reportsOnEveryService(
		di.Diagnostic{Severity: di.SeverityWarning, Message: "connection refused"},
	)).Build()
	require.NoError(t, err)

	marks := graphOf(t, c, graph.WithDiagnosticMarks())
	node := nodeOf(t, marks, "v2_test.(*Store)")
	require.Len(t, node.Diagnostics, 1, "what is worth saying still shows as said")
	require.Empty(t, node.Diagnostics[0].Message, "and what it says is left out")
	require.Equal(t, "nosy", node.Diagnostics[0].Pass)

	none := graphOf(t, c, graph.WithoutDiagnostics())
	require.Empty(t, nodeOf(t, none, "v2_test.(*Store)").Diagnostics)
	require.Empty(t, none.AllDiagnostics())

	// Left out means left out, whoever found it. Extraction's own account of an
	// argument nothing wired goes the same way as a pass's, marks and all.
	broken := func() *godi.Builder {
		return godi.New().Services(godi.Svc(NewServer, "localhost:8080").NotAutowired())
	}
	require.NotEmpty(t, graphAtValidation(t, broken()).AllDiagnostics(),
		"the container is broken, which is what makes this worth asking")

	quiet := graphAtValidation(t, broken(), graph.WithoutDiagnostics())
	require.Empty(t, quiet.AllDiagnostics())
	require.False(t, nodeOf(t, quiet, "v2_test.(*Server)").Faulty())

	redacted := graphOf(t, c, graph.WithDiagnosticRedactor(
		func(message string) (string, bool) { return "‹redacted›", strings.Contains(message, "connection") },
	))
	require.Equal(t, "‹redacted›", nodeOf(t, redacted, "v2_test.(*Store)").Diagnostics[0].Message)
}

// reportsOnEveryService is a pass of the kind an extension would write: it says
// something about each service and leaves the build to carry on.
func reportsOnEveryService(d di.Diagnostic) *di.CompilerPass {
	return di.NewCompilerPass("nosy", di.PreValidation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		for _, def := range b.ServiceDefinitionsSeq() {
			d.Site = di.AtService(def)
			b.Report(d)
		}
		return nil
	}))
}

func messagesOf(diagnostics []graph.Diagnostic) string {
	out := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		out = append(out, d.Message)
	}
	return strings.Join(out, "\n")
}
