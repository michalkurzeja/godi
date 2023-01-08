package di

// NewAlias creates a new alias.
// NewAlias("foo", "bar") aliases service "foo" as "bar".
func NewAlias(target, aliasID ID) Alias {
	return Alias{id: aliasID, target: target}
}

// NewAliasT creates a new alias. The target ID is derived from the type parameter.
// NewAliasT[Foo]("bar") aliases service "Foo" as "bar".
func NewAliasT[T any](aliasID ID) Alias {
	return NewAlias(FQN[T](), aliasID)
}

// NewAliasTT creates a new alias. The target and alias IDs are derived from the type parameters.
// NewAliasTT[Foo, Bar]() aliases service "Foo" as "Bar".
func NewAliasTT[T, A any]() Alias {
	return NewAlias(FQN[T](), FQN[A]())
}

// Alias represents an additional ID for a service.
// Any number of aliases can be created for a single service.
// Services may be referenced by their ID or any of their aliases.
type Alias struct {
	id     ID
	target ID
}

func (a Alias) ID() ID {
	return a.id
}

func (a Alias) Target() ID {
	return a.target
}
