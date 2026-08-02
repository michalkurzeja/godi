package di

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// A nil carries no type, so reflect answers it with the zero Value and will
// neither pass nor append that. What the value is going into is the only thing
// that says what the nil means.
func TestANilTakesTheTypeOfWhateverWantsIt(t *testing.T) {
	t.Parallel()

	t.Run("a nil is the zero value of a type it can be nil in", func(t *testing.T) {
		t.Parallel()

		v, err := valueOf(nil, reflect.TypeFor[[]string]())
		require.NoError(t, err)
		require.Equal(t, reflect.TypeFor[[]string](), v.Type())
		require.True(t, v.IsNil())
	})
	// Not reachable from a container, since a service of a type that cannot be
	// nil never is one. A zero here would be a number the container never found.
	t.Run("a nil is not the zero value of a type it cannot be nil in", func(t *testing.T) {
		t.Parallel()

		_, err := valueOf(nil, reflect.TypeFor[int]())
		require.ErrorContains(t, err, "nil is not a value of type int")
	})
	t.Run("anything else is itself", func(t *testing.T) {
		t.Parallel()

		v, err := valueOf("foo", reflect.TypeFor[string]())
		require.NoError(t, err)
		require.Equal(t, "foo", v.Interface())
	})
}
