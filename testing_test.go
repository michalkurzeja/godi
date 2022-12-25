package di_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	di "github.com/michalkurzeja/godi"
)

func TestTestContainer(t *testing.T) {
	c := di.NewTestContainer()

	override, _ := NewFoo("override")

	err := di.Override[Foo](c, override)
	assert.ErrorAs(t, err, &di.NodeNotFoundError{})

	err = di.Register(c, di.SvcT[Foo](NewFoo).With(di.Val("foo")))
	assert.NoError(t, err)

	got, err := di.Get[Foo](c)
	assert.NoError(t, err)
	assert.Equal(t, "foo", got.field)

	err = di.Override[Foo](c, override)
	assert.NoError(t, err)

	got, err = di.Get[Foo](c)
	assert.NoError(t, err)
	assert.Equal(t, "override", got.field)
}
