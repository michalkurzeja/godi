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
