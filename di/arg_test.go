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

// The facade turns a nil literal away with a message, but the engine is an
// extension API: whoever builds one directly gets an error rather than a panic
// out of a method call on the type it does not have.
func TestAnArgumentWithNoTypeFitsNothing(t *testing.T) {
	t.Parallel()

	arg := di.NewLiteralArg(nil)
	require.Nil(t, arg.Type())

	slot := di.NewSlot(reflect.TypeFor[[]string](), 0, false)
	require.False(t, slot.SettableBy(arg))
	require.False(t, slot.AppendableBy(arg))
	require.ErrorContains(t, slot.Fill(arg), "argument <nil> cannot fill slot 0")

	_, err := di.NewCompoundArg(reflect.TypeFor[string](), arg)
	require.ErrorContains(t, err, "argument <nil> cannot be assigned to type string")

	_, err = di.NewInterfaceBinding(reflect.TypeFor[argReporter](), arg)
	require.ErrorContains(t, err, "invalid binding: <nil> does not implement")
}

func TestOnlyASliceArgumentCanBeSpread(t *testing.T) {
	t.Parallel()

	_, err := di.NewSpreadSliceArg(di.NewLiteralArg("foo"))
	require.ErrorContains(t, err, "argument string cannot be spread: it is not a slice")
}

func TestACompoundTakesASpreadSliceByItsElementType(t *testing.T) {
	t.Parallel()

	spread, err := di.NewSpreadSliceArg(di.NewLiteralArg([]string{"foo", "bar"}))
	require.NoError(t, err)

	arg, err := di.NewCompoundArg(reflect.TypeFor[string](), spread)
	require.NoError(t, err)
	require.Equal(t, reflect.TypeFor[[]string](), arg.Type())

	_, err = di.NewCompoundArg(reflect.TypeFor[int](), spread)
	require.ErrorContains(t, err, "argument []string cannot be spread into a slice of int")
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
