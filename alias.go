package di

// NewAlias creates a new alias.
// NewAlias("foo", "bar") aliases service "bar" as "foo".
func NewAlias(aliasID, target ID) Alias {
	return Alias{id: aliasID, target: target}
}

// NewAliasT creates a new alias. The target ID is derived from the type parameter.
// NewAliasT[Foo]("bar") aliases service "bar" as "Foo".
func NewAliasT[A any](target ID) Alias {
	return NewAlias(FQN[A](), target)
}

// NewAliasTT creates a new alias. The target and alias IDs are derived from the type parameters.
// NewAliasTT[Foo, Bar]() aliases service "Bar" as "Foo".
func NewAliasTT[A, T any]() Alias {
	return NewAlias(FQN[A](), FQN[T]())
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
