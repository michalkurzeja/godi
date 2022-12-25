package di

import (
	"errors"
	"reflect"

	"github.com/dominikbraun/graph"
)

// Override returns an override node that can be used to replace a node in the container.
func Override[T any](c *TestContainer, val T) error {
	return OverrideWithID[T](c, FQN[T](), val)
}

func MustOverride[T any](c *TestContainer, val T) {
	if err := Override[T](c, val); err != nil {
		panic(err)
	}
}

// OverrideWithID returns an override node that can be used to replace a node in the container.
func OverrideWithID[T any](c *TestContainer, id string, val T) error {
	return c.Override(&override[T]{id: id, val: val})
}

func MustOverrideWithID[T any](c *TestContainer, id string, val T) {
	if err := OverrideWithID[T](c, id, val); err != nil {
		panic(err)
	}
}

// NewTestContainer returns an implementation of the Container for tests.
// It permits overriding nodes.
func NewTestContainer() *TestContainer {
	return &TestContainer{container: New().(*container)}
}

type TestContainer struct {
	*container
}

// Override replaces a node in the container (by its ID).
func (c *TestContainer) Override(node Node) error {
	v, err := c.graph.Vertex(node.ID())
	if errors.Is(err, graph.ErrVertexNotFound) {
		return NodeNotFoundError{ID: node.ID()}
	}
	if err != nil {
		return err
	}

	v.SwapNode(node)

	return nil
}

type override[T any] struct {
	id  string
	val T
}

func (o override[T]) ID() string {
	return o.id
}

func (o override[T]) Type() reflect.Type {
	return typeOf[T]()
}

func (o override[T]) Value(_ Container) (any, error) {
	return o.val, nil
}

func (o override[T]) Dependencies() []Node {
	return nil
}
