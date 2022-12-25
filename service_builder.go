package di

import "reflect"

func Svc(provider any) *ServiceBuilder {
	return &ServiceBuilder{provider: provider}
}

func SvcT[T any](provider any) *ServiceBuilder {
	return &ServiceBuilder{id: FQN[T](), provider: provider, t: typeOf[T]()}
}

type ServiceBuilder struct {
	id       string
	t        reflect.Type
	provider any
	deps     []Dependency
}

func (b *ServiceBuilder) ID(id string) *ServiceBuilder {
	b.id = id
	return b
}

func (b *ServiceBuilder) With(deps ...Dependency) *ServiceBuilder {
	b.deps = append(b.deps, deps...)
	return b
}

func (b *ServiceBuilder) build(c Container) (err error) {
	var service *lazyService
	if b.shouldInfer() {
		service, err = newLazyServiceWithAutoType(b.id, b.provider, b.deps...)
	} else {
		service, err = newLazyService(b.id, b.t, b.provider, b.deps...)
	}
	if err != nil {
		return err
	}
	return c.Register(service)
}

func (b *ServiceBuilder) shouldInfer() bool {
	return b.t == nil
}
