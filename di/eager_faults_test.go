package di_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// A failure while building eagerly is discovered at the far end of a chain of
// factories, and most of what turns up there is a fault in one argument: a label
// that names a service of the wrong type, a nil where the type has no zero. The
// definition is where the pass used to report them, because the definition was
// all it had.
//
// These pin where a failure lands, and what the reader is told when it does.

type eagerStore struct{}

type eagerHolder struct{}

// eagerFaults builds a container of the given definitions eagerly and returns
// what the eager initialisation pass reported.
func eagerFaults(t *testing.T, defs ...*di.ServiceDefinition) []di.Diagnostic {
	t.Helper()

	b := di.NewContainerBuilder(di.NewConfig())
	for _, def := range defs {
		def.SetLazy(false)
		def.SetScope(b.RootScope())
	}
	b.RootScope().AddServiceDefinitions(defs...)

	container := b.Container()
	_, err := b.Build()
	require.Error(t, err, "the container is meant to fail")

	var reported []di.Diagnostic
	for _, d := range container.Diagnostics() {
		if d.Pass == "eager initialization" {
			reported = append(reported, d)
		}
	}
	return reported
}

func svcDef(t *testing.T, factory any, args ...di.Arg) *di.ServiceDefinition {
	t.Helper()

	f, err := di.NewFactory(factory, args...)
	require.NoError(t, err)
	return di.NewServiceDefinition(f)
}

// The wiring is correct as far as anything can tell before the container runs:
// the label names a service that exists. What it does not name is its type.
func TestAnArgumentThatFailsToResolveIsReportedAgainstThatArgument(t *testing.T) {
	t.Parallel()

	store := svcDef(t, func() *eagerStore { return &eagerStore{} })
	store.SetLabels("greeter")

	holder := svcDef(t, func(s string) *eagerHolder { return &eagerHolder{} },
		di.NewLabelArg("greeter", reflect.TypeFor[string](), false))
	holder.SetAutowired(false)

	reported := eagerFaults(t, store, holder)
	require.Len(t, reported, 1)

	d := reported[0]
	require.Equal(t, holder, d.Site.Service(), "the argument belongs to the service that would not build")
	require.NotNil(t, d.Site.Slot(), "the fault is about one argument, and the site has to say which")
	require.Equal(t, uint(0), d.Site.Slot().Index())

	// The element it is shown against already says which argument of which
	// service. What is left to say is the fault.
	require.Equal(t, "service labeled as greeter should be of type string, got github.com/michalkurzeja/godi/v2/di_test.(*eagerStore)", d.Message)

	// The error the build fails with keeps the whole chain, because it has to
	// stand alone in a terminal.
	require.ErrorContains(t, d.Err, "failed to initialise eager service")
	require.ErrorContains(t, d.Err, "failed to resolve argument 0")
	require.ErrorContains(t, d.Err, "should be of type string")
}

// A factory that fails on its own account implicates no argument. Reporting it
// against one would send a reader to change wiring that is correct.
func TestAFactoryThatReturnsAnErrorIsReportedAgainstTheService(t *testing.T) {
	t.Parallel()

	broken := svcDef(t, func() (*eagerStore, error) { return nil, errors.New("connection refused") })

	reported := eagerFaults(t, broken)
	require.Len(t, reported, 1)

	d := reported[0]
	require.Equal(t, broken, d.Site.Service())
	require.Nil(t, d.Site.Slot(), "no argument is at fault")
	require.Contains(t, d.Message, "connection refused")
	require.Contains(t, d.Message, "failed to initialise eager service",
		"with no argument to sit beside, the message says what was being built")
}

// A service fails because its dependency fails. Two arguments are named on the
// way out, and only the inner one is worth changing: the wiring to the broken
// dependency is correct, and the dependency is not.
func TestTheInnermostArgumentIsTheOneReported(t *testing.T) {
	t.Parallel()

	store := svcDef(t, func() *eagerStore { return &eagerStore{} })
	store.SetLabels("greeter")

	// Fails on its own argument, exactly as above.
	inner := svcDef(t, func(s string) *eagerHolder { return &eagerHolder{} },
		di.NewLabelArg("greeter", reflect.TypeFor[string](), false))
	inner.SetAutowired(false)

	// Correctly wired to something that will not build.
	outer := svcDef(t, func(h *eagerHolder) *eagerStore { return &eagerStore{} })
	outer.SetAutowired(false)
	ref, err := di.NewRefArg(inner)
	require.NoError(t, err)
	require.NoError(t, outer.Factory().AddArgs(ref))

	reported := eagerFaults(t, store, inner, outer)
	require.NotEmpty(t, reported)

	d := reported[0]
	require.Equal(t, inner, d.Site.Service(), "the outer service is wired correctly to a broken one")
	require.Equal(t, "service labeled as greeter should be of type string, got github.com/michalkurzeja/godi/v2/di_test.(*eagerStore)", d.Message,
		"the innermost fault is the bare one; the outer levels are how the container got there")
}
