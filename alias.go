package di

// NewAlias creates a new alias.
//
// If there's a service with ID bar, after declaring NewAlias("foo", "bar")
// it can now be referenced with both "bar" and "foo" IDs.
//
// Additionally, if any other service already had the ID "foo", it would get overwritten by the alias:
// retrieving "foo" from the container would get the aliased service ("bar") instead of the original one with that ID.
func NewAlias(aliasID, target ID) Alias {
	return Alias{id: aliasID, target: target}
}

// NewAliasT creates a new alias. The target ID is derived from the type parameter.
// NewAliasT[Foo]("bar") aliases service "bar" as "fooPkg.Foo".
func NewAliasT[A any](target ID) Alias {
	return NewAlias(FQN[A](), target)
}

// NewAliasTT creates a new alias. The target and alias IDs are derived from the type parameters.
// NewAliasTT[Foo, Bar]() aliases service "barPkg.Bar" as "fooPkg.Foo".
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
