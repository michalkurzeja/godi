package di

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

// Container is a dependency injection container.
// It holds definitions of services and is responsible for building and
// storing instances of services.
type Container interface {
	Get(id ID) (any, error)
	GetByTag(tag TagID) ([]any, error)
	Has(id ID) bool
	CallFunction(id ID) error
	CallFunctions() error
	HasFunction(id ID) bool
	Initialised(id ID) bool
}

type container struct {
	definitions map[ID]*Definition
	aliases     map[ID]Alias
	functions   map[ID]*FunctionDefinition

	instances map[ID]any

	// Lookup maps:
	private map[ID]nothing
	byTag   map[TagID][]ID
}

func newContainer() *container {
	return &container{
		definitions: make(map[ID]*Definition),
		aliases:     make(map[ID]Alias),
		functions:   make(map[ID]*FunctionDefinition),

		instances: make(map[ID]any),

		private: make(map[ID]nothing),
		byTag:   make(map[TagID][]ID),
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

	if def.shared {
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

func (c *container) GetByTag(tag TagID) ([]any, error) {
	return c.getByTag(tag, true)
}

func (c *container) getByTag(tag TagID, filterPrivate bool) ([]any, error) {
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

func (c *container) HasFunction(id ID) bool {
	_, ok := c.functions[id]
	return ok
}

func (c *container) CallFunction(id ID) error {
	def, ok := c.functions[id]
	if !ok {
		return fmt.Errorf("function %s not found", id)
	}

	err := def.fn.call(c)
	if err != nil {
		return fmt.Errorf("function %s call error: %w", def, err)
	}

	return nil
}

func (c *container) CallFunctions() error {
	var errs *multierror.Error

	for _, id := range sortedAsc(lo.Keys(c.functions), func(id ID) ID { return id }) {
		err := c.CallFunction(id)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs.ErrorOrNil()
}

func (c *container) isPrivate(id ID) bool {
	_, ok := c.private[c.resolveAlias(id)]
	return ok
}
