package di_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
)

type printLogger interface{ Log(string) }

type printConsole struct{}

func (printConsole) Log(string) {}

type printFile struct{}

func (printFile) Log(string) {}

type (
	printConn   struct{}
	printServer struct{}
	printSink   struct{}
)

func newPrintConsole() printConsole { return printConsole{} }
func newPrintFile() printFile       { return printFile{} }
func newPrintConn() *printConn      { return &printConn{} }

func newPrintServer(*printConn, printLogger) *printServer { return &printServer{} }

// Collects every logger in the container, so the slot's own type is
// []printLogger while the binding godi creates is on printLogger.
func newPrintSink(...printLogger) *printSink { return &printSink{} }

func printed(t *testing.T, c godi.Container) string {
	t.Helper()

	var sb strings.Builder
	c.Print(&sb) //nolint:staticcheck // SA1019: this is the test for the deprecated call.
	return sb.String()
}

// Print only ever walked the one scope it was handed, and Container.Print hands
// it the root - so everything registered with Children(...) was missing from
// the output entirely.
func TestPrintShowsChildScopes(t *testing.T) {
	t.Parallel()

	c, err := godi.New().Services(
		godi.Svc(newPrintServer).Children(godi.Svc(newPrintConn)),
		godi.Svc(newPrintConsole),
	).Build()
	require.NoError(t, err)

	out := printed(t, c)

	require.Contains(t, out, "di_test.(*printConn)", "a service in a child scope is still in the container")
	require.Contains(t, out, "children of di_test.(*printServer)", "and the scope it lives in is named")
}

// Bindings were looked up on the slot's own type. An autowired slice slot asks
// for []printLogger while the binding godi creates is on printLogger, so the
// row showed the declared type and never what was actually injected.
func TestPrintResolvesASliceSlotThroughItsElementBinding(t *testing.T) {
	t.Parallel()

	c, err := godi.New().Services(
		godi.Svc(newPrintSink),
		godi.Svc(newPrintConsole),
		godi.Svc(newPrintFile),
	).Build()
	require.NoError(t, err)

	out := printed(t, c)

	require.Contains(t, out, "di_test.printConsole", "the implementations that were injected")
	require.Contains(t, out, "di_test.printFile")
}

// Print read every slot's Arg without asking whether anything had filled it,
// and an unfilled slot returns a nil Arg. Unreachable from a built container -
// argument validation rejects those first - but reachable through the exported
// Print on a builder's scope, which is exactly what a debugging compiler pass
// does.
func TestPrintingABuilderBeforeAutowiringDoesNotPanic(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	_, err := godi.New().
		Services(godi.Svc(newPrintServer), godi.Svc(newPrintConn), godi.Svc(newPrintConsole)).
		CompilerPasses(di.NewCompilerPass("print", di.PreAutomation, di.CompilerOpFunc(
			func(b *di.ContainerBuilder) error {
				require.NotPanics(t, func() { di.Print(b.RootScope(), &out) })
				return nil
			},
		))).
		Build()
	require.NoError(t, err)

	require.Contains(t, out.String(), "not wired", "nothing has filled the slots yet, and it says so")
}

func TestPrintingAScopePrintsThatScopeAndWhatIsNestedInIt(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	_, err := godi.New().
		Services(
			godi.Svc(newPrintServer).Children(godi.Svc(newPrintConn)),
			godi.Svc(newPrintConsole),
		).
		CompilerPasses(di.NewCompilerPass("print", di.PostFinalization, di.CompilerOpFunc(
			func(b *di.ContainerBuilder) error {
				var child *di.Scope
				for scope := range b.Scopes() {
					if scope.Parent() != nil {
						child = scope
					}
				}
				require.NotNil(t, child, "the server declared a child scope")

				di.Print(child, &out)
				return nil
			},
		))).
		Build()
	require.NoError(t, err)

	// The parent names the child scope, so look at what is registered rather
	// than at the whole text.
	require.Contains(t, out.String(), "di_test.(*printConn)")
	require.NotContains(t, out.String(), "factory: di_test.newPrintServer",
		"the parent scope is not part of the subtree")
}

func TestPrintingNothingWritesNothing(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	di.Print(nil, &out)

	require.Empty(t, out.String())
}
