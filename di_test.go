package godi_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
)

const constMethodArg = "const-method-arg"

type TestSvc struct {
	Args []any
}

func NewTestSvcNoArgs() *TestSvc {
	return &TestSvc{}
}

func NewTestSvcStrArg(s string) *TestSvc {
	return &TestSvc{Args: []any{s}}
}

func NewTestSvcIfaceArg(i TestIface) *TestSvc {
	return &TestSvc{Args: []any{i}}
}

func NewTestSvcSliceArgs(args []string) *TestSvc {
	return &TestSvc{Args: lo.ToAnySlice(args)}
}

func NewTestSvcVariadicArgs(args ...string) *TestSvc {
	return &TestSvc{Args: lo.ToAnySlice(args)}
}

func NewTestSvcIfaceSliceArgs(args []TestIface) *TestSvc {
	return &TestSvc{Args: lo.ToAnySlice(args)}
}

func NewTestSvcIfaceVariadicArgs(args ...TestIface) *TestSvc {
	return &TestSvc{Args: lo.ToAnySlice(args)}
}

func (s *TestSvc) AddConstArg() {
	s.Args = append(s.Args, constMethodArg)
}

func (s *TestSvc) AddArgStr(arg string) {
	s.Args = append(s.Args, arg)
}

func (s *TestSvc) AddArgIface(arg TestIface) {
	s.Args = append(s.Args, arg)
}

func (s *TestSvc) AddArgsSlice(args []string) {
	s.Args = append(s.Args, lo.ToAnySlice(args)...)
}

func (s *TestSvc) AddArgsVariadic(args ...string) {
	s.Args = append(s.Args, lo.ToAnySlice(args)...)
}

func (s *TestSvc) AddArgsIfaceSlice(args []TestIface) {
	s.Args = append(s.Args, lo.ToAnySlice(args)...)
}

func (s *TestSvc) AddArgsIfaceVariadic(args ...TestIface) {
	s.Args = append(s.Args, lo.ToAnySlice(args)...)
}

type TestIface interface {
	TestIfaceMethod()
}

type TestIfaceImpl struct {
	MethodCalled bool
}

func (i *TestIfaceImpl) TestIfaceMethod() {
	i.MethodCalled = true
}

