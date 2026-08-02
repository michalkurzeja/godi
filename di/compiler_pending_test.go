package di_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// Discovering what is registered and then scheduling work over it is a fair thing
// for a pass to want, and AddPass is reachable from inside one. It used to do
// nothing at all, silently, because the run had already taken the slice header.
func TestAPassCanAddAPass(t *testing.T) {
	t.Parallel()

	var ran bool
	adder := di.NewCompilerPass("adder", di.PreAutomation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		b.Compiler().AddPass(di.NewCompilerPass("added", di.Finalization, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
			ran = true
			return nil
		})))
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(adder)

	_, err := b.Build()
	require.NoError(t, err)
	require.True(t, ran, "the added pass never ran")
}

// A pass added at the stage that is already running is still to come, so it runs.
// Its place is after the one that added it.
func TestAPassAddedAtTheSameStageRunsAfterTheOneThatAddedIt(t *testing.T) {
	t.Parallel()

	var ran []string
	adder := di.NewCompilerPass("adder", di.PreAutomation, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		ran = append(ran, "adder")
		b.Compiler().AddPass(di.NewCompilerPass("added", di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
			ran = append(ran, "added")
			return nil
		})))
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(adder)

	_, err := b.Build()
	require.NoError(t, err)
	require.Equal(t, []string{"adder", "added"}, ran)
}

// Asking for a place that has already gone by has no honest answer: running it
// now runs the stages out of order, and running it later is not the place it
// asked for. So the build says so, naming both passes.
func TestAPassCannotAddOneThatWouldHaveRunBeforeIt(t *testing.T) {
	t.Parallel()

	var ran bool
	adder := di.NewCompilerPass("late arrival", di.Finalization, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		b.Compiler().AddPass(di.NewCompilerPass("too early", di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
			ran = true
			return nil
		})))
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(adder)

	_, err := b.Build()
	require.ErrorContains(t, err,
		"compiler pass (late arrival) added pass (too early) before itself")
	require.False(t, ran)
}
