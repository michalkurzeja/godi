package di_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
)

type eagerly struct{}

func newEagerly(built *bool) *eagerly {
	*built = true
	return &eagerly{}
}

// The defaults belong to the container they were given to. Two in one process
// disagreeing is the whole point: a package-level default cannot be told apart
// from another test's leftovers.
func TestDefaultsBelongToTheContainerTheyWereGivenTo(t *testing.T) {
	t.Parallel()

	var eager, lazy bool

	_, err := godi.New(godi.DefaultEager()).
		Services(godi.Svc(newEagerly, &eager)).
		Build()
	require.NoError(t, err)
	require.True(t, eager, "the container was told to build everything as it was built")

	_, err = godi.New().
		Services(godi.Svc(newEagerly, &lazy)).
		Build()
	require.NoError(t, err)
	require.False(t, lazy, "and the next container was told nothing of the sort")
}

// What the registration asked for wins: the default fills in what was left
// unsaid and nothing else.
func TestARegistrationOutranksTheDefault(t *testing.T) {
	t.Parallel()

	var built bool

	_, err := godi.New(godi.DefaultEager()).
		Services(godi.Svc(newEagerly, &built).Lazy()).
		Build()
	require.NoError(t, err)
	require.False(t, built, "this one asked to wait")
}

func TestDefaultsReachAChildDefinition(t *testing.T) {
	t.Parallel()

	var parent, child bool

	_, err := godi.New(godi.DefaultEager()).
		Services(godi.Svc(newEagerly, &parent).Children(godi.Svc(newEagerly, &child))).
		Build()
	require.NoError(t, err)
	require.True(t, parent)
	require.True(t, child, "a child is registered by the same call and takes the same defaults")
}

// A definition a compiler pass registers takes the container's defaults, the
// same as one registered up front.
func TestDefaultsReachADefinitionBuiltByACompilerPass(t *testing.T) {
	t.Parallel()

	var built bool

	pass := di.NewCompilerPass("install", di.PreAutomation, di.CompilerOpFunc(
		func(b *di.ContainerBuilder) error {
			return godi.Svc(newEagerly, &built).ParseAndBuild(b.RootScope())
		},
	))

	_, err := godi.New(godi.DefaultEager()).CompilerPasses(pass).Build()
	require.NoError(t, err)
	require.True(t, built, "the container was told to build everything as it was built")
}

func TestARegistrationInACompilerPassOutranksTheDefault(t *testing.T) {
	t.Parallel()

	var built bool

	pass := di.NewCompilerPass("install", di.PreAutomation, di.CompilerOpFunc(
		func(b *di.ContainerBuilder) error {
			return godi.Svc(newEagerly, &built).Lazy().ParseAndBuild(b.RootScope())
		},
	))

	_, err := godi.New(godi.DefaultEager()).CompilerPasses(pass).Build()
	require.NoError(t, err)
	require.False(t, built, "this one asked to wait")
}

// DefaultNotAutowired reaches a pass-built definition too, so godi does not fill
// an argument the registration left for the caller.
func TestDefaultNotAutowiredReachesADefinitionBuiltByACompilerPass(t *testing.T) {
	t.Parallel()

	var built bool

	pass := di.NewCompilerPass("install", di.PreAutomation, di.CompilerOpFunc(
		func(b *di.ContainerBuilder) error {
			return godi.Svc(newEagerly).ParseAndBuild(b.RootScope())
		},
	))

	_, err := godi.New(godi.DefaultNotAutowired()).
		Services(godi.Svc(func() *bool { return &built })).
		CompilerPasses(pass).
		Build()
	require.ErrorContains(t, err, "argument 0 is not set")
}

func TestAPassCanReadTheContainersDefaults(t *testing.T) {
	t.Parallel()

	var defaults di.Defaults

	pass := di.NewCompilerPass("read", di.PreAutomation, di.CompilerOpFunc(
		func(b *di.ContainerBuilder) error {
			defaults = b.Defaults()
			return nil
		},
	))

	_, err := godi.New(godi.DefaultEager(), godi.DefaultNotShared()).CompilerPasses(pass).Build()
	require.NoError(t, err)
	require.Equal(t, di.Defaults{Lazy: false, Shared: false, Autowired: true}, defaults)
}
