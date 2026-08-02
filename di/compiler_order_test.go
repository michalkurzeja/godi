package di_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// Passes of the same priority run in the order they were added, which is what
// the docs promise. Twenty of them on purpose: Go sorts by insertion below twelve
// elements, so a smaller count would pass whether the sort kept ties in order or
// not.
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

// A pass registered while the queue is running joins the ties at its stage behind
// everything already waiting there. Registering it appends it, and the re-sort of
// the rest of the queue leaves ties where it found them.
//
// This is the guarantee that breaks if a pass is ever put into the queue anywhere
// but the end.
func TestAPassAddedMidRunGoesBehindTheOnesAlreadyQueued(t *testing.T) {
	t.Parallel()

	const count = 20

	var ran []string
	record := func(name string) di.CompilerOpFunc {
		return func(*di.ContainerBuilder) error {
			ran = append(ran, name)
			return nil
		}
	}

	adder := di.NewCompilerPass("adder", di.PreAutomation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		ran = append(ran, "adder")
		b.Compiler().AddPass(di.NewCompilerPass("added", di.PreAutomation, record("added")))
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(adder)
	for i := range count {
		name := fmt.Sprintf("queued %02d", i)
		b.Compiler().AddPass(di.NewCompilerPass(name, di.PreAutomation, record(name)))
	}

	_, err := b.Build()
	require.NoError(t, err)

	want := []string{"adder"}
	for i := range count {
		want = append(want, fmt.Sprintf("queued %02d", i))
	}
	want = append(want, "added")
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
