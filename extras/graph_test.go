package extras_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
	engine "github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/extras"
	"github.com/michalkurzeja/godi/v2/graph"
)

type store struct{}

type server struct{}

func newStore() *store         { return &store{} }
func newServer(*store) *server { return &server{} }

func TestCaptureGraph(t *testing.T) {
	t.Run("hands over the graph as it stands at that stage", func(t *testing.T) {
		t.Parallel()

		var before, after *graph.Graph

		_, err := di.New().
			Services(
				di.Svc(newServer),
				di.Svc(newStore),
			).
			CompilerPasses(
				extras.CaptureGraph(engine.PreAutomation, func(g *graph.Graph) error {
					before = g
					return nil
				}),
				extras.CaptureGraph(engine.PreValidation, func(g *graph.Graph) error {
					after = g
					return nil
				}),
			).
			Build()
		require.NoError(t, err)

		require.True(t, before.Partial())
		require.Empty(t, before.Edges, "autowiring has not run yet")

		require.True(t, after.Partial())
		require.Len(t, after.Edges, 1, "autowiring has run by now")
		require.Contains(t, after.Snapshot.Done, "autowiring")
	})

	t.Run("passes the extraction options on", func(t *testing.T) {
		t.Parallel()

		var seen *graph.Graph

		_, err := di.New().
			Services(di.Svc(newServer), di.Svc(newStore)).
			CompilerPasses(extras.CaptureGraph(engine.PreValidation, func(g *graph.Graph) error {
				seen = g
				return nil
			}, graph.WithoutLiterals())).
			Build()
		require.NoError(t, err)

		require.NotNil(t, seen)
	})

	t.Run("a failure to capture fails the build", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			CompilerPasses(extras.CaptureGraph(engine.PreAutomation, func(*graph.Graph) error {
				return errors.New("nowhere to put it")
			})).
			Build()

		require.ErrorContains(t, err, "compiler pass (graph snapshot) returned an error: nowhere to put it")
	})

	t.Run("returns an error when given nothing to capture with", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			CompilerPasses(extras.CaptureGraph(engine.PreAutomation, nil)).
			Build()

		require.ErrorContains(t, err, "cannot capture the graph: no capture function")
	})
}
