package di

func Val(v any) *ArgumentBuilder {
	return &ArgumentBuilder{
		arg: NewValue(v),
	}
}

func Ref[T any](id ...ID) *ArgumentBuilder {
	refID := fqn(typeOf[T]())
	if len(id) > 0 {
		refID = id[len(id)-1]
	}

	return &ArgumentBuilder{
		arg: NewReference(refID, typeOf[T]()),
	}
}

func Tagged[T any](tag Tag) *ArgumentBuilder {
	return &ArgumentBuilder{
		arg: NewTaggedCollection(tag, typeOf[T]()),
	}
}

type ArgumentBuilder struct {
	arg Argument
}

func (b *ArgumentBuilder) Idx(i uint) *ArgumentBuilder {
	b.arg.setIndex(i)
	return b
}

func (b *ArgumentBuilder) Build() Argument {
	return b.arg
}
