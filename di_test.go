package di_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	di "github.com/michalkurzeja/godi"
)

func TestDI(t *testing.T) {
	t.Run("works when", func(t *testing.T) {
		t.Parallel()
		t.Run("given simple services", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo"),
				),
				di.SvcT[Bar](NewBar),
			)
			assert.NoError(t, err)

			foo, err := di.Get[Foo](c)
			assert.NoError(t, err)
			bar, err := di.Get[Bar](c)
			assert.NoError(t, err)

			assert.Equal(t, "foo", foo.field)
			assert.Equal(t, foo, bar.foo)
		})
		t.Run("registering an iterface type", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[AnInterface](NewAnInterfaceImpl1),
			)
			assert.NoError(t, err)

			v, err := di.Get[AnInterface](c)
			assert.NoError(t, err)
			assert.IsType(t, &AnInterfaceImpl1{}, v)
		})
		t.Run("an argument is overridden in build call", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo"),
				),
				di.SvcT[Bar](NewBar).With(
					di.Val(Foo{field: "overridden"}),
				),
			)
			assert.NoError(t, err)

			foo, err := di.Get[Foo](c)
			assert.NoError(t, err)
			bar, err := di.Get[Bar](c)
			assert.NoError(t, err)

			assert.Equal(t, "foo", foo.field)
			assert.Equal(t, Foo{field: "overridden"}, bar.foo)
		})
		t.Run("the same service is registered twice under custom IDs", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).ID("foo1").With(
					di.Val("foo1"),
				),
				di.SvcT[Foo](NewFoo).ID("foo2").With(
					di.Val("foo2"),
				),
				di.SvcT[Bar](NewBar).With(
					di.Ref[Foo]("foo1"),
				),
			)
			assert.NoError(t, err)

			foo1, err := di.Get[Foo](c, di.WithID("foo1"))
			assert.NoError(t, err)
			foo2, err := di.Get[Foo](c, di.WithID("foo2"))
			assert.NoError(t, err)
			bar, err := di.Get[Bar](c)
			assert.NoError(t, err)

			assert.Equal(t, "foo1", foo1.field)
			assert.Equal(t, "foo2", foo2.field)
			assert.Equal(t, foo1, bar.foo)
		})
		t.Run("given an interface as dependency, with one registered implementation", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*AnInterfaceImpl1](NewAnInterfaceImpl1),
				di.SvcT[DependsOnInterface](NewDependsOnInterface),
			)
			assert.NoError(t, err)

			dep, err := di.Get[DependsOnInterface](c)
			assert.NoError(t, err)
			assert.IsType(t, (*AnInterfaceImpl1)(nil), dep.Dep)
		})
		t.Run("given an interface as dependency, with multiple registered implementations, with explicit argument", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*AnInterfaceImpl1](NewAnInterfaceImpl1),
				di.SvcT[*AnInterfaceImpl2](NewAnInterfaceImpl2),
				di.SvcT[DependsOnInterface](NewDependsOnInterface).With(
					di.Ref[*AnInterfaceImpl1](),
				),
			)
			assert.NoError(t, err)

			dep, err := di.Get[DependsOnInterface](c)
			assert.NoError(t, err)
			assert.IsType(t, (*AnInterfaceImpl1)(nil), dep.Dep)
		})
		t.Run("given a valid provider to di.Svc", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.Svc(NewFoo).With(
					di.Val("foo"),
				),
				di.Svc(NewBar),
			)
			assert.NoError(t, err)

			foo, err := di.Get[Foo](c)
			assert.NoError(t, err)
			bar, err := di.Get[Bar](c)
			assert.NoError(t, err)

			assert.Equal(t, "foo", foo.field)
			assert.Equal(t, foo, bar.foo)
		})
		t.Run("the same service is registered twice under custom IDs using di.Svc", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.Svc(NewFoo).With(
					di.Val("foo"),
				),
				di.Svc(NewBar).ID("bar1"),
				di.Svc(NewBar).ID("bar2"),
			)
			assert.NoError(t, err)

			foo, err := di.Get[Foo](c)
			assert.NoError(t, err)
			bar1, err := di.Get[Bar](c, di.WithID("bar1"))
			assert.NoError(t, err)
			bar2, err := di.Get[Bar](c, di.WithID("bar2"))
			assert.NoError(t, err)

			assert.Equal(t, "foo", foo.field)
			assert.Equal(t, foo, bar1.foo)
			assert.Equal(t, foo, bar2.foo)
		})
		t.Run("given a cyclic dependency with deferred injection", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC).With(
					di.Ref[*CyclicD]("target-without-err"),
				),
				di.SvcT[*CyclicD](NewCyclicD).ID("target-without-err").With(
					di.Ref[*CyclicC]().DeferTo("SetCyclicC"),
				),
				di.SvcT[*CyclicD](NewCyclicD).ID("target-with-err").With(
					di.Ref[*CyclicC]().DeferTo("SetCyclicCWithErr"),
				),
			)
			assert.NoError(t, err)

			cycC, err := di.Get[*CyclicC](c)
			assert.NoError(t, err)
			cycD1, err := di.Get[*CyclicD](c, di.WithID("target-without-err"))
			assert.NoError(t, err)
			cycD2, err := di.Get[*CyclicD](c, di.WithID("target-with-err"))
			assert.NoError(t, err)

			assert.Equal(t, cycC, cycD1.C)
			assert.Equal(t, cycC, cycD2.C)
			assert.Equal(t, cycD1, cycC.D)
		})
		t.Run("when deferring injection to an interface type", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo"),
				),
				di.SvcT[InterfaceWithSetter](NewInterfaceWithSetterImpl).With(
					di.Ref[Foo]().DeferTo("SetFoo"),
				),
			)
			assert.NoError(t, err)

			foo, err := di.Get[Foo](c)
			assert.NoError(t, err)
			v, err := di.Get[InterfaceWithSetter](c)
			assert.NoError(t, err)
			assert.IsType(t, (*InterfaceWithSetterImpl)(nil), v)
			assert.Equal(t, foo, v.(*InterfaceWithSetterImpl).foo)
		})
		t.Run("when overriding a dependency at a specified position", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo1"),
				),
				di.SvcT[Foo](NewFoo).ID("foo2").With(
					di.Val("foo2"),
				),
				di.SvcT[MultiDep](NewMultiDep).With(
					di.Ref[Foo]("foo2").Pos(1),
				),
			)
			assert.NoError(t, err)

			got, err := di.Get[MultiDep](c)
			assert.NoError(t, err)
			assert.Equal(t, "foo1", got.foo1.field)
			assert.Equal(t, "foo2", got.foo2.field)
			assert.Equal(t, "foo1", got.foo3.field)
		})
	})
	t.Run("Register returns an error when", func(t *testing.T) {
		t.Parallel()
		t.Run("container is already compiled", func(t *testing.T) {
			t.Parallel()

			c := di.New()
			err := c.Compile()
			assert.NoError(t, err)

			err = di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo"),
				),
				di.SvcT[Bar](NewBar),
			)
			assert.ErrorIs(t, err, di.ErrContainerCompiled)
		})
		t.Run("no services are provided", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c)
			assert.EqualError(t, err, "di: no services to register")
		})
		t.Run("given a non-function", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](42))
			assert.EqualError(t, err, "di: provider must be a function, got int")
		})
		t.Run("function returns nothing", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() {}))
			assert.EqualError(t, err, "di: provider must return at least one value")
		})
		t.Run("function returns the provided value twice", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() (Foo, Foo) { return Foo{}, Foo{} }))
			assert.EqualError(t, err, "di: provider must not return more than one value of type github.com/michalkurzeja/godi_test.Foo")
		})
		t.Run("function returns an error twice", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() (Foo, error, error) { return Foo{}, nil, nil }))
			assert.EqualError(t, err, "di: provider must not return more than one value of type error")
		})
		t.Run("wrong constructor is passed", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](NewBar))
			assert.EqualError(t, err, "di: provider must return github.com/michalkurzeja/godi_test.Foo")
		})
		t.Run("registering a service twice", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo),
				di.SvcT[Foo](NewFoo),
			)
			assert.EqualError(t, err, "di: node github.com/michalkurzeja/godi_test.Foo already exists")
		})
		t.Run("registering a cyclic dependency", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicA](NewCyclicA),
				di.SvcT[*CyclicB](NewCyclicB),
			)
			assert.EqualError(t, err, `di: detected cyclic dependency between *github.com/michalkurzeja/godi_test.CyclicB and *github.com/michalkurzeja/godi_test.CyclicA`)
		})
		t.Run("given too many manual dependencies", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val("foo"),
					di.Val("foo"),
				),
			)
			assert.EqualError(t, err, "di: invalid dependencies of github.com/michalkurzeja/godi_test.Foo: provider expects 1 argument(s), got 2")
		})
		t.Run("given dependencies that do not match the provider", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val(42),
				),
			)
			assert.EqualError(t, err, "di: invalid dependencies of github.com/michalkurzeja/godi_test.Foo: argument(s) do not match provider inputs: [42]")
		})
		t.Run("given positional dependencies of invalid type", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val(42).Pos(0),
				),
			)
			assert.EqualError(t, err, "di: invalid dependencies of github.com/michalkurzeja/godi_test.Foo: dependency 42 cannot be used as argument 0 of the provider")
		})
		t.Run("given pout-of-bounds positional dependencies", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).With(
					di.Val(42).Pos(99),
				),
			)
			assert.EqualError(t, err, "di: invalid dependencies of github.com/michalkurzeja/godi_test.Foo: dependency 42 has position 99 but the provider only takes 1 argument(s)")
		})
	})
	t.Run("Compile returns an error when", func(t *testing.T) {
		t.Parallel()
		t.Run("container is already compiled", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := c.Compile()
			assert.NoError(t, err)

			err = c.Compile()
			assert.ErrorIs(t, err, di.ErrContainerCompiled)
		})
		t.Run("no implementations of an interface dependency are registered", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[DependsOnInterface](NewDependsOnInterface))

			var wantErr di.NoImplementationError
			assert.ErrorAs(t, err, &wantErr)
		})
		t.Run("multiple implementations of an interface dependency are registered", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*AnInterfaceImpl1](NewAnInterfaceImpl1),
				di.SvcT[*AnInterfaceImpl2](NewAnInterfaceImpl2),
				di.SvcT[DependsOnInterface](NewDependsOnInterface),
			)

			var wantErr di.AmbiguousImplementationError
			assert.ErrorAs(t, err, &wantErr)
		})
		t.Run("a dependency of a service is not registered", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Bar](NewBar))

			var wantErr di.UnresolvedRefError
			errors.As(err, &wantErr)
			assert.Equal(t, di.UnresolvedRefError{
				ID:       di.FQN[Foo](),
				RefNodes: []string{di.FQN[Bar]()},
			}, wantErr)
		})
	})
	t.Run("Get returns an error when", func(t *testing.T) {
		t.Parallel()
		t.Run("the constructor returns an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() (Foo, error) { return Foo{}, assert.AnError }))
			assert.NoError(t, err)

			_, err = di.Get[Foo](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("the constructor panics with an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() Foo { panic(assert.AnError) }))
			assert.NoError(t, err)

			_, err = di.Get[Foo](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("the constructor panics with an non-error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c, di.SvcT[Foo](func() Foo { panic("oops!") }))
			assert.NoError(t, err)

			_, err = di.Get[Foo](c)
			assert.EqualError(t, err, `di: building service github.com/michalkurzeja/godi_test.Foo failed: oops!`)
		})
		t.Run("a dependency returns an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() (Foo, error) { return Foo{}, assert.AnError }),
				di.SvcT[Bar](NewBar),
			)
			assert.NoError(t, err)

			_, err = di.Get[Bar](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("a dependency panics with an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() Foo { panic(assert.AnError) }),
				di.SvcT[Bar](NewBar),
			)
			assert.NoError(t, err)

			_, err = di.Get[Bar](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("a dependency panics with an non-error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() Foo { panic("oops!") }),
				di.SvcT[Bar](NewBar),
			)
			assert.NoError(t, err)

			_, err = di.Get[Bar](c)
			assert.EqualError(t, err, `di: building service github.com/michalkurzeja/godi_test.Foo failed: oops!`)
		})
		t.Run("a deferred dependency returns an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() (Foo, error) { return Foo{}, assert.AnError }),
				di.SvcT[*InterfaceWithSetterImpl](NewInterfaceWithSetterImpl).With(
					di.Ref[Foo]().DeferTo("SetFoo"),
				),
			)
			assert.NoError(t, err)

			_, err = di.Get[*InterfaceWithSetterImpl](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("a deferred dependency panics with an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() Foo { panic(assert.AnError) }),
				di.SvcT[*InterfaceWithSetterImpl](NewInterfaceWithSetterImpl).With(
					di.Ref[Foo]().DeferTo("SetFoo"),
				),
			)
			assert.NoError(t, err)

			_, err = di.Get[*InterfaceWithSetterImpl](c)
			assert.ErrorIs(t, err, assert.AnError)
		})
		t.Run("a deferred dependency panics with an non-error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](func() Foo { panic("oops!") }),
				di.SvcT[*InterfaceWithSetterImpl](NewInterfaceWithSetterImpl).With(
					di.Ref[Foo]().DeferTo("SetFoo"),
				),
			)
			assert.NoError(t, err)

			_, err = di.Get[*InterfaceWithSetterImpl](c)
			assert.EqualError(t, err, `di: building service github.com/michalkurzeja/godi_test.Foo failed: oops!`)
		})
		t.Run("requesting a service that was not registered", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := c.Compile()
			assert.NoError(t, err)

			_, err = di.Get[Bar](c)
			assert.EqualError(t, err, `di: node github.com/michalkurzeja/godi_test.Bar not found`)
		})
		t.Run("wrong type is provided", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[Foo](NewFoo).ID("my-svc").With(
					di.Val("foo"),
				),
			)
			assert.NoError(t, err)

			_, err = di.Get[Bar](c, di.WithID("my-svc"))
			assert.EqualError(t, err, `di: service my-svc is of wrong type; expected github.com/michalkurzeja/godi_test.Bar; got github.com/michalkurzeja/godi_test.Foo`)
		})
		t.Run("a deferred dependency target method returns an error", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("SetCyclicCReturnsErr"),
				),
			)
			assert.NoError(t, err)

			_, err = di.Get[*CyclicD](c)
			assert.EqualError(t, err, `di: error while injecting *github.com/michalkurzeja/godi_test.CyclicC via "SetCyclicCReturnsErr": foobar`)
		})
		t.Run("a deferred dependency calls non-existent method", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("non-existent"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: method "non-existent" not found in type *github.com/michalkurzeja/godi_test.CyclicD`)
		})
		t.Run("a deferred dependency calls a method with no arguments", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("NotEnoughInputs"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: method "NotEnoughInputs" must have 1 input; got 0`)
		})
		t.Run("a deferred dependency calls a method with too many arguments", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("TooManyInputs"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: method "TooManyInputs" must have 1 input; got 2`)
		})
		t.Run("a deferred dependency calls a method with non-assignable argument", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Val(42).DeferTo("SetCyclicC"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: expected method "SetCyclicC" argument to be of type *github.com/michalkurzeja/godi_test.CyclicC; got int`)
		})
		t.Run("a deferred dependency calls a method with more than 1 output", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("TooManyOutputs"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: method "TooManyOutputs" must not have more than 1 output; got 2`)
		})
		t.Run("a deferred dependency calls a method with non-error output", func(t *testing.T) {
			t.Parallel()

			c := di.New()

			err := di.Register(c,
				di.SvcT[*CyclicC](NewCyclicC),
				di.SvcT[*CyclicD](NewCyclicD).With(
					di.Ref[*CyclicC]().DeferTo("NonErrOutput"),
				),
			)
			assert.EqualError(t, err, `di: invalid deferred dependency of *github.com/michalkurzeja/godi_test.CyclicD: method "NonErrOutput" must have no outputs or return an error; got bool`)
		})
	})
}

type Foo struct {
	field string
}

func NewFoo(field string) (Foo, error) {
	return Foo{field: field}, nil
}

type Bar struct {
	foo Foo
}

func NewBar(foo Foo) Bar {
	return Bar{foo: foo}
}

type AnInterface interface {
	SomeFunc()
}

type AnInterfaceImpl1 struct{}

func NewAnInterfaceImpl1() *AnInterfaceImpl1 {
	return &AnInterfaceImpl1{}
}

func (AnInterfaceImpl1) SomeFunc() {}

type AnInterfaceImpl2 struct{}

func NewAnInterfaceImpl2() *AnInterfaceImpl2 {
	return &AnInterfaceImpl2{}
}

func (AnInterfaceImpl2) SomeFunc() {}

type DependsOnInterface struct {
	Dep AnInterface
}

func NewDependsOnInterface(dep AnInterface) DependsOnInterface {
	return DependsOnInterface{Dep: dep}
}

type CyclicA struct {
	B *CyclicB
}

func NewCyclicA(b *CyclicB) *CyclicA {
	return &CyclicA{B: b}
}

type CyclicB struct {
	A *CyclicA
}

func NewCyclicB(a *CyclicA) *CyclicB {
	return &CyclicB{A: a}
}

type CyclicC struct {
	D *CyclicD
}

func NewCyclicC(d *CyclicD) *CyclicC {
	return &CyclicC{D: d}
}

type CyclicD struct{ C *CyclicC }

func NewCyclicD() *CyclicD {
	return &CyclicD{}
}

func (c *CyclicD) SetCyclicC(cyclicC *CyclicC) {
	c.C = cyclicC
}

func (c *CyclicD) SetCyclicCWithErr(cyclicC *CyclicC) error {
	c.C = cyclicC
	return nil
}

func (c *CyclicD) SetCyclicCReturnsErr(cyclicC *CyclicC) error {
	c.C = cyclicC
	return errors.New("foobar")
}

func (c *CyclicD) NotEnoughInputs()                        {}
func (c *CyclicD) TooManyInputs(_, _ *CyclicC)             {}
func (c *CyclicD) NonErrOutput(_ *CyclicC) bool            { return false }
func (c *CyclicD) TooManyOutputs(_ *CyclicC) (bool, error) { return false, nil }

type MultiDep struct {
	foo1, foo2, foo3 Foo
}

func NewMultiDep(foo1, foo2, foo3 Foo) MultiDep {
	return MultiDep{foo1: foo1, foo2: foo2, foo3: foo3}
}

type InterfaceWithSetter interface {
	SetFoo(foo Foo)
}

type InterfaceWithSetterImpl struct {
	foo Foo
}

func NewInterfaceWithSetterImpl() *InterfaceWithSetterImpl { return &InterfaceWithSetterImpl{} }

func (i *InterfaceWithSetterImpl) SetFoo(foo Foo) { i.foo = foo }
