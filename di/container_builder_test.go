package di_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/michalkurzeja/godi/v2/di"
)

// Whether the builder still holds its container is what tells a builder that
// has been spent from one that stopped partway, and anything reading a builder
// has to be able to tell them apart.
func TestASpentBuilderNoLongerHoldsItsContainer(t *testing.T) {
	t.Parallel()

	b := di.NewContainerBuilder(di.NewConfig())
	require.NotNil(t, b.Container())

	_, err := b.Build()
	require.NoError(t, err)
	require.Nil(t, b.Container())
}

func TestAFailedBuildKeepsItsContainer(t *testing.T) {
	t.Parallel()

	b := di.NewContainerBuilder(di.NewConfig())
	b.Compiler().AddPass(di.NewCompilerPass("stop", di.PreAutomation, di.CompilerOpFunc(func(*di.ContainerBuilder) error {
		return errors.New("no further")
	})))

	_, err := b.Build()
	require.Error(t, err)
	require.NotNil(t, b.Container(), "the container of where the compiler stopped is the one worth looking at")
}
