package di

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/graph"
)

type parityStore struct{}

func newParityStore() *parityStore { return &parityStore{} }

func newParityImplA() testImpl { return testImpl{} }

type parityImplB struct{}

func (parityImplB) Do() {}

func newParityImplB() parityImplB { return parityImplB{} }

func newParityConsumer(testIface, *parityStore, string) *parityConsumer { return nil }

type parityConsumer struct{}

func newParityCollector(...testIface) *parityCollector { return nil }

type parityCollector struct{}

func newParityLabelled(*parityStore) *parityLabelled { return nil }

type parityLabelled struct{}

func newParityIfaceSlice() []testIface { return nil }

func newParitySliceConsumer([]testIface) *paritySliceConsumer { return nil }

type paritySliceConsumer struct{}

// TestGraphEdgesMatchTheResolver is the guard against the extractor's walker
// drifting from the resolver it mirrors. For every argument slot in a compiled
// container, the dependencies the graph shows must be exactly the ones the
// container itself would resolve.
//
// It fails the day anyone changes arg_resolver.go without changing graph.go.
func TestGraphEdgesMatchTheResolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T, builder *ContainerBuilder)
	}{
		{
			name: "literals, refs and autowired types",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				storeFactory, err := NewFactory(newParityStore)
				require.NoError(t, err)
				store := NewServiceDefinition(storeFactory).SetScope(root)

				ref, err := NewRefArg(store)
				require.NoError(t, err)

				consumerFactory, err := NewFactory(newParityConsumer, NewLiteralArg("hello"), ref)
				require.NoError(t, err)

				implFactory, err := NewFactory(newParityImplA)
				require.NoError(t, err)

				root.AddServiceDefinitions(
					store,
					NewServiceDefinition(consumerFactory).SetScope(root),
					NewServiceDefinition(implFactory).SetScope(root),
				)
			},
		},
		{
			name: "an interface godi binds itself",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				storeFactory, err := NewFactory(newParityStore)
				require.NoError(t, err)
				implFactory, err := NewFactory(newParityImplA)
				require.NoError(t, err)
				consumerFactory, err := NewFactory(newParityConsumer, NewLiteralArg("hello"))
				require.NoError(t, err)

				root.AddServiceDefinitions(
					NewServiceDefinition(storeFactory).SetScope(root),
					NewServiceDefinition(implFactory).SetScope(root),
					NewServiceDefinition(consumerFactory).SetScope(root),
				)
			},
		},
		{
			name: "a binding the user declared, with two implementations",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				implFactory, err := NewFactory(newParityImplA)
				require.NoError(t, err)
				impl := NewServiceDefinition(implFactory).SetScope(root)

				otherFactory, err := NewFactory(newParityImplB)
				require.NoError(t, err)

				storeFactory, err := NewFactory(newParityStore)
				require.NoError(t, err)

				consumerFactory, err := NewFactory(newParityConsumer, NewLiteralArg("hello"))
				require.NoError(t, err)

				ref, err := NewRefArg(impl)
				require.NoError(t, err)
				binding, err := NewInterfaceBinding(reflect.TypeFor[testIface](), ref)
				require.NoError(t, err)

				root.AddServiceDefinitions(
					impl,
					NewServiceDefinition(otherFactory).SetScope(root),
					NewServiceDefinition(storeFactory).SetScope(root),
					NewServiceDefinition(consumerFactory).SetScope(root),
				).AddBindings(binding)
			},
		},
		{
			name: "a variadic slot collecting every implementation",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				for _, fn := range []any{newParityCollector, newParityImplA, newParityImplB} {
					factory, err := NewFactory(fn)
					require.NoError(t, err)
					root.AddServiceDefinitions(NewServiceDefinition(factory).SetScope(root))
				}
			},
		},
		{
			// The discriminating case for the flexible slice branch order: a
			// service of the slice type wins outright over the services of the
			// element type, which are never even looked at.
			name: "a slice slot with both a slice service and element services",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				for _, fn := range []any{
					newParitySliceConsumer, newParityIfaceSlice, newParityImplA, newParityImplB,
				} {
					factory, err := NewFactory(fn)
					require.NoError(t, err)
					root.AddServiceDefinitions(NewServiceDefinition(factory).SetScope(root))
				}
			},
		},
		{
			name: "a labelled dependency",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				storeFactory, err := NewFactory(newParityStore)
				require.NoError(t, err)

				consumerFactory, err := NewFactory(newParityLabelled,
					NewLabelArg("primary", reflect.TypeFor[*parityStore](), false))
				require.NoError(t, err)

				root.AddServiceDefinitions(
					NewServiceDefinition(storeFactory).SetScope(root).SetLabels("primary"),
					NewServiceDefinition(consumerFactory).SetScope(root),
				)
			},
		},
		{
			name: "a child scope reaching into its parent",
			build: func(t *testing.T, builder *ContainerBuilder) {
				root := builder.RootScope()

				implFactory, err := NewFactory(newParityImplA)
				require.NoError(t, err)

				consumerFactory, err := NewFactory(newParityConsumer, NewLiteralArg("hello"))
				require.NoError(t, err)
				consumer := NewServiceDefinition(consumerFactory).SetScope(root)

				child := root.NewChild(consumer.ID().String())
				consumer.SetChildScope(child)

				storeFactory, err := NewFactory(newParityStore)
				require.NoError(t, err)
				child.AddServiceDefinitions(NewServiceDefinition(storeFactory).SetScope(child))

				root.AddServiceDefinitions(consumer, NewServiceDefinition(implFactory).SetScope(root))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := NewContainerBuilder(NewConfig())
			tt.build(t, builder)
			require.NoError(t, builder.compiler.Run(builder))

			container := builder.container
			g := container.Graph(graph.Config{})

			requireEdgesMatchResolver(t, container, g)
		})
	}
}

