package di_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
)

type provDep struct{}

func newProvDep() *provDep { return &provDep{} }

func (*provDep) speak() {}

type provIface interface{ speak() }

type provSvc struct{}

func newProvSvc(_ *provDep, _ provIface, _ string) *provSvc { return &provSvc{} }

// Telling an argument the user wrote from one godi wired is the whole point of
// recording where it came from, and a pass that wants to replace only what
// autowiring chose has no other way to know.
func TestAPassCanReadWhoWiredWhat(t *testing.T) {
	t.Parallel()

	origins := make(map[uint]di.ArgOrigin)
	var (
		bindOrigin di.BindOrigin
		bindPass   string
	)

	pass := di.NewCompilerPass("inspect", di.PreValidation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		for _, def := range b.ServiceDefinitionsSeq() {
			if def.Type() != reflect.TypeFor[*provSvc]() {
				continue
			}
			for _, slot := range def.Factory().Args().Slots() {
				origin, _ := slot.Origin()
				origins[slot.Index()] = origin
			}
		}
		for _, binding := range b.RootScope().GetBindings() {
			bindOrigin, bindPass = binding.Origin()
		}
		return nil
	}))

	_, err := godi.New().
		Services(
			godi.Svc(newProvDep),
			godi.Svc(newProvSvc, godi.Val("written by hand").Slot(2)),
		).
		CompilerPasses(pass).
		Build()
	require.NoError(t, err)

	require.Equal(t, di.ArgOriginAutowiring, origins[0])
	require.Equal(t, di.ArgOriginAutowiring, origins[1])
	require.Equal(t, di.ArgOriginManual, origins[2])

	require.Equal(t, di.BindOriginAutobinding, bindOrigin, "the binding pass created this one")
	require.Equal(t, "interface binding", bindPass)
}
