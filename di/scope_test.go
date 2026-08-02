package di_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

type plugin struct{}

func newPlugin() *plugin { return &plugin{} }

type otherPlugin struct{}

func newOtherPlugin() *otherPlugin { return &otherPlugin{} }

// Two scopes of one name used to leave the container holding only the second. The
// first went on existing in the parent tree, so nothing complained.
//
// Everything that walks the container then missed it: argument validation, eager
// initialisation and the graph alike.
func TestASecondScopeOfTheSameNameDoesNotReplaceTheFirst(t *testing.T) {
	t.Parallel()

	var built []string

	pass := di.NewCompilerPass("scopes", di.PreAutomation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		first := b.RootScope().NewChild("plugins")
		second := b.RootScope().NewChild("plugins")
		require.NotSame(t, first, second)

		add := func(scope *di.Scope, factory any) error {
			f, err := di.NewFactory(factory)
			if err != nil {
				return err
			}
			scope.AddServiceDefinitions(di.NewServiceDefinition(f).SetScope(scope).SetLazy(false))
			return nil
		}
		if err := add(first, newPlugin); err != nil {
			return err
		}
		return add(second, newOtherPlugin)
	}))

	count := di.NewCompilerPass("count", di.PostFinalization, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		for scope := range b.Scopes() {
			built = append(built, scope.Name())
		}
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(pass)
	b.Compiler().AddPass(count)

	c, err := b.Build()
	require.NoError(t, err)

	require.Equal(t, []string{"root", "plugins", "plugins#2"}, built,
		"both scopes are the container's, and the second is named apart")

	first, ok := c.Scope("plugins")
	require.True(t, ok)
	require.Len(t, first.GetServiceDefinitions(), 1)

	second, ok := c.Scope("plugins#2")
	require.True(t, ok)
	require.Len(t, second.GetServiceDefinitions(), 1, "the definitions of the second scope are still there")
}

// ExecuteFunctionInChain used to look the function up in the scope it was called
// on rather than in the one the walk had reached, so it never saw past that
// scope. HasFunctionInChain said the function was there and executing it failed.
func TestAChildScopeCanExecuteAFunctionOfItsParent(t *testing.T) {
	t.Parallel()

	var called bool

	fn, err := di.NewFunc(reflect.ValueOf(func() { called = true }))
	require.NoError(t, err)

	c := di.NewContainer()
	root := c.Root()
	def := di.NewFunctionDefinition(fn).SetScope(root)
	root.AddFunctionDefinitions(def)

	child := root.NewChild("child")
	require.True(t, child.HasFunctionInChain(def.ID()))

	_, err = child.ExecuteFunctionInChain(def.ID())
	require.NoError(t, err)
	require.True(t, called)
}

func TestExecutingAFunctionNoScopeInTheChainHasFails(t *testing.T) {
	t.Parallel()

	c := di.NewContainer()
	child := c.Root().NewChild("child")

	_, err := child.ExecuteFunctionInChain(di.NewID())
	require.ErrorContains(t, err, "not found")
}
