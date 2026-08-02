package di_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/michalkurzeja/godi/v2"
)

type benchDep struct{ n int }

type benchSvc struct{}

func newBenchDep(n int) *benchDep { return &benchDep{n: n} }

func newBenchSvc(*benchDep, string, int) *benchSvc { return &benchSvc{} }

// BenchmarkResolve measures the warm path: every service is already built, so a
// resolve is a lookup and nothing else. BenchmarkResolveParallel is the one that
// matters — construction serialises, and this is the evidence that retrieval
// does not.
func BenchmarkResolve(b *testing.B) {
	c := benchContainer(b)

	for b.Loop() {
		_, err := di.SvcByType[*benchSvc](c)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveParallel(b *testing.B) {
	c := benchContainer(b)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := di.SvcByType[*benchSvc](c)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func benchContainer(b *testing.B) di.Container {
	b.Helper()

	dep := new(di.SvcReference)
	c, err := di.New().
		Services(
			di.Svc(newBenchDep, 1).Bind(dep),
			di.Svc(newBenchSvc, di.Ref(dep), "literal", 1),
		).
		Build()
	require.NoError(b, err)

	_, err = di.SvcByType[*benchSvc](c) // Warm it, so the loop never constructs.
	require.NoError(b, err)

	return c
}

// BenchmarkBuild measures container compilation, which is where the Compiler
// walks every slot and binding once per pass to attribute them.
func BenchmarkBuild(b *testing.B) {
	for _, size := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("services=%d", size), func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				builder := di.New()
				for i := range size {
					ref := new(di.SvcReference)
					builder.Services(
						di.Svc(newBenchDep, i).Bind(ref),
						di.Svc(newBenchSvc, di.Ref(ref), "literal", i),
					)
				}
				b.StartTimer()

				_, err := builder.Build()
				require.NoError(b, err)
			}
		})
	}
}
