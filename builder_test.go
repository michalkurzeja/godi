package di_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
)

// The builder registers nothing until it is prepared, which is what makes the
// order and the number of Services/Functions/Bindings calls irrelevant. Both
// tests below pass by construction today; they are here so that a builder
// "simplified" into registering eagerly says so.

type deferredDep struct{}

func newDeferredDep() *deferredDep { return &deferredDep{} }

func (*deferredDep) doIt() {}

type deferredIface interface{ doIt() }

type deferredSvc struct{ dep *deferredDep }

func newDeferredSvc(dep *deferredDep) *deferredSvc { return &deferredSvc{dep: dep} }

type deferredUser struct{ iface deferredIface }

func newDeferredUser(iface deferredIface) *deferredUser { return &deferredUser{iface: iface} }

func TestAServiceCanReferToOneRegisteredByALaterCall(t *testing.T) {
	t.Parallel()

	var ref di.SvcReference

	c, err := di.New().
		Services(di.Svc(newDeferredSvc, di.Ref(&ref))).
		Services(di.Svc(newDeferredDep).Bind(&ref)).
		Build()
	require.NoError(t, err)

	svc, err := di.SvcByType[*deferredSvc](c)
	require.NoError(t, err)

	dep, err := di.SvcByType[*deferredDep](c)
	require.NoError(t, err)
	require.Same(t, dep, svc.dep)
}

func TestABindingCanNameAServiceRegisteredAfterIt(t *testing.T) {
	t.Parallel()

	var ref di.SvcReference

	c, err := di.New().
		Bindings(di.BindArg[deferredIface](di.Ref(&ref))).
		Services(di.Svc(newDeferredUser)).
		Services(di.Svc(newDeferredDep).Bind(&ref)).
		Build()
	require.NoError(t, err)

	user, err := di.SvcByType[*deferredUser](c)
	require.NoError(t, err)

	dep, err := di.SvcByType[*deferredDep](c)
	require.NoError(t, err)
	require.Same(t, dep, user.iface)
}
