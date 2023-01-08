package di

import (
	"fmt"
)

type Container interface {
	Get(id ID) (any, error)
	GetByTag(tag Tag) ([]any, error)
	Has(id ID) bool
	Initialised(id ID) bool
}

type container struct {
	definitions map[ID]*Definition
	aliases     map[ID]Alias

	instances map[ID]any

	// Lookup maps:
	private map[ID]struct{}
	byTag   map[Tag][]ID
}

func newContainer() *container {
	return &container{
		definitions: make(map[ID]*Definition),
		aliases:     make(map[ID]Alias),

		instances: make(map[ID]any),

		private: make(map[ID]struct{}),
		byTag:   make(map[Tag][]ID),
	}
}

func (c *container) Has(id ID) bool {
	id = c.resolveAlias(id)

	_, ok := c.instances[id]
	if ok {
		return true
	}
	_, ok = c.definitions[id]
	return ok
}

func (c *container) resolveAlias(id ID) ID {
	alias, ok := c.aliases[id]
	if ok {
		return alias.target
	}
	return id
}

func (c *container) Initialised(id ID) bool {
	id = c.resolveAlias(id)

	_, ok := c.instances[id]
	return ok
}

func (c *container) Get(id ID) (any, error) {
	return c.get(id, true)
}

func (c *container) get(id ID, filterPrivate bool) (any, error) {
	if filterPrivate && c.isPrivate(id) {
		return nil, fmt.Errorf("service %s is private", id)
	}

	id = c.resolveAlias(id)

	svc, ok := c.instances[id]
	if ok {
		return svc, nil
	}

	def, ok := c.definitions[id]
	if !ok {
		return nil, fmt.Errorf("service %s not found", id)
	}

	svc, err := c.instantiate(def)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate service %s: %w", id, err)
	}

	return svc, nil
}

func (c *container) instantiate(def *Definition) (any, error) {
	svc, err := def.factory.call(c)
	if err != nil {
		return nil, err
	}

	if def.cached {
		c.instances[def.id] = svc
	}

	for _, method := range def.methodCalls {
		err = method.call(c, svc)
		if err != nil {
			return nil, err
		}
	}

	return svc, nil
}

func (c *container) GetByTag(tag Tag) ([]any, error) {
	return c.getByTag(tag, true)
}

func (c *container) getByTag(tag Tag, filterPrivate bool) ([]any, error) {
	ids, ok := c.byTag[tag]
	if !ok {
		return nil, nil
	}

	svcs := make([]any, 0, len(ids))
	for _, id := range ids {
		if filterPrivate && c.isPrivate(id) {
			continue
		}

		svc, err := c.get(id, filterPrivate)
		if err != nil {
			return nil, err
		}

		svcs = append(svcs, svc)
	}

	return svcs, nil
}

func (c *container) isPrivate(id ID) bool {
	_, ok := c.private[c.resolveAlias(id)]
	return ok
}
