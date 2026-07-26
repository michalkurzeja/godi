package di

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

type preparedSvc struct{}

func newPreparedSvc() *preparedSvc { return &preparedSvc{} }

// A prepared builder must be prepared: everything the facade was told about is
// in the engine, passes included. Building through the container builder is the
// route that used to drop them, and it is the one a graph of a prepared builder
// describes.
func TestAPreparedBuilderAlreadyKnowsItsCompilerPasses(t *testing.T) {
	t.Parallel()

	var ran bool
	pass := di.NewCompilerPass("marker", di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		ran = true
		return nil
	}))

	b := New().Services(Svc(newPreparedSvc)).CompilerPasses(pass)
	require.NoError(t, b.prepare())

	_, err := b.cb.Build()
	require.NoError(t, err)
	require.True(t, ran, "the pass was registered before the builder was prepared")
}

// Preparing twice must not register a pass twice, for the same reason it must
// not build a definition twice.
func TestPreparingAgainDoesNotRegisterAPassTwice(t *testing.T) {
	t.Parallel()

	var runs int
	pass := di.NewCompilerPass("counter", di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		runs++
		return nil
	}))

	b := New().Services(Svc(newPreparedSvc)).CompilerPasses(pass)
	require.NoError(t, b.prepare())
	require.NoError(t, b.prepare())

	_, err := b.Build()
	require.NoError(t, err)
	require.Equal(t, 1, runs)
}
