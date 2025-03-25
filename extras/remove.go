package extras

import (
	"errors"

	godi "github.com/michalkurzeja/godi/v2"
	"github.com/michalkurzeja/godi/v2/di"
)

func RemoveSvc(ref *godi.SvcReference) *di.CompilerPass {
	return di.NewCompilerPass("remove svc", di.PreAutomation, di.CompilerOpFunc(func(builder *di.ContainerBuilder) error {
		if ref.IsEmpty() {
			return errors.New("cannot remove service: empty reference")
		}
		builder.RootScope().RemoveServiceDefinitions(ref.SvcID())
		return nil
	}))
}

func RemoveFunc(ref *godi.FuncReference) *di.CompilerPass {
	return di.NewCompilerPass("remove func", di.PreAutomation, di.CompilerOpFunc(func(builder *di.ContainerBuilder) error {
		if ref.IsEmpty() {
			return errors.New("cannot remove function: empty reference")
		}
		builder.RootScope().RemoveFunctionDefinitions(ref.FuncID())
		return nil
	}))
}
