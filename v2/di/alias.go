package di

func NewAlias(target, aliasID ID) Alias {
	return Alias{id: aliasID, target: target}
}

func NewAliasT[T any](aliasID ID) Alias {
	return NewAlias(FQN[T](), aliasID)
}

func NewAliasTT[T, A any]() Alias {
	return NewAlias(FQN[T](), FQN[A]())
}

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
