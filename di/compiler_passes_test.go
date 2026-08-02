package di_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// BasePasses was exported with nothing able to consume it. A pass had no way to
// find out what it runs beside, and a CompilerPass would not say its own name,
// stage or priority.
//
// The pipeline can be read now. It still cannot be replaced, which is deliberate.
func TestAPassCanSeeWhatItRunsBeside(t *testing.T) {
	t.Parallel()

	var seen []string
	inspect := di.NewCompilerPass("inspect", di.PostFinalization, di.CompilerOpFunc(func(b *di.ContainerBuilder) error {
		for pass := range b.Compiler().Passes() {
			seen = append(seen, pass.Name())
		}
		return nil
	}))

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(inspect)

	_, err := b.Build()
	require.NoError(t, err)

	for _, want := range []string{"interface binding", "autowiring", "argument validation", "cycle validation", "inspect"} {
		require.Contains(t, seen, want)
	}
}

func TestAPassSaysWhenItRuns(t *testing.T) {
	t.Parallel()

	pass := di.NewCompilerPass("mine", di.PreValidation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		return nil
	})).WithPriority(7)

	require.Equal(t, "mine", pass.Name())
	require.Equal(t, di.PreValidation, pass.Stage())
	require.Equal(t, 7, pass.Priority())
}

// SkipCycleValidation is how that pass is turned off, and the pipeline is where
// it shows.
func TestTheBasePassesAreWhatTheCompilerStartsWith(t *testing.T) {
	t.Parallel()

	names := func(conf di.Config) []string {
		var out []string
		for pass := range di.NewContainerBuilder(conf).Compiler().Passes() {
			out = append(out, pass.Name())
		}
		return out
	}

	require.Contains(t, names(di.NewConfig()), "cycle validation")

	conf := di.NewConfig()
	conf.SkipCycleValidation = true
	require.NotContains(t, names(conf), "cycle validation")

	base := di.BasePasses(false)
	require.Len(t, base, 5)
	require.True(t, slices.ContainsFunc(base, func(p *di.CompilerPass) bool { return p.Name() == "autowiring" }))
}
