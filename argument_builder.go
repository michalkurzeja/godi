package di

func Zero() *ArgumentBuilder {
	return &ArgumentBuilder{arg: NewZero()}
}

// Val returns a new argument builder for a Value.
func Val(v any) *ArgumentBuilder {
	return &ArgumentBuilder{arg: NewValue(v)}
}

// Ref returns a new argument builder for a Reference.
func Ref[T any](id ...ID) *ArgumentBuilder {
	refID := fqn(typeOf[T]())
	if len(id) > 0 {
		refID = id[len(id)-1]
	}

	return &ArgumentBuilder{arg: NewReference(refID, typeOf[T]())}
}

// Tagged returns a new argument builder for a TaggedCollection.
func Tagged[T any](tag Tag) *ArgumentBuilder {
	return &ArgumentBuilder{arg: NewTaggedCollection(tag, typeOf[T]())}
}

// ArgumentBuilder is a helper for building arguments.
// It is used by the DefinitionBuilder to define arguments for a service factory and method calls.
type ArgumentBuilder struct {
	arg Argument
}

// Idx sets the index of the argument.
// E.g. Idx(1) will set the argument as the second argument of a function.
func (b *ArgumentBuilder) Idx(i uint) *ArgumentBuilder {
	b.arg.setIndex(i)
	return b
}

func (b *ArgumentBuilder) Build() Argument {
	return b.arg
}
