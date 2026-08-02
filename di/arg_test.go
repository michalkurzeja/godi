package di_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

type argReporter interface{ report() }

type argConsole struct{}

func (argConsole) report() {}

// A compound assembles a slice, so that is what it has to report. Saying it is
// the element type used to route it to Slot.Append, which wrapped it in a second
// compound and failed at instantiation.
func TestACompoundArgIsTypedAsTheSliceItBuilds(t *testing.T) {
	t.Parallel()

	arg, err := di.NewCompoundArg(reflect.TypeFor[string](), di.NewLiteralArg("foo"))
	require.NoError(t, err)
	require.Equal(t, reflect.TypeFor[[]string](), arg.Type())
}

func TestACompoundArgTakesNoArgumentOfAnotherType(t *testing.T) {
	t.Parallel()

	_, err := di.NewCompoundArg(reflect.TypeFor[string](), di.NewLiteralArg(42))
	require.ErrorContains(t, err, "argument int cannot be assigned to type string")
}

// A binding says what an interface resolves to, and a slice argument resolving
// through one expects a slice back. So a slice of the interface is a valid
// target, which is what BindSlice binds.
func TestAnInterfaceCanBeBoundToASliceOfItself(t *testing.T) {
	t.Parallel()

	iface := reflect.TypeFor[argReporter]()

	compound, err := di.NewCompoundArg(iface, di.NewLiteralArg(argConsole{}))
	require.NoError(t, err)

	binding, err := di.NewInterfaceBinding(iface, compound)
	require.NoError(t, err)
	require.Equal(t, iface, binding.Interface())
}

func TestAnInterfaceCannotBeBoundToSomethingUnrelated(t *testing.T) {
	t.Parallel()

	_, err := di.NewInterfaceBinding(reflect.TypeFor[argReporter](), di.NewLiteralArg("foo"))
	require.ErrorContains(t, err, "invalid binding: string does not implement github.com/michalkurzeja/godi/v2/di_test.argReporter, and is not a slice of it")
}
