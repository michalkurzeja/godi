package di_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi"
)

func TestContainer(t *testing.T) {
	t.Run("can register simple services with autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.SvcT[*NoDepSvc](NewNoDepSvc),
			di.SvcT[*DepSvc](NewDepSvc).
				Args(di.Val("prop")).
				Autowired().
				Lazy().
				Public(),
		).Build()
		require.NoError(t, err)

		bar, err := di.Get[*DepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &DepSvc{dep: NewNoDepSvc(), prop: "prop"}, bar)
	})
	t.Run("can register simple services without autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(di.Ref[*NoDepSvc](), di.Val("prop")).
				NotAutowired().
				Public(),
		).Build()
		require.NoError(t, err)

		bar, err := di.Get[*DepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &DepSvc{dep: NewNoDepSvc(), prop: "prop"}, bar)
	})
	t.Run("can register the same service twice with different IDs", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("foo").Public(),
			di.Svc(NewNoDepSvc).ID("bar").Public(),
		).Build()
		require.NoError(t, err)

		foo, err := di.Get[*NoDepSvc](c, di.WithID("foo"))
		require.NoError(t, err)

		bar, err := di.Get[*NoDepSvc](c, di.WithID("bar"))
		require.NoError(t, err)

		assert.NotSame(t, foo, bar)
	})
	t.Run("can register a service with slice factory argument", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewStrSliceDepSvc).
				Args(di.Val([]string{"foo", "bar"})).
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*StrSliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &StrSliceDepSvc{props: []string{"foo", "bar"}}, svc)
	})
	t.Run("can register a service with variadic factory argument", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewStrSliceDepSvcVariadic).
				Args(di.Val([]string{"foo", "bar"})).
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*StrSliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &StrSliceDepSvc{props: []string{"foo", "bar"}}, svc)
	})
	t.Run("can register a service with scalar argument and zero value", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewStrDepSvc).
				Args(di.Zero()).
				Public(),
		).Build()
		require.NoError(t, err)

		svc1, err := di.Get[*StrDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &StrDepSvc{}, svc1)
	})
	t.Run("can register a service with slice factory argument and zero value", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewStrSliceDepSvc).
				Args(di.Zero()).
				Public(),
			di.Svc(NewSliceDepSvc).
				Args(di.Zero()).
				Public(),
		).Build()
		require.NoError(t, err)

		svc1, err := di.Get[*StrSliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &StrSliceDepSvc{}, svc1)

		svc2, err := di.Get[*SliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &SliceDepSvc{}, svc2)
	})
	t.Run("can register a service with method call with autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDep").
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*MethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &MethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with method call without autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDep", di.Ref[*NoDepSvc]()).
				NotAutowired().
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*MethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &MethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with method call that can return error", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDepE").
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*MethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &MethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can alias a service by type", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(di.Svc(NewNoDepSvc).ID("bar").Public()).
			Aliases(di.NewAliasT[Fooer]("bar")).
			Build()
		require.NoError(t, err)

		svc1, err := di.Get[*NoDepSvc](c, di.WithID("bar"))
		require.NoError(t, err)

		svc2, err := di.Get[Fooer](c)
		require.NoError(t, err)

		assert.Same(t, svc1, svc2)
	})
	t.Run("can alias a service by id", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(di.Svc(NewNoDepSvc).ID("foo").Public()).
			Aliases(di.NewAlias("bar", "foo")).
			Build()
		require.NoError(t, err)

		svc1, err := di.Get[*NoDepSvc](c, di.WithID("foo"))
		require.NoError(t, err)

		svc2, err := di.Get[*NoDepSvc](c, di.WithID("bar"))
		require.NoError(t, err)

		assert.Same(t, svc1, svc2)
	})
	t.Run("can register a service with interface dependency with autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewIFaceDepSvc).
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with interface dependency with autowiring and manual alias", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(
				di.Svc(NewNoDepSvc),
				di.Svc(NewIFaceDepSvc).
					Public(),
			).
			Aliases(di.NewAliasTT[Fooer, *NoDepSvc]()).
			Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with interface dependency without autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewIFaceDepSvc).
				Args(di.Ref[*NoDepSvc]()).
				NotAutowired().
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with interface method argument with autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewIFaceMethodDepSvc).
				MethodCall("SetDep").
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceMethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceMethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with interface method argument with autowiring and manual alias", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(
				di.Svc(NewNoDepSvc),
				di.Svc(NewIFaceMethodDepSvc).
					MethodCall("SetDep").
					Public(),
			).
			Aliases(di.NewAliasTT[Fooer, *NoDepSvc]()).
			Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceMethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceMethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a service with interface method argument without autowiring", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewIFaceMethodDepSvc).
				MethodCall("SetDep", di.Ref[*NoDepSvc]()).
				NotAutowired().
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*IFaceMethodDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &IFaceMethodDepSvc{dep: NewNoDepSvc()}, svc)
	})
	t.Run("can register a circular dependency with method calls", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewCircularMethodASvc).MethodCall("SetDep").Public(),
			di.Svc(NewCircularMethodBSvc).MethodCall("SetDep").Public(),
			di.Svc(NewCircularMethodCSvc).MethodCall("SetDep").Public(),
		).Build()
		require.NoError(t, err)

		svcA, err := di.Get[*CircularMethodASvc](c)
		require.NoError(t, err)
		svcB, err := di.Get[*CircularMethodBSvc](c)
		require.NoError(t, err)
		svcC, err := di.Get[*CircularMethodCSvc](c)
		require.NoError(t, err)

		assert.Equal(t, &CircularMethodASvc{dep: svcB}, svcA)
		assert.Equal(t, &CircularMethodBSvc{dep: svcC}, svcB)
		assert.Equal(t, &CircularMethodCSvc{dep: svcA}, svcC)
	})
	t.Run("can register an eager service", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Eager().Public(),
		).Build()
		require.NoError(t, err)

		assert.True(t, di.Initialised[*NoDepSvc](c))
	})
	t.Run("can register a service with tagged dependency", func(t *testing.T) {
		t.Parallel()

		var tag = di.NewTag("my-tag")
		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("foo").Tags(tag),
			di.Svc(NewNoDepSvc).ID("bar").Tags(tag),
			di.Svc(NewSliceDepSvc).
				Args(di.Tagged[[]*NoDepSvc](tag.ID())).
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*SliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &SliceDepSvc{deps: []*NoDepSvc{NewNoDepSvc(), NewNoDepSvc()}}, svc)
	})
	t.Run("can register a variadic service with tagged dependency", func(t *testing.T) {
		t.Parallel()

		var tag = di.NewTag("my-tag")
		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("foo").Tags(tag),
			di.Svc(NewNoDepSvc).ID("bar").Tags(tag),
			di.Svc(NewSliceDepSvcVariadic).
				Args(di.Tagged[[]*NoDepSvc](tag.ID())).
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*SliceDepSvc](c)
		require.NoError(t, err)
		assert.Equal(t, &SliceDepSvc{deps: []*NoDepSvc{NewNoDepSvc(), NewNoDepSvc()}}, svc)
	})
	t.Run("can register a service with interface type and add a method call", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.SvcT[Fooer](NewNoDepSvc).
				MethodCall("Foo").
				Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[Fooer](c)
		require.NoError(t, err)
		assert.Equal(t, &NoDepSvc{foo: "foo"}, svc)
	})
	t.Run("registering an alias of non-existent service fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Aliases(di.NewAlias("foo", "bar")).
			Build()

		assertErrorInMultiError(t, err, `alias foo points to a non-existing service bar`)
	})
	t.Run("registering service twice with the same ID overrides it", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewStrDepSvc).Args(di.Val("foo")).Public(),
			di.Svc(NewStrDepSvc).Args(di.Val("bar")).Public(),
		).Build()
		require.NoError(t, err)

		svc, err := di.Get[*StrDepSvc](c)
		require.NoError(t, err)

		assert.Equal(t, "bar", svc.prop)
	})
	t.Run("registering a service with invalid references fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewDepSvc).Args(di.Val("foo")).Public(),
		).Build()
		assertErrorInMultiError(t, err, `service *github.com/michalkurzeja/godi_test.NoDepSvc is not registered but is referenced by factory of: *github.com/michalkurzeja/godi_test.DepSvc`)
	})
	t.Run("registering a manual service without arguments fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(di.Val("foo")). // Missing di.Ref[*NoDepSvc]()
				NotAutowired(),
		).Build()
		assertErrorInMultiError(t, err, `argument 0 of *github.com/michalkurzeja/godi_test.DepSvc factory is not set`)
	})
	t.Run("registering a service with mistyped argument fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(di.Val(42)),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.DepSvc: argument int cannot be assigned to any of the function arguments`)
	})
	t.Run("registering a service with mistyped index argument fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(di.Val(42).Idx(1)),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.DepSvc: argument 1 must be assignable to string, got int`)
	})
	t.Run("registering a service with index-out-of-range argument fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(di.Val("foo").Idx(42)),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.DepSvc: argument index out of range: 42`)
	})
	t.Run("registering a service with too many arguments fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewDepSvc).
				Args(
					di.Ref[*NoDepSvc](),
					di.Val("foo"),
					di.Val("bar"), // Too many.
					di.Val(42),    // Too many.
				),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.DepSvc: argument string cannot be assigned to any of the function arguments`)
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.DepSvc: argument int cannot be assigned to any of the function arguments`)
	})
	t.Run("registering a factory with no return values fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(func() {}),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory must return at least one value`)
	})
	t.Run("registering a factory with no return values fails (with type param)", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.SvcT[*NoDepSvc](func() {}),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory must return at least one value`)
	})
	t.Run("registering a factory with too many return values fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(func() (int, int, int) {
				return 1, 2, 3
			}),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory must return at most two values`)
	})
	t.Run("registering a factory with error returned first fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			//nolint:stylecheck
			di.Svc(func() (error, *NoDepSvc) {
				return nil, &NoDepSvc{}
			}),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory may only return an error as a second return value, not *github.com/michalkurzeja/godi_test.NoDepSvc`)
	})
	t.Run("registering a factory that isn't a func fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc("foobar"),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory must be a function, got string`)
	})
	t.Run("registering a factory that isn't a func fails (with type param)", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.SvcT[*NoDepSvc]("foobar"),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition: factory must be a function, got string`)
	})
	t.Run("registering a service with factory returning wrong type fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.SvcT[*DepSvc](NewNoDepSvc),
		).Build()

		assertErrorInMultiError(t, err, `invalid definition: factory of *github.com/michalkurzeja/godi_test.DepSvc must return a value assignable to *github.com/michalkurzeja/godi_test.DepSvc as a first return value`)
	})
	t.Run("registering a service with a non-existing method fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewMethodDepSvc).
				MethodCall("NonExistingMethod"),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.MethodDepSvc: no such method: NonExistingMethod`)
	})
	t.Run("registering a service with a method with wrong return value type fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDepInvalid"),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.MethodDepSvc: method SetDepInvalid may only return an error, not int`)
	})
	t.Run("registering a service with a method with too many return values fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDepInvalid2"),
		).Build()
		assertErrorInMultiError(t, err, `invalid definition of *github.com/michalkurzeja/godi_test.MethodDepSvc: method SetDepInvalid2 must return at most one value`)
	})
	t.Run("registering a circular dependency with factories fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewCircularDepASvc),
			di.Svc(NewCircularDepBSvc),
			di.Svc(NewCircularDepCSvc),
		).Build()

		assertErrorInMultiError(t, err, `service *github.com/michalkurzeja/godi_test.CircularDepCSvc has a circular dependency on *github.com/michalkurzeja/godi_test.CircularDepASvc`)
	})
	t.Run("registering an eager service with erroring factory fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(func() (*NoDepSvc, error) {
				return nil, assert.AnError
			}).Eager(),
		).Build()

		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("registering an interface dependency with no implementations fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewIFaceDepSvc),
		).Build()

		assertErrorInMultiError(t, err, `service github.com/michalkurzeja/godi_test.Fooer is not registered but is referenced by factory of: *github.com/michalkurzeja/godi_test.IFaceDepSvc`)
	})
	t.Run("registering an interface dependency with multiple implementations fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("dep1"),
			di.Svc(NewNoDepSvc).ID("dep2"),
			di.Svc(NewIFaceDepSvc),
		).Build()

		assertErrorInMultiError(t, err, `multiple implementations of github.com/michalkurzeja/godi_test.Fooer found: [dep1 dep2]`)
	})
	t.Run("registering an interface dependency (method) with no implementations fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewIFaceMethodDepSvc).MethodCall("SetDep"),
		).Build()

		assertErrorInMultiError(t, err, `service github.com/michalkurzeja/godi_test.Fooer is not registered but is referenced by: *github.com/michalkurzeja/godi_test.IFaceMethodDepSvc.SetDep`)
	})
	t.Run("registering an interface dependency (method) with multiple implementations fails", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("dep1"),
			di.Svc(NewNoDepSvc).ID("dep2"),
			di.Svc(NewIFaceMethodDepSvc).MethodCall("SetDep"),
		).Build()

		assertErrorInMultiError(t, err, `multiple implementations of github.com/michalkurzeja/godi_test.Fooer found: [dep1 dep2]`)
	})
	t.Run("getting a service twice returns the same instance", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Public(),
		).Build()
		require.NoError(t, err)

		foo1, err := di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		foo2, err := di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		assert.Same(t, foo1, foo2)
	})
	t.Run("getting by a non-existent tag returns an empty array", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Build()
		require.NoError(t, err)

		svcs, err := di.GetByTag[*NoDepSvc](c, "foo")
		require.NoError(t, err)
		assert.Empty(t, svcs)
	})
	t.Run("getting by tag works and returns only returns public services", func(t *testing.T) {
		t.Parallel()

		var tag = di.NewTag("my-tag")
		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("foo").Tags(tag).Public(),
			di.Svc(NewNoDepSvc).ID("bar").Tags(tag).Private(),
			di.Svc(NewNoDepSvc).ID("baz").Tags(tag).Public(),
		).Build()
		require.NoError(t, err)

		svcs, err := di.GetByTag[*NoDepSvc](c, tag.ID())
		require.NoError(t, err)
		assert.Equal(t, []*NoDepSvc{NewNoDepSvc(), NewNoDepSvc()}, svcs)
	})
	t.Run("getting a non-existent service fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Build()
		require.NoError(t, err)

		_, err = di.Get[*NoDepSvc](c)
		assert.EqualError(t, err, `service *github.com/michalkurzeja/godi_test.NoDepSvc not found`)
	})
	t.Run("getting a private service fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*NoDepSvc](c)
		assert.EqualError(t, err, `service *github.com/michalkurzeja/godi_test.NoDepSvc is private`)
	})
	t.Run("getting a private service by alias fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(di.Svc(NewNoDepSvc)).
			Aliases(di.NewAlias("foo", di.FQN[*NoDepSvc]())).
			Build()
		require.NoError(t, err)

		_, err = di.Get[*NoDepSvc](c, di.WithID("foo"))
		assert.EqualError(t, err, `service foo is private`)
	})
	t.Run("getting a service with erroring factory fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(func() (*NoDepSvc, error) {
				return nil, assert.AnError
			}).Public(),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*NoDepSvc](c)
		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("getting a service with erroring method fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc),
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDepErr").
				Public(),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*MethodDepSvc](c)
		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("getting a service with erroring factory dependency fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(func() (*NoDepSvc, error) {
				return nil, assert.AnError
			}),
			di.Svc(NewDepSvc).Args(di.Val("foo")).Public(),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*DepSvc](c)
		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("getting a service with erroring method dependency fails", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(func() (*NoDepSvc, error) {
				return nil, assert.AnError
			}),
			di.Svc(NewMethodDepSvc).
				MethodCall("SetDep").
				Public(),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*MethodDepSvc](c)
		assert.ErrorIs(t, err, assert.AnError)
	})
	t.Run("getting by tag when different types are tagged fails", func(t *testing.T) {
		t.Parallel()

		var tag = di.NewTag("my-tag")

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Public().
				Tags(tag),
			di.Svc(NewDepSvc).Args(di.Val("foo")).Public().
				Tags(tag),
		).Build()
		require.NoError(t, err)

		_, err = di.GetByTag[*NoDepSvc](c, tag.ID())
		assertErrorInMultiError(t, err, `di: service tagged with my-tag is of wrong type; expected *github.com/michalkurzeja/godi_test.NoDepSvc; got *github.com/michalkurzeja/godi_test.DepSvc`)
	})
	t.Run("panics on MustGet error", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Build()
		require.NoError(t, err)

		assert.Panics(t, func() {
			di.MustGet[*NoDepSvc](c)
		})
	})
	t.Run("panics on MustGetByTag error", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Tags(di.NewTag("foo")).Public(),
		).Build()
		require.NoError(t, err)

		assert.Panics(t, func() {
			di.MustGetByTag[*DepSvc](c, "foo") // Wrong type.
		})
	})
	t.Run("has works correctly for definitions", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("pub").Public(),
			di.Svc(NewNoDepSvc).ID("priv").Private(),
		).Build()
		require.NoError(t, err)

		assert.True(t, di.Has[*NoDepSvc](c, di.WithID("pub")))
		assert.True(t, di.Has[*NoDepSvc](c, di.WithID("priv")))
		assert.False(t, di.Has[*NoDepSvc](c, di.WithID("nope")))
		assert.False(t, di.Has[*NoDepSvc](c))
		assert.False(t, di.Has[*DepSvc](c))
	})
	t.Run("has works correctly for instances", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).ID("pub").Public(),
			di.Svc(NewNoDepSvc).ID("priv").Private(),
		).Build()
		require.NoError(t, err)

		_, err = di.Get[*NoDepSvc](c, di.WithID("pub"))
		require.NoError(t, err)

		assert.True(t, di.Has[*NoDepSvc](c, di.WithID("pub")))
		assert.True(t, di.Has[*NoDepSvc](c, di.WithID("priv")))
		assert.False(t, di.Has[*NoDepSvc](c, di.WithID("nope")))
		assert.False(t, di.Has[*NoDepSvc](c))
		assert.False(t, di.Has[*DepSvc](c))
	})
	t.Run("initialised works correctly", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Public(),
		).Build()
		require.NoError(t, err)

		assert.False(t, di.Initialised[*NoDepSvc](c))

		_, err = di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		assert.True(t, di.Initialised[*NoDepSvc](c))
	})
	t.Run("uncached service is always constructed anew", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Public().NotShared(),
		).Build()
		require.NoError(t, err)

		svc1, err := di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		svc2, err := di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		assert.NotSame(t, svc1, svc2)
	})
	t.Run("uncached service is never considered initialised", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().Services(
			di.Svc(NewNoDepSvc).Public().NotShared(),
		).Build()
		require.NoError(t, err)

		assert.False(t, di.Initialised[*NoDepSvc](c))

		_, err = di.Get[*NoDepSvc](c)
		require.NoError(t, err)

		assert.False(t, di.Initialised[*NoDepSvc](c))
	})
	t.Run("builder can only be used once", func(t *testing.T) {
		t.Parallel()

		b := di.New()
		_, err := b.Build()
		require.NoError(t, err)

		_, err = b.Build()
		assertErrorInMultiError(t, err, `container already built`)

		assert.Panics(t, func() {
			b.Aliases(di.NewAlias("foo", "bar"))
		})
	})
}

type NoDepSvc struct {
	foo string
}

func (d NoDepSvc) Foo() {}

func NewNoDepSvc() *NoDepSvc {
	return &NoDepSvc{foo: "foo"}
}

type DepSvc struct {
	dep  *NoDepSvc
	prop string
}

func NewDepSvc(foo *NoDepSvc, prop string) *DepSvc {
	return &DepSvc{dep: foo, prop: prop}
}

type StrDepSvc struct {
	prop string
}

func NewStrDepSvc(prop string) *StrDepSvc {
	return &StrDepSvc{prop: prop}
}

type SliceDepSvc struct {
	deps []*NoDepSvc
}

func NewSliceDepSvc(deps []*NoDepSvc) *SliceDepSvc {
	return &SliceDepSvc{deps: deps}
}

func NewSliceDepSvcVariadic(deps ...*NoDepSvc) *SliceDepSvc {
	return &SliceDepSvc{deps: deps}
}

type StrSliceDepSvc struct {
	props []string
}

func NewStrSliceDepSvc(props []string) *StrSliceDepSvc {
	return &StrSliceDepSvc{props: props}
}

func NewStrSliceDepSvcVariadic(props ...string) *StrSliceDepSvc {
	return &StrSliceDepSvc{props: props}
}

type MethodDepSvc struct {
	dep *NoDepSvc
}

func NewMethodDepSvc() *MethodDepSvc {
	return &MethodDepSvc{}
}

func (s *MethodDepSvc) SetDep(dep *NoDepSvc) {
	s.dep = dep
}

func (s *MethodDepSvc) SetDepE(dep *NoDepSvc) error {
	s.dep = dep
	return nil
}

func (s *MethodDepSvc) SetDepErr(dep *NoDepSvc) error {
	s.dep = dep
	return assert.AnError
}

func (s *MethodDepSvc) SetDepInvalid(dep *NoDepSvc) int {
	s.dep = dep
	return 0
}

func (s *MethodDepSvc) SetDepInvalid2(dep *NoDepSvc) (int, error) {
	s.dep = dep
	return 0, nil
}

type Fooer interface {
	Foo()
}

type IFaceDepSvc struct {
	dep Fooer
}

func NewIFaceDepSvc(dep Fooer) *IFaceDepSvc {
	return &IFaceDepSvc{dep: dep}
}

type IFaceMethodDepSvc struct {
	dep Fooer
}

func NewIFaceMethodDepSvc() *IFaceMethodDepSvc {
	return &IFaceMethodDepSvc{}
}

func (s *IFaceMethodDepSvc) SetDep(dep Fooer) {
	s.dep = dep
}

func assertErrorInMultiError(t *testing.T, err error, errString string, msgAndArgs ...any) bool {
	t.Helper()
	multi, ok := lo.ErrorsAs[*multierror.Error](err)
	if !ok {
		t.Fatalf("expected multierror, got %T", err)
	}
	for _, merr := range multi.Errors {
		if merr.Error() == errString {
			return true
		}
	}
	return assert.Fail(t, fmt.Sprintf("Error message not found in multierror:\n"+
		"expected: %q\n"+
		"got: %q", errString, multi), msgAndArgs...)
}

type CircularDepASvc struct {
	dep *CircularDepBSvc
}

func NewCircularDepASvc(dep *CircularDepBSvc) *CircularDepASvc {
	return &CircularDepASvc{dep: dep}
}

type CircularDepBSvc struct {
	dep *CircularDepCSvc
}

func NewCircularDepBSvc(dep *CircularDepCSvc) *CircularDepBSvc {
	return &CircularDepBSvc{dep: dep}
}

type CircularDepCSvc struct {
	dep *CircularDepASvc
}

func NewCircularDepCSvc(dep *CircularDepASvc) *CircularDepCSvc {
	return &CircularDepCSvc{dep: dep}
}

type CircularMethodASvc struct {
	dep *CircularMethodBSvc
}

func NewCircularMethodASvc() *CircularMethodASvc {
	return &CircularMethodASvc{}
}

func (s *CircularMethodASvc) SetDep(dep *CircularMethodBSvc) {
	s.dep = dep
}

type CircularMethodBSvc struct {
	dep *CircularMethodCSvc
}

func NewCircularMethodBSvc() *CircularMethodBSvc {
	return &CircularMethodBSvc{}
}

func (s *CircularMethodBSvc) SetDep(dep *CircularMethodCSvc) {
	s.dep = dep
}

type CircularMethodCSvc struct {
	dep *CircularMethodASvc
}

func NewCircularMethodCSvc() *CircularMethodCSvc {
	return &CircularMethodCSvc{}
}

func (s *CircularMethodCSvc) SetDep(dep *CircularMethodASvc) {
	s.dep = dep
}
