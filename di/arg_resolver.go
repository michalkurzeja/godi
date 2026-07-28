package di

// ArgResolver resolves arguments. Each kind of argument does its own resolving,
// so this is the three functions above under an older name.
//
// Deprecated: use the functions directly: ValidateArg, ResolveArg, ResolveArgIDs.
type ArgResolver struct{}

func NewArgResolver() *ArgResolver {
	return &ArgResolver{}
}

func (r *ArgResolver) Validate(scope *Scope, arg Arg) error {
	return ValidateArg(scope, arg)
}

func (r *ArgResolver) Resolve(scope *Scope, arg Arg) (any, error) {
	return ResolveArg(scope, arg)
}

func (r *ArgResolver) ResolveIDs(scope *Scope, arg Arg) []ID {
	return ResolveArgIDs(scope, arg)
}
