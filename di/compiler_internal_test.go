package di

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type testIface interface{ Do() }

type testImpl struct{}

func (testImpl) Do() {}

func newTestImpl() testImpl { return testImpl{} }

func newTestConsumer(testIface, string) *testConsumer { return nil }

type testConsumer struct{}

// buildAttributed compiles a container out of the given definitions and returns
// the builder, so that the test can inspect the attribution left on it.
// It compiles rather than calling Build, because Build nils out the container.
func buildAttributed(t *testing.T, passes []*CompilerPass, defs ...*ServiceDefinition) (*ContainerBuilder, error) {
	t.Helper()

	builder := NewContainerBuilder(NewConfig())
	for _, def := range defs {
		def.SetScope(builder.RootScope())
	}
	builder.RootScope().AddServiceDefinitions(defs...)
	for _, pass := range passes {
		builder.Compiler().AddPass(pass)
	}

	return builder, builder.compiler.Run(builder)
}

func TestSlotOriginIsManualWhenWiredAtDefinitionTime(t *testing.T) {
	t.Parallel()

	factory, err := NewFactory(newTestConsumer, NewLiteralArg("hello"))
	require.NoError(t, err)
	impl, err := NewFactory(newTestImpl)
	require.NoError(t, err)

	_, err = buildAttributed(t, nil, NewServiceDefinition(factory), NewServiceDefinition(impl))
	require.NoError(t, err)

	slots := factory.Args().Slots()

	// Slot 0 is the interface: nobody filled it, so autowiring did.
	require.Equal(t, ArgOriginAutowiring, slots[0].origin)
	require.Equal(t, "autowiring", slots[0].originPass)

	// Slot 1 was given a literal by hand.
	require.Equal(t, ArgOriginManual, slots[1].origin)
	require.Empty(t, slots[1].originPass)
}

func TestBindingOriginDistinguishesAutobindingFromUserBinding(t *testing.T) {
	t.Parallel()

	ifaceTyp := reflect.TypeFor[testIface]()

	t.Run("created by the interface binding pass", func(t *testing.T) {
		t.Parallel()

		factory, err := NewFactory(newTestConsumer, NewLiteralArg("hello"))
		require.NoError(t, err)
		impl, err := NewFactory(newTestImpl)
		require.NoError(t, err)

		builder, err := buildAttributed(t, nil, NewServiceDefinition(factory), NewServiceDefinition(impl))
		require.NoError(t, err)

		binding, ok := builder.RootScope().GetBinding(ifaceTyp)
		require.True(t, ok)
		require.Equal(t, BindOriginAutobinding, binding.origin)
		require.Equal(t, "interface binding", binding.originPass)
	})

	t.Run("declared by the user", func(t *testing.T) {
		t.Parallel()

		factory, err := NewFactory(newTestConsumer, NewLiteralArg("hello"))
		require.NoError(t, err)
		implFactory, err := NewFactory(newTestImpl)
		require.NoError(t, err)
		implDef := NewServiceDefinition(implFactory)

		ref, err := NewRefArg(implDef)
		require.NoError(t, err)
		declared, err := NewInterfaceBinding(ifaceTyp, ref)
		require.NoError(t, err)

		builder := NewContainerBuilder(NewConfig())
		consumerDef := NewServiceDefinition(factory).SetScope(builder.RootScope())
		implDef.SetScope(builder.RootScope())
		builder.RootScope().
			AddServiceDefinitions(consumerDef, implDef).
			AddBindings(declared)
		require.NoError(t, builder.compiler.Run(builder))

		binding, ok := builder.RootScope().GetBinding(ifaceTyp)
		require.True(t, ok)
		require.Equal(t, BindOriginManual, binding.origin)
		require.Empty(t, binding.originPass)
	})
}

func TestSlotOriginCreditsThirdPartyPassByName(t *testing.T) {
	t.Parallel()

	factory, err := NewFactory(newTestConsumer, NewLiteralArg("hello"))
	require.NoError(t, err)
	impl, err := NewFactory(newTestImpl)
	require.NoError(t, err)

	// Runs after autowiring and replaces the literal it had nothing to do with.
	override := NewCompilerPass("my override", PreValidation, CompilerOpFunc(func(b *ContainerBuilder) error {
		return factory.Args().SetSlot(NewSlottedArg(NewLiteralArg("overridden"), 1))
	}))

	_, err = buildAttributed(t, []*CompilerPass{override}, NewServiceDefinition(factory), NewServiceDefinition(impl))
	require.NoError(t, err)

	slots := factory.Args().Slots()
	require.Equal(t, ArgOriginCompilerPass, slots[1].origin)
	require.Equal(t, "my override", slots[1].originPass)

	// The autowired slot is untouched by the override pass.
	require.Equal(t, ArgOriginAutowiring, slots[0].origin)
	require.Equal(t, "autowiring", slots[0].originPass)
}

// A pass that objects to one argument still wired everything else it handles, and
// the graph of a build that stopped is where that difference matters most. Leaving
// its work unattributed reads as wiring the user did.
func TestAFailingPassIsStillCreditedWithWhatItWired(t *testing.T) {
	t.Parallel()

	factory, err := NewFactory(newTestConsumer, NewLiteralArg("hello"))
	require.NoError(t, err)
	impl, err := NewFactory(newTestImpl)
	require.NoError(t, err)

	// Replaces the literal, then objects to the argument beside it.
	failing := NewCompilerPass("my override", PreValidation, CompilerOpFunc(func(b *ContainerBuilder) error {
		if err := factory.Args().SetSlot(NewSlottedArg(NewLiteralArg("overridden"), 1)); err != nil {
			return err
		}
		for _, def := range b.ServiceDefinitionsSeq() {
			if def.Type() == reflect.TypeFor[*testConsumer]() {
				b.ReportError(AtServiceArg(def, def.Factory().Args().Slots()[0]), errors.New("not good enough"), "")
			}
		}
		return nil
	}))

	builder, err := buildAttributed(t, []*CompilerPass{failing}, NewServiceDefinition(factory), NewServiceDefinition(impl))
	require.ErrorContains(t, err, "not good enough")

	slots := factory.Args().Slots()
	require.Equal(t, ArgOriginCompilerPass, slots[1].origin)
	require.Equal(t, "my override", slots[1].originPass)

	progress := builder.Compiler().Progress()
	require.Equal(t, "my override", progress.Failed)
	require.NotContains(t, progress.Done, "my override", "a pass that stopped the build has not finished")
}
