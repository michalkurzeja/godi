package extras_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/extras"
)

func TestOverrideSvcArg(t *testing.T) {
	t.Run("can override an arg value", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		c, err := di.New().
			Services(
				di.Svc(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, 42),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("can override an interface arg with a value", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference
		sprint := func(v any) string { return fmt.Sprint(v) }

		c, err := di.New().
			Services(
				di.Svc(sprint, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, "foo"),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "foo", got)
	})
	t.Run("can override a variadic arg", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		c, err := di.New().
			Services(
				di.Svc(fmt.Sprint, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "foo", got)
	})
	t.Run("can override autowired argument", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		c, err := di.New().
			Services(
				di.SvcVal(0),
				di.Svc(strconv.Itoa).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, 42),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("can override autowired interface argument", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference
		sprint := func(v any) string { return fmt.Sprint(v) }

		c, err := di.New().
			Services(
				di.SvcVal(0),
				di.Svc(sprint).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, "foo"),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "foo", got)
	})
	t.Run("can override an autowired variadic arg", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		c, err := di.New().
			Services(
				di.SvcVal(0),
				di.Svc(fmt.Sprint).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "foo", got)
	})
	t.Run("returns an error when the service is not found", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		_, _ = di.New().Services(
			di.Svc(strconv.Itoa, 0).Bind(&ref),
		).Build()

		_, err := di.New().
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of string: service not found")
	})
	t.Run("returns an error when slot is out of bounds", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		_, err := di.New().
			Services(
				di.Svc(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 1, 42),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of string: argument int is assigned to slot 1, but function has only 1 argument slots")
	})
	t.Run("returns an error when argument is not assignable to slot", func(t *testing.T) {
		t.Parallel()

		var ref di.SvcReference

		_, err := di.New().
			Services(
				di.Svc(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideSvcArg(ref, 0, "foo"),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of string: argument string cannot be assigned to slot 0")
	})
}

func TestOverrideFuncArg(t *testing.T) {
	t.Run("can override an arg value", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		c, err := di.New().
			Functions(
				di.Func(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, 42),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"42"}, got)
	})
	t.Run("can override an interface arg with a value", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference
		sprint := func(v any) string { return fmt.Sprint(v) }

		c, err := di.New().
			Functions(
				di.Func(sprint, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, "foo"),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"foo"}, got)
	})
	t.Run("can override a variadic arg", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		c, err := di.New().
			Functions(
				di.Func(fmt.Sprint, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"foo"}, got)
	})
	t.Run("can override autowired argument", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		c, err := di.New().
			Services(
				di.SvcVal(0),
			).
			Functions(
				di.Func(strconv.Itoa).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, 42),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"42"}, got)
	})
	t.Run("can override autowired interface argument", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference
		sprint := func(v any) string { return fmt.Sprint(v) }

		c, err := di.New().
			Services(
				di.SvcVal(0),
			).
			Functions(
				di.Func(sprint).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, "foo"),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"foo"}, got)
	})
	t.Run("can override an autowired variadic arg", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		c, err := di.New().
			Services(
				di.SvcVal(0),
			).
			Functions(
				di.Func(fmt.Sprint).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.NoError(t, err)

		got, err := di.ExecByRef(c, ref)
		require.NoError(t, err)
		require.Equal(t, []any{"foo"}, got)
	})
	t.Run("returns an error when the service is not found", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		_, _ = di.New().Functions(
			di.Func(strconv.Itoa, 0).Bind(&ref),
		).Build()

		_, err := di.New().
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, []any{"foo"}),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of strconv.Itoa: function not found")
	})
	t.Run("returns an error when slot is out of bounds", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		_, err := di.New().
			Functions(
				di.Func(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 1, 42),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of strconv.Itoa: argument int is assigned to slot 1, but function has only 1 argument slots")
	})
	t.Run("returns an error when argument is not assignable to slot", func(t *testing.T) {
		t.Parallel()

		var ref di.FuncReference

		_, err := di.New().
			Functions(
				di.Func(strconv.Itoa, 0).Bind(&ref),
			).
			CompilerPasses(
				extras.OverrideFuncArg(ref, 0, "foo"),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (override arg) returned an error: cannot override argument of strconv.Itoa: argument string cannot be assigned to slot 0")
	})
}