func TestGodi_Services(t *testing.T) {
	tests := []struct {
		name           string
		build          func(b *di.Builder, refs *Refs)
		assertBuildErr func(t *testing.T, err error)
		assert         func(t *testing.T, c di.Container, refs *Refs)
	}{
		// Empty container
		{
			name: "can build an empty container",
		},
		// Retrieval by ref
		{
			name: "can retrieve a service by ref",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").Bind(refs.Svc.New("foo")),
					di.Svc(NewTestSvcStrArg, "bar").Bind(refs.Svc.New("bar")),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svcFoo, err := di.SvcByRef[*TestSvc](c, refs.Svc.Get(t, "foo"))
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcFoo.Args)
				svcBar, err := di.SvcByRef[*TestSvc](c, refs.Svc.Get(t, "bar"))
				require.NoError(t, err)
				require.Equal(t, []any{"bar"}, svcBar.Args)
			},
		},
		{
			name: "retrieving a service by empty ref results in an error",
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.SvcByRef[*TestSvc](c, *refs.Svc.New("foo"))
				require.ErrorContains(t, err, "service not found: empty reference")
			},
		},
		{
			name: "retrieving a service that doesn't exist by reference results in an error",
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, _ = di.New().Services(
					di.Svc(NewTestSvcNoArgs).Bind(refs.Svc.New("foo")), // Different container, so even though the ref is not empty, the service is not registered in the current container.
				).Build()

				_, err := di.SvcByRef[*TestSvc](c, refs.Svc.Get(t, "foo"))
				require.ErrorContains(t, err, "service github.com/michalkurzeja/godi/v2_test.(*TestSvc) not found")
			},
		},
		// Retrieval by type
		{
			name: "can retrieve a service by type",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: "can retrieve multiple services by type",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo"),
					di.Svc(NewTestSvcStrArg, "bar"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svcs, err := di.SvcsByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcs[0].Args)
				require.Equal(t, []any{"bar"}, svcs[1].Args)
			},
		},
		{
			name: "cannot retrieve a single service by type if multiple exist",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo"),
					di.Svc(NewTestSvcStrArg, "bar"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.SvcByType[*TestSvc](c)
				require.ErrorContains(t, err, "found multiple services of type github.com/michalkurzeja/godi/v2_test.(*TestSvc)")
			},
		},
		{
			name: "retrieving a type that doesn't exist results in an error",
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.SvcByType[*TestSvc](c)
				require.ErrorContains(t, err, "service of type github.com/michalkurzeja/godi/v2_test.(*TestSvc) not found")
			},
		},
		// Retrieval by label
		{
			name: "can retrieve a service by label",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").Labels("my-label", "my-second-label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc1, err := di.SvcByLabel[*TestSvc](c, "my-label")
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc1.Args)
				svc2, err := di.SvcByLabel[*TestSvc](c, "my-second-label")
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc2.Args)
				require.Same(t, svc1, svc2)
			},
		},
		{
			name: "can retrieve multiple services by label",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").Labels("my-label"),
					di.Svc(NewTestSvcStrArg, "bar").Labels("my-label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svcs, err := di.SvcsByLabel[*TestSvc](c, "my-label")
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcs[0].Args)
				require.Equal(t, []any{"bar"}, svcs[1].Args)
			},
		},
		{
			name: "cannot retrieve a single service by label if multiple exist",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").Labels("my-label"),
					di.Svc(NewTestSvcStrArg, "bar").Labels("my-label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.SvcByLabel[*TestSvc](c, "my-label")
				require.ErrorContains(t, err, "found multiple services with label my-label")
			},
		},
		{
			name: "retrieving a label that doesn't exist results in an error",
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.SvcByLabel[*TestSvc](c, "my-label")
				require.ErrorContains(t, err, "service with label my-label not found")
			},
		},
		// Retrieval by various means
		{
			name: "can retrieve a service by ref, type, and label",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").
						Bind(refs.Svc.New("foo")).
						Labels("my-label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svcRef, err := di.SvcByRef[*TestSvc](c, refs.Svc.Get(t, "foo"))
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcRef.Args)

				svcType, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcType.Args)

				svcLabel, err := di.SvcByLabel[*TestSvc](c, "my-label")
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svcLabel.Args)
			},
		},
		// Literal args (no autowiring) - good cases
		{
			name: "can register a service with no args",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Empty(t, svc.Args)
			},
		},
		{
			name: "can register a service with manual single arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, "foo").NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: "can register a service with manual single interface arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceArg, new(TestIfaceImpl)).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.IsType(t, []any{&TestIfaceImpl{}}, svc.Args)
			},
		},
		{
			name: "can register a service with manual slice arg (variadic-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs, "foo", "bar").NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "can register a service with manual slice arg (slice-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs, []string{"foo", "bar"}).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "can register a service with manual interface slice arg (variadic-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceSliceArgs, new(TestIfaceImpl), new(TestIfaceImpl)).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can register a service with manual interface slice arg (slice-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceSliceArgs, []TestIface{new(TestIfaceImpl), new(TestIfaceImpl)}).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can register a service with manual variadic arg (variadic-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcVariadicArgs, "foo", "bar").NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "can register a service with manual variadic arg (slice-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcVariadicArgs, []string{"foo", "bar"}).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "can register a service with manual interface variadic arg (variadic-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceVariadicArgs, new(TestIfaceImpl), new(TestIfaceImpl)).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can register a service with manual interface variadic arg (slice-style)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceVariadicArgs, []TestIface{new(TestIfaceImpl), new(TestIfaceImpl)}).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can register a service with manual label args",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs, di.SliceOf[string]("label")).NotAutowired(),
					di.SvcVal("foo"),
					di.SvcVal("bar").Labels("label"),
					di.SvcVal("baz").Labels("label", "other"),
					di.SvcVal("qux").Labels("other"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"bar", "baz"}, svc.Args)
			},
		},
		{
			name: "Ref arg selects service by reference",
			build: func(b *di.Builder, refs *Refs) {
				var fooRef di.SvcReference
				b.Services(
					di.SvcVal("foo").Bind(&fooRef),
					di.Svc(NewTestSvcStrArg, di.Ref(&fooRef)).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: "Ref arg selects service by reference (binding after arg)",
			build: func(b *di.Builder, refs *Refs) {
				var fooRef di.SvcReference
				b.Services(
					di.Svc(NewTestSvcStrArg, di.Ref(&fooRef)).NotAutowired(),
					di.SvcVal("foo").Bind(&fooRef),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: `"Type[[]string]" arg selects "[]string" service over "string" services`,
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.SvcVal("loose-1"),
					di.SvcVal("loose-2"),
					di.SvcVal([]string{"slice-elem-1", "slice-elem-2"}),
					di.Svc(NewTestSvcSliceArgs, di.Type[[]string]()).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"slice-elem-1", "slice-elem-2"}, svc.Args)
			},
		},
		{
			name: `"SliceOf[string]" arg selects "string" services over "[]string" service`,
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.SvcVal("loose-1"),
					di.SvcVal("loose-2"),
					di.SvcVal([]string{"slice-elem-1", "slice-elem-2"}),
					di.Svc(NewTestSvcSliceArgs, di.SliceOf[string]()).NotAutowired(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"loose-1", "loose-2"}, svc.Args)
			},
		},
		// Manual args (no autowiring) - error cases
		{
			name: "cannot register a service with no args and manual args",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs, "foo").NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "invalid definition of github.com/michalkurzeja/godi/v2_test.(*TestSvc): failed to add factory args: argument string cannot be slotted to function")
			},
		},
		{
			name: "cannot register a service with missing args",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*TestSvc): invalid factory github.com/michalkurzeja/godi/v2_test.NewTestSvcStrArg: argument 0 is not set")
			},
		},
		{
			name: "cannot register a service with empty interface arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceArg).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*TestSvc): invalid factory github.com/michalkurzeja/godi/v2_test.NewTestSvcIfaceArg: argument 0 is not set")
			},
		},
		{
			name: "cannot register a service with empty slice arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*TestSvc): invalid factory github.com/michalkurzeja/godi/v2_test.NewTestSvcSliceArgs: argument 0 is not set")
			},
		},
		{
			name: "cannot register a manual service with empty variadic arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcVariadicArgs).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*TestSvc): invalid factory github.com/michalkurzeja/godi/v2_test.NewTestSvcVariadicArgs: argument 0 is not set")
			},
		},
		{
			name: "cannot register a service with wrong type of arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg, 42).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "invalid definition of github.com/michalkurzeja/godi/v2_test.(*TestSvc): failed to add factory args: argument int cannot be slotted to function")
			},
		},
		{
			name: "cannot register a service with wrong type of interface arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceArg, 42).NotAutowired(),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "invalid definition of github.com/michalkurzeja/godi/v2_test.(*TestSvc): failed to add factory args: argument int cannot be slotted to function")
			},
		},
		// Child services
		{
			name: "can register a child service, which can only be used as a dependency for the parent",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg).Children(
						di.SvcVal("foo"),
					),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)

				_, err = di.SvcByType[string](c) // This svc only exists in the child scope!
				require.ErrorContains(t, err, "service of type string not found")
			},
		},
		{
			name: "can use both children and siblings as dependencies",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.SvcVal("bar"),
					di.Svc(NewTestSvcVariadicArgs).Children(
						di.SvcVal("foo"),
					),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "sibling service can be accessed by Ref arg",
			build: func(b *di.Builder, refs *Refs) {
				var barRef di.SvcReference
				b.Services(
					di.SvcVal("bar").Bind(&barRef),
					di.Svc(NewTestSvcStrArg, di.Ref(&barRef)).Children(
						di.SvcVal("foo"),
					),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"bar"}, svc.Args)
			},
		},
		{
			name: "child service can be accessed by Ref arg",
			build: func(b *di.Builder, refs *Refs) {
				var fooRef di.SvcReference
				b.Services(
					di.SvcVal("bar"),
					di.Svc(NewTestSvcStrArg, di.Ref(&fooRef)).Children(
						di.SvcVal("foo").Bind(&fooRef),
					),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: "sibling service can be accessed by Type arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.SvcVal("bar").Labels("sibling"),
					di.Svc(NewTestSvcStrArg, di.Type[string]("sibling")).Children(
						di.SvcVal("foo"),
					),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"bar"}, svc.Args)
			},
		},
		{
			name: "child service can access services from the outer scopes",
			build: func(b *di.Builder, refs *Refs) {
				var (
					foo   di.SvcReference
					child di.SvcReference
				)
				b.Services(
					di.Svc(Echo[string], di.Ref(&child)).
						Bind(refs.Svc.New("svc")).
						Children(
							di.Svc(Echo[string], di.Ref(&foo)).Bind(&child),
						),
					di.SvcVal(Echo("foo")).Bind(&foo),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				got, err := di.SvcByRef[string](c, refs.Svc.Get(t, "svc"))
				require.NoError(t, err)
				require.Equal(t, "foo", got)
			},
		},
		// Methods
		{
			name: "can register method calls",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, "foo").
						MethodCall((*AppendableEcho[string]).AppendVariadic, "bar", "baz"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"foo", "bar", "baz"}, svc.Echo())
			},
		},
		{
			name: "duplicated method call replaces the previous one",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, "foo").
						MethodCall((*AppendableEcho[string]).Append, "bar", "baz"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"bar", "baz"}, svc.Echo())
			},
		},
		{
			name: "can register method calls with refs",
			build: func(b *di.Builder, refs *Refs) {
				var fooRef di.SvcReference
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, di.Ref(&fooRef)),
					di.SvcVal("foo").Bind(&fooRef),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"foo"}, svc.Echo())
			},
		},
		{
			name: "can register method calls with Type arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, di.Type[[]string]()),
					di.SvcVal([]string{"foo", "bar"}),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"foo", "bar"}, svc.Echo())
			},
		},
		{
			name: "can register method calls with SliceOf arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, di.SliceOf[string]()),
					di.SvcVal("foo"),
					di.SvcVal("bar"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"foo", "bar"}, svc.Echo())
			},
		},
		{
			name: "can register method calls with label arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewAppendableEcho[string]).
						MethodCall((*AppendableEcho[string]).Append, di.SliceOf[string]("label")),
					di.SvcVal("foo"),
					di.SvcVal("bar").Labels("label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*AppendableEcho[string]](c)
				require.NoError(t, err)
				require.Equal(t, []string{"bar"}, svc.Echo())
			},
		},
		// Shared
		{
			name: "shared service is only instantiated once",
			build: func(b *di.Builder, refs *Refs) {
				var counter int
				b.Services(
					di.Svc(Increment, &counter).
						Bind(refs.Svc.New("incr")).
						Shared(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				incr1, err := di.SvcByRef[int](c, refs.Svc.Get(t, "incr"))
				require.NoError(t, err)
				require.Equal(t, 1, incr1)
				incr2, err := di.SvcByRef[int](c, refs.Svc.Get(t, "incr"))
				require.NoError(t, err)
				require.Equal(t, 1, incr2)
			},
		},
		{
			name: "non-shared service is instantiated on every retrieval",
			build: func(b *di.Builder, refs *Refs) {
				var counter int
				b.Services(
					di.Svc(Increment, &counter).
						Bind(refs.Svc.New("incr")).
						NotShared(),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				incr1, err := di.SvcByRef[int](c, refs.Svc.Get(t, "incr"))
				require.NoError(t, err)
				require.Equal(t, 1, incr1)
				incr2, err := di.SvcByRef[int](c, refs.Svc.Get(t, "incr"))
				require.NoError(t, err)
				require.Equal(t, 2, incr2)
			},
		},
		// Funcs
		{
			name: "can register and call a function by ref",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcNoArgs).Bind(refs.Func.New("foo")),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				got, err := di.ExecByRef(c, refs.Func.Get(t, "foo"))
				require.NoError(t, err)
				require.Len(t, got, 1)
				require.IsType(t, new(TestSvc), got[0])
			},
		},
		{
			name: "can register and call a function by label",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcNoArgs).Labels("my-label"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				got, err := di.ExecByLabel(c, "my-label")
				require.NoError(t, err)
				require.Len(t, got, 1)
				require.IsType(t, new(TestSvc), got[0])
			},
		},
		{
			name: "can register and call a function by type",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcNoArgs),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				got, err := di.ExecByType[func() *TestSvc](c)
				require.NoError(t, err)
				require.Len(t, got, 1)
				require.IsType(t, new(TestSvc), got[0])
			},
		},
		{
			name: "returns an error when executing a function that doesn't exist",
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				_, err := di.ExecByType[func() *TestSvc](c)
				require.ErrorContains(t, err, "function of type func() *godi_test.TestSvc not found")
			},
		},
		// Autowiring
		{
			name: "can autowire a service with a single arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcStrArg),
					di.SvcVal("foo"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo"}, svc.Args)
			},
		},
		{
			name: "can autowire a service with a slice arg",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs),
					di.SvcVal("foo"),
					di.SvcVal("bar"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "can autowire a service with a variadic args list",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcVariadicArgs),
					di.SvcVal("foo"),
					di.SvcVal("bar"),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar"}, svc.Args)
			},
		},
		{
			name: "autowiring picks exact type match for slices (slice arg)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcSliceArgs),
					di.SvcVal("foo"),
					di.SvcVal("bar"),
					di.SvcVal([]string{"slice-elem-1", "slice-elem-2"}),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"slice-elem-1", "slice-elem-2"}, svc.Args)
			},
		},
		{
			name: "autowiring picks exact type match for slices (variadic arg)",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcVariadicArgs),
					di.SvcVal("foo"),
					di.SvcVal("bar"),
					di.SvcVal([]string{"slice-elem-1", "slice-elem-2"}),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"slice-elem-1", "slice-elem-2"}, svc.Args)
			},
		},
		{
			name: "autowires only missing args",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(func(a, b string, i int, f float64) *TestSvc { return &TestSvc{Args: []any{a, b, i, f}} }, "foo", 4.2),
					di.SvcVal("bar"),
					di.SvcVal(42),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{"foo", "bar", 42, 4.2}, svc.Args)
			},
		},
		// Interface binding
		{
			name: "can bind an interface to a concrete type",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceArg),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a slice of interface to multiple implementations",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceSliceArgs),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a variadic list of interface to multiple implementations",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceVariadicArgs),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "returns a build error when multiple implementations of an interface are available",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcIfaceArg),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, `could not bind argument 0 of service github.com/michalkurzeja/godi/v2_test.(*TestSvc): multiple implementations of interface github.com/michalkurzeja/godi/v2_test.TestIface found: [github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl) github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl)]`)
			},
		},
		// Interface binding - method calls
		{
			name: "can bind an interface to a concrete type in a method call",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs).
						MethodCall((*TestSvc).AddArgIface),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a slice of interface to multiple implementations in a method call",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs).
						MethodCall((*TestSvc).AddArgsIfaceSlice),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a variadic list of interface to multiple implementations in a method call",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs).
						MethodCall((*TestSvc).AddArgsIfaceVariadic),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				svc, err := di.SvcByType[*TestSvc](c)
				require.NoError(t, err)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "returns a build error when multiple implementations of an interface are available in a method call",
			build: func(b *di.Builder, refs *Refs) {
				b.Services(
					di.Svc(NewTestSvcNoArgs).
						MethodCall((*TestSvc).AddArgIface),
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, `could not bind argument 1 of method github.com/michalkurzeja/godi/v2_test.(*TestSvc).AddArgIface: multiple implementations of interface github.com/michalkurzeja/godi/v2_test.TestIface found: [github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl) github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl)]`)
			},
		},
		// Interface binding - functions
		{
			name: "can bind an interface to a concrete type in a function",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcIfaceArg).Labels("fn"),
				).Services(
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				res, err := di.ExecByLabel(c, "fn")
				require.NoError(t, err)
				require.Len(t, res, 1)
				svc, ok := res[0].(*TestSvc)
				require.True(t, ok)
				require.Equal(t, []any{new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a slice of interface to multiple implementations in a function",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcIfaceSliceArgs).Labels("fn"),
				).Services(
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				res, err := di.ExecByLabel(c, "fn")
				require.NoError(t, err)
				require.Len(t, res, 1)
				svc, ok := res[0].(*TestSvc)
				require.True(t, ok)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "can bind a variadic list of interface to multiple implementations in a function",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcIfaceVariadicArgs).Labels("fn"),
				).Services(
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assert: func(t *testing.T, c di.Container, refs *Refs) {
				res, err := di.ExecByLabel(c, "fn")
				require.NoError(t, err)
				require.Len(t, res, 1)
				svc, ok := res[0].(*TestSvc)
				require.True(t, ok)
				require.Equal(t, []any{new(TestIfaceImpl), new(TestIfaceImpl)}, svc.Args)
			},
		},
		{
			name: "returns a build error when multiple implementations of an interface are available in a function",
			build: func(b *di.Builder, refs *Refs) {
				b.Functions(
					di.Func(NewTestSvcIfaceArg),
				).Services(
					di.SvcVal(new(TestIfaceImpl)),
					di.SvcVal(new(TestIfaceImpl)),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, `could not bind argument 0 of function github.com/michalkurzeja/godi/v2_test.NewTestSvcIfaceArg: multiple implementations of interface github.com/michalkurzeja/godi/v2_test.TestIface found: [github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl) github.com/michalkurzeja/godi/v2_test.(*TestIfaceImpl)]`)
			},
		},
		// Cycle
		{
			name: "returns a build error when a cycle is detected",
			build: func(b *di.Builder, refs *Refs) {
				var aRef, bRef, cRef di.SvcReference
				b.Services(
					di.Svc(Echo[string], di.Ref(&bRef)).
						Bind(&aRef).
						Labels("echo-a"),
					di.Svc(Echo[string], di.Ref(&cRef)).
						Bind(&bRef).
						Labels("echo-b"),
					di.Svc(Echo[string], di.Ref(&aRef)).
						Bind(&cRef).
						Labels("echo-c"),
				)
			},
			assertBuildErr: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "service string (echo-c) has a circular dependency on string (echo-a)")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			refs := Refs{Svc: make(SvcRefs), Func: make(FuncRefs)}
			builder := di.New()
			if tt.build != nil {
				tt.build(builder, &refs)
			}

			c, err := builder.Build()
			if tt.assertBuildErr != nil {
				tt.assertBuildErr(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.assert != nil {
				tt.assert(t, c, &refs)
			}
		})
	}
}

