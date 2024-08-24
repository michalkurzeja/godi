package extras

import (
	"errors"
	"fmt"

	"github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
	"github.com/michalkurzeja/godi/v2/internal/errorsx"
)

// OverrideSvcArgPass overrides an argument of the referenced service with the one provided.
// slotIdx is the index of the argument to override.
// arg is the argument to override the argument with. It can be a literal value (e.g. "foo" or 42) or an *godi.ArgBuilder (e.g. godi.Val("foo") or godi.Type[MyType]()).
func OverrideSvcArgPass(ref godi.SvcReference, slotIdx uint, arg any) *di.CompilerPass {
	return di.NewCompilerPass("override arg", di.PreAutomation, di.CompilerOpFunc(func(builder *di.ContainerBuilder) error {
		if ref.IsEmpty() {
			return errors.New("cannot override argument: empty reference")
		}
		def, ok := builder.GetServiceDefinition(ref.SvcID())
		if !ok {
			return fmt.Errorf("cannot override argument of %s: service not found", ref)
		}
		a, err := godi.Arg(arg).Build()
		if err != nil {
			return errorsx.Wrap(err, "cannot override argument of %s: invalid arg")
		}
		err = def.GetFactory().Args().SetSlot(di.NewSlottedArg(a, slotIdx))
		if err != nil {
			return errorsx.Wrapf(err, "cannot override argument of %s", ref)
		}
		return nil
	}))
}

// OverrideFuncArgPass overrides an argument of the referenced function with the one provided.
// slotIdx is the index of the argument to override.
// arg is the argument to override the argument with. It can be a literal value (e.g. "foo" or 42) or an *godi.ArgBuilder (e.g. godi.Val("foo") or godi.Type[MyType]()).
func OverrideFuncArgPass(ref godi.FuncReference, slotIdx uint, arg any) *di.CompilerPass {
	return di.NewCompilerPass("override arg", di.PreAutomation, di.CompilerOpFunc(func(builder *di.ContainerBuilder) error {
		if ref.IsEmpty() {
			return errors.New("cannot override argument: empty reference")
		}
		def, ok := builder.GetFunctionDefinition(ref.FuncID())
		if !ok {
			return fmt.Errorf("cannot override argument of %s: function not found", ref)
		}
		a, err := godi.Arg(arg).Build()
		if err != nil {
			return errorsx.Wrap(err, "cannot override argument of %s: invalid arg")
		}
		err = def.GetFunc().Args().SetSlot(di.NewSlottedArg(a, slotIdx))
		if err != nil {
			return errorsx.Wrapf(err, "cannot override argument of %s", ref)
		}
		return nil
	}))
}
