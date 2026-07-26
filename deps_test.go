package di_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Nothing a godi binary links should be able to draw a graph. Renderers mean an
// embedded Graphviz and a viewer nobody asked for.
//
// The rule is what lets the failure snapshot exist at all: it writes JSON,
// because JSON is the only thing this package can produce without breaking the
// rule. Leaving it as a comment in CLAUDE.md made it something to remember, so
// it is a test instead.
func TestTheLibraryCarriesNoRenderersAndCannotRunPrograms(t *testing.T) {
	t.Parallel()

	deps := depsOf(t, "github.com/michalkurzeja/godi/v2")
	for _, pkg := range []string{
		"github.com/michalkurzeja/godi/v2/graph/dot",
		"github.com/michalkurzeja/godi/v2/graph/html",
		"github.com/michalkurzeja/godi/v2/graph/serve",
		"github.com/michalkurzeja/godi/v2/graph/text",
		"os/exec",
		// cmd/godi is in this module, so cobra is a requirement of it. Nothing
		// but the CLI may import it: a library that drags a command-line
		// framework into a program is a library nobody wants.
		"github.com/spf13/cobra",
		"github.com/spf13/pflag",
	} {
		require.NotContains(t, deps, pkg, "every godi binary would carry %s", pkg)
	}
}

// The model is a leaf, and that is what makes it worth having as a wire format:
// a tool that only reads a graph - the godi CLI reading a file above all -
// carries no container engine, and neither does a third-party encoder.
func TestTheGraphModelCarriesNoEngine(t *testing.T) {
	t.Parallel()

	for _, pkg := range []string{
		"github.com/michalkurzeja/godi/v2/graph",
		"github.com/michalkurzeja/godi/v2/graph/dot",
		"github.com/michalkurzeja/godi/v2/graph/text",
		"github.com/michalkurzeja/godi/v2/graph/html",
	} {
		deps := depsOf(t, pkg)
		require.NotContains(t, deps, "github.com/michalkurzeja/godi/v2/di", "%s would carry the engine", pkg)
		require.NotContains(t, deps, "github.com/michalkurzeja/godi/v2", "%s would carry the facade", pkg)
	}
}

// No godi library package may start a process, and this is asserted over the
// library as a whole rather than over the root package: it is now true by
// construction - the only os/exec left in the module is in cmd/godi, where a
// command-line tool opening a browser is unremarkable - and this is what keeps
// it that way.
func TestNothingInTheLibraryCanStartAProcess(t *testing.T) {
	t.Parallel()

	for _, pkg := range packagesOf(t, "./...") {
		if strings.HasPrefix(pkg, "github.com/michalkurzeja/godi/v2/cmd/") ||
			strings.HasPrefix(pkg, "github.com/michalkurzeja/godi/v2/examples/") {
			continue
		}
		require.NotContains(t, depsOf(t, pkg), "os/exec", "%s can start a process", pkg)
	}
}

func packagesOf(t *testing.T, pattern string) []string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain to ask about packages")
	}

	out, err := exec.Command("go", "list", pattern).Output()
	require.NoError(t, err)

	return strings.Fields(string(out))
}

func depsOf(t *testing.T, pkg string) []string {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain to ask about dependencies")
	}

	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	require.NoError(t, err)

	return strings.Fields(string(out))
}
