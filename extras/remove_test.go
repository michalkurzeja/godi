package extras_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/extras"
)

func TestRemoveSvc(t *testing.T) {
	t.Run("can remove a service", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		c, err := di.New().
			Services(
				di.Svc(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.RemoveSvc(&ref),
			).
			Build()
		require.NoError(t, err)

		_, err = di.SvcByRef[string](c, ref)

		require.ErrorContains(t, err, `service string not found`)
	})
	t.Run("returns an error on an empty ref", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		_, err := di.New().
			CompilerPasses(
				extras.RemoveSvc(&ref),
			).
			Build()

		require.ErrorContains(t, err, `compilation failed: compiler pass (remove svc) returned an error: cannot remove service: empty reference`)
	})
}

func TestRemoveFunc(t *testing.T) {
	t.Run("can remove a function", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		c, err := di.New().
			Functions(
				di.Func(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.RemoveFunc(&ref),
			).
			Build()
		require.NoError(t, err)

		_, err = di.ExecByRef(c, ref)

		require.ErrorContains(t, err, `not found`)
	})
	t.Run("returns an error on an empty ref", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		_, err := di.New().
			CompilerPasses(
				extras.RemoveFunc(&ref),
			).
			Build()

		require.ErrorContains(t, err, `compilation failed: compiler pass (remove func) returned an error: cannot remove function: empty reference`)
	})
}