func requireEdgesMatchResolver(t *testing.T, c *Container, g *graph.Graph) {
	t.Helper()

	byParam := make(map[graph.ParamID][]string)
	for _, edge := range g.Edges {
		node, ok := g.Node(edge.To)
		require.True(t, ok, "edge %s -> %s points at a node that is not in the graph", edge.From, edge.To)
		byParam[edge.Param] = append(byParam[edge.Param], node.UUID)
	}

	var checked int
	for _, def := range c.serviceDefsSeq() {
		scope := def.EffectiveScope()

		for _, slot := range def.Factory().Args().Slots() {
			checked++
			requireSlotMatches(t, scope, slot, g, byParam, def.ID(), graph.InjectFactoryArg, "")
		}
		for _, method := range def.MethodCalls() {
			name := shortMethodName(method.Name())
			for _, slot := range method.Args().Slots()[1:] {
				checked++
				requireSlotMatches(t, scope, slot, g, byParam, def.ID(), graph.InjectMethodArg, name)
			}
		}
	}
	require.NotZero(t, checked, "the fixture wired nothing worth checking")
}

func requireSlotMatches(
	t *testing.T,
	scope *Scope,
	slot *Slot,
	g *graph.Graph,
	byParam map[graph.ParamID][]string,
	defID ID,
	kind graph.InjectionKind,
	method string,
) {
	t.Helper()

	var nodeID graph.NodeID
	for _, node := range g.Nodes {
		if node.UUID == defID.String() {
			nodeID = node.ID
			break
		}
	}
	require.NotEmpty(t, nodeID, "definition %s is missing from the graph", defID)

	id := paramID(nodeID, kind, method, int(slot.Index()))

	var want []string
	if slot.IsFilled() {
		for _, resolved := range ResolveArgIDs(scope, slot.Arg()) {
			want = append(want, resolved.String())
		}
	}

	require.ElementsMatch(t, want, byParam[id],
		"param %s: the graph and the resolver disagree about what gets injected", id)
}