func TestDI_Eager(t *testing.T) {
	var (
		eagerCounter int
		lazyCounter  int
	)

	_, err := di.New().
		Services(
			di.Svc(Increment, &eagerCounter).Eager(),
			di.Svc(Increment, &lazyCounter).Lazy(),
		).Build()

	require.NoError(t, err)
	require.Equal(t, 1, eagerCounter)
	require.Equal(t, 0, lazyCounter)
}

type Refs struct {
	Svc  SvcRefs
	Func FuncRefs
}

type SvcRefs = RefsMap[di.SvcReference]
type FuncRefs = RefsMap[di.FuncReference]

type RefsMap[R di.SvcReference | di.FuncReference] map[string]*R

func (r RefsMap[R]) New(k string) *R {
	ref := new(R)
	r[k] = ref
	return ref
}

func (r RefsMap[R]) Get(t *testing.T, k string) R {
	ref, ok := r[k]
	require.Truef(t, ok, "reference not found by key: %s", k)
	return *ref
}

func TestGodi_Old(t *testing.T) {
	t.Run("can register and call a function by ref", func(t *testing.T) {
		t.Parallel()

		var fooRef, barRef di.FuncReference

		var fooCalled, barCalled bool

		c, err := di.New().
			Functions(
				di.Func(func() { fooCalled = true }).Bind(&fooRef),
				di.Func(func() { barCalled = true }).Bind(&barRef),
			).
			Build()
		require.NoError(t, err)

		require.False(t, fooCalled)
		require.False(t, barCalled)

		_, err = di.ExecByRef(c, fooRef)
		require.NoError(t, err)
		require.True(t, fooCalled)
		_, err = di.ExecByRef(c, barRef)
		require.NoError(t, err)
		require.True(t, barCalled)
	})
	t.Run("can register and call multiple functions by type", func(t *testing.T) {
		t.Parallel()

		type myFunc func()

		var fooCalled, barCalled bool

		c, err := di.New().
			Functions(
				di.Func(myFunc(func() { fooCalled = true })),
				di.Func(myFunc(func() { barCalled = true })),
			).
			Build()
		require.NoError(t, err)

		require.False(t, fooCalled)
		require.False(t, barCalled)

		_, err = di.ExecAllByType[myFunc](c)
		require.NoError(t, err)
		require.True(t, fooCalled)
		require.True(t, barCalled)
	})
	t.Run("can register and call multiple functions by label", func(t *testing.T) {
		t.Parallel()

		var fooCalled, barCalled bool

		c, err := di.New().
			Functions(
				di.Func(func() { fooCalled = true }).Labels("my-label"),
				di.Func(func() { barCalled = true }).Labels("my-label"),
			).
			Build()
		require.NoError(t, err)

		require.False(t, fooCalled)
		require.False(t, barCalled)

		_, err = di.ExecAllByLabel(c, "my-label")
		require.NoError(t, err)
		require.True(t, fooCalled)
		require.True(t, barCalled)
	})

	t.Run("can manually bind a slice of interface", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(false),
				di.Svc(fmt.Sprint),
			).
			Bindings(
				di.BindSlice[any, int](),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("can automatically bind an interface", func(t *testing.T) {
		t.Parallel()

		sprintOne := func(v any) string {
			return fmt.Sprint(v)
		}

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.Svc(sprintOne),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("can manually bind an interface", func(t *testing.T) {
		t.Parallel()

		sprintOne := func(v any) string {
			return fmt.Sprint(v)
		}

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(false),
				di.Svc(sprintOne),
			).
			Bindings(
				di.BindType[any, int](),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("can automatically bind a slice of interface", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.Svc(fmt.Sprint),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("automatic interface binding results in an error when multiple implementations are found", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.SvcVal(42),    // Implements any!
				di.SvcVal(false), // Also implements any!
				di.Svc(func(a any) string { return fmt.Sprint(a) }),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (interface binding) returned an error: could not bind argument 0 of service string: multiple implementations of interface interface {} found: [int bool]")
	})

	t.Run("autowire matches a single arg", func(t *testing.T) {
		t.Parallel()

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.Svc(strconv.Itoa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[string](c)
		require.NoError(t, err)
		require.Equal(t, "42", got)
	})
	t.Run("autowire matches a slice arg", func(t *testing.T) {
		t.Parallel()

		iitoaa := func(ii []int) []string {
			return lo.Map(ii, func(i int, _ int) string {
				return strconv.Itoa(i)
			})
		}
		c, err := di.New().
			Services(
				di.SvcVal([]int{42, 66}),
				di.Svc(iitoaa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[[]string](c)
		require.NoError(t, err)
		require.Equal(t, []string{"42", "66"}, got)
	})
	t.Run("autowire matches single values with a slice arg", func(t *testing.T) {
		t.Parallel()

		iitoaa := func(ii []int) []string {
			return lo.Map(ii, func(i int, _ int) string {
				return strconv.Itoa(i)
			})
		}
		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(66),
				di.Svc(iitoaa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[[]string](c)
		require.NoError(t, err)
		require.Equal(t, []string{"42", "66"}, got)
	})
	t.Run("autowire prefers slices over single values when matching a slice arg", func(t *testing.T) {
		t.Parallel()

		iitoaa := func(ii []int) []string {
			return lo.Map(ii, func(i int, _ int) string {
				return strconv.Itoa(i)
			})
		}
		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(66),
				di.SvcVal([]int{1, 2}),
				di.Svc(iitoaa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[[]string](c)
		require.NoError(t, err)
		require.Equal(t, []string{"1", "2"}, got)
	})
	t.Run("autowire works (variadic)", func(t *testing.T) {
		t.Parallel()

		iitoaa := func(ii ...int) []string {
			return lo.Map(ii, func(i int, _ int) string {
				return strconv.Itoa(i)
			})
		}

		c, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(66),
				di.Svc(iitoaa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[[]string](c)
		require.NoError(t, err)
		require.Equal(t, []string{"42", "66"}, got)
	})
	t.Run("can register a variadic service with no variadic args", func(t *testing.T) {
		t.Parallel()

		iitoaa := func(ii ...int) []string {
			return lo.Map(ii, func(i int, _ int) string {
				return strconv.Itoa(i)
			})
		}

		c, err := di.New().
			Services(
				di.Svc(iitoaa),
			).
			Build()
		require.NoError(t, err)

		got, err := di.SvcByType[[]string](c)
		require.NoError(t, err)
		require.Equal(t, []string{}, got)
	})

	t.Run("returns error when a service is missing an argument", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(strconv.Itoa).NotAutowired(),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service string: invalid factory strconv.Itoa: argument 0 is not set")
	})
	t.Run("returns error when autowired argument does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(strconv.Itoa),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service string: invalid factory strconv.Itoa: invalid argument 0: no services found for type int")
	})
	t.Run("returns error when autowired argument resolves to multiple services", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(66),
				di.Svc(strconv.Itoa),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service string: invalid factory strconv.Itoa: invalid argument 0: multiple services found for type int")
	})
	t.Run("returns error when method is missing an argument", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(NewAppendableEcho[string]).
					MethodCall((*AppendableEcho[string]).Append).
					NotAutowired(),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[string]): invalid method github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[...]).Append: argument 1 is not set")
	})
	t.Run("returns error when autowired method argument does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(NewAppendableEcho[string]).MethodCall((*AppendableEcho[string]).Append),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[string]): invalid method github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[...]).Append: invalid argument 1: no services found for type []string")
	})
	t.Run("returns error when autowired method argument resolves to multiple services", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.SvcVal([]string{"foo"}),
				di.SvcVal([]string{"bar"}),
				di.Svc(NewAppendableEcho[string]).MethodCall((*AppendableEcho[string]).Append),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid service github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[string]): invalid method github.com/michalkurzeja/godi/v2_test.(*AppendableEcho[...]).Append: invalid argument 1: multiple services found for type []string")
	})
	t.Run("returns error when a func is missing an argument", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Functions(
				di.Func(strconv.Itoa).NotAutowired(),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid function strconv.Itoa: argument 0 is not set")
	})
	t.Run("returns error when autowired func argument does not exist", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Functions(
				di.Func(strconv.Itoa),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid function strconv.Itoa: invalid argument 0: no services found for type int")
	})
	t.Run("returns error when autowired func argument resolves to multiple services", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.SvcVal(42),
				di.SvcVal(66),
			).
			Functions(
				di.Func(strconv.Itoa),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (argument validation) returned an error: invalid function strconv.Itoa: invalid argument 0: multiple services found for type int")
	})

	t.Run("returns error when a service is self-referencing", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(Echo[string]),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (cycle validation) returned an error: service string has a circular dependency on string")
	})
	t.Run("returns error when service dependencies are cyclic", func(t *testing.T) {
		t.Parallel()

		_, err := di.New().
			Services(
				di.Svc(func(string) int { return 0 }),
				di.Svc(func(int) bool { return false }),
				di.Svc(func(bool) string { return "" }),
			).
			Build()
		require.ErrorContains(t, err, "compilation failed: compiler pass (cycle validation) returned an error: service string has a circular dependency on bool")
	})
}

func Echo[T any](v T) T            { return v }
func EchoMany[T any](vs []T) []T   { return vs }
func EchoManyV[T any](vs ...T) []T { return vs }

type AppendableEcho[T any] struct {
	vals []T
}

func NewAppendableEcho[T any]() *AppendableEcho[T] {
	return &AppendableEcho[T]{}
}

func (e *AppendableEcho[T]) AppendVariadic(vs ...T) { e.vals = append(e.vals, vs...) }
func (e *AppendableEcho[T]) Append(vs []T)          { e.vals = append(e.vals, vs...) }

func (e *AppendableEcho[T]) Echo() []T {
	return e.vals
}

func Increment(i *int) int {
	*i++
	return *i
}
