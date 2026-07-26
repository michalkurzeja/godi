package di_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// Passes of the same priority run in the order they were added, which is what
// the docs promise. Sorting is not stable, so it held only by accident: Go
// falls back to insertion sort below twelve elements, and an extension author
// adding a handful of passes at one stage is exactly the case that goes past
// that.
func TestPassesOfTheSamePriorityRunInTheOrderTheyWereAdded(t *testing.T) {
	t.Parallel()

	const count = 20

	var ran []string
	b := di.NewContainerBuilder(di.NewConfig())
	for i := range count {
		name := fmt.Sprintf("pass %02d", i)
		b.Compiler().AddPass(di.NewCompilerPass(name, di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
			ran = append(ran, name)
			return nil
		})))
	}

	_, err := b.Build()
	require.NoError(t, err)

	want := make([]string, count)
	for i := range want {
		want[i] = fmt.Sprintf("pass %02d", i)
	}
	require.Equal(t, want, ran)
}

// Priority still wins, and so does the stage: the order they were added is only
// the last word.
func TestPriorityDecidesBeforeTheOrderTheyWereAdded(t *testing.T) {
	t.Parallel()

	var ran []string
	record := func(name string) di.CompilerOpFunc {
		return func(*di.ContainerBuilder) error {
			ran = append(ran, name)
			return nil
		}
	}

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(di.NewCompilerPass("late", di.PreAutomation, record("late")).WithPriority(10))
	b.Compiler().AddPass(di.NewCompilerPass("early", di.PreAutomation, record("early")).WithPriority(-10))
	b.Compiler().AddPass(di.NewCompilerPass("last", di.Finalization, record("last")).WithPriority(-100))

	_, err := b.Build()
	require.NoError(t, err)

	require.Equal(t, []string{"early", "late", "last"}, ran)
}
