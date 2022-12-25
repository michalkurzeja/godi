package di

import (
	"fmt"
	"io"
)

type Container interface {
	Register(node Node) error
	Compile() error
	Get(id string) (Node, error)
	Export(w io.Writer) error
}

func Register(c Container, services ...*ServiceBuilder) error {
	if len(services) == 0 {
		return fmt.Errorf("di: no services to register")
	}

	for _, builder := range services {
		if err := builder.build(c); err != nil {
			return fmt.Errorf("di: %w", err)
		}
	}

	err := c.Compile()
	if err != nil {
		return fmt.Errorf("di: %w", err)
	}

	return nil
}

func Get[T any](c Container, opts ...GetOptionsFunc) (T, error) {
	opt := getOptions{nodeID: FQN[T]()}
	for _, optFn := range opts {
		optFn(&opt)
	}

	node, err := c.Get(opt.nodeID)
	if err != nil {
		return zero[T](), fmt.Errorf("di: %w", err)
	}

	valAny, err := node.Value(c)
	if err != nil {
		return zero[T](), fmt.Errorf("di: %w", err)
	}

	val, ok := valAny.(T)
	if !ok {
		return zero[T](), fmt.Errorf(`di: service %s is of wrong type; expected %s; got %s`, opt.nodeID, FQN[T](), fqn(node.Type()))
	}

	return val, nil
}

func MustGet[T any](c Container, opts ...GetOptionsFunc) T {
	val, err := Get[T](c, opts...)
	if err != nil {
		panic(err)
	}
	return val
}

func Export(c Container, w io.Writer) error {
	return c.Export(w)
}

type getOptions struct {
	nodeID string
}

type GetOptionsFunc func(opt *getOptions)

func WithID(id string) GetOptionsFunc {
	return func(opt *getOptions) {
		opt.nodeID = id
	}
}
