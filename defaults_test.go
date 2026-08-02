package di_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
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

	_, err := di.New(di.DefaultEager()).
		Services(di.Svc(newEagerly, &eager)).
		Build()
	require.NoError(t, err)
	require.True(t, eager, "the container was told to build everything as it was built")

	_, err = di.New().
		Services(di.Svc(newEagerly, &lazy)).
		Build()
	require.NoError(t, err)
	require.False(t, lazy, "and the next container was told nothing of the sort")
}

// What the registration asked for wins: the default fills in what was left
// unsaid and nothing else.
func TestARegistrationOutranksTheDefault(t *testing.T) {
	t.Parallel()

	var built bool

	_, err := di.New(di.DefaultEager()).
		Services(di.Svc(newEagerly, &built).Lazy()).
		Build()
	require.NoError(t, err)
	require.False(t, built, "this one asked to wait")
}

func TestDefaultsReachAChildDefinition(t *testing.T) {
	t.Parallel()

	var parent, child bool

	_, err := di.New(di.DefaultEager()).
		Services(di.Svc(newEagerly, &parent).Children(di.Svc(newEagerly, &child))).
		Build()
	require.NoError(t, err)
	require.True(t, parent)
	require.True(t, child, "a child is registered by the same call and takes the same defaults")
}
