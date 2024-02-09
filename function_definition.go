package di

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"
)

type FunctionDefinition struct {
	id ID
	fn *Function

	autowired bool
}

func NewFunctionDefinition(id ID, fn *Function) *FunctionDefinition {
	return &FunctionDefinition{
		id: id,
		fn: fn,

		autowired: DefaultAutowired,
	}
}

func (d *FunctionDefinition) ID() ID {
	return d.id
}

func (d *FunctionDefinition) GetFunction() *Function {
	return d.fn
}

func (d *FunctionDefinition) IsAutowired() bool {
	return d.autowired
}

func (d *FunctionDefinition) SetAutowired(autowired bool) {
	d.autowired = autowired
}

func (d *FunctionDefinition) String() string {
	return string(d.ID())
}

type FunctionDefinitionBuilder struct {
	def *FunctionDefinition
	err *multierror.Error
}

func Func(id ID, fn any, args ...any) *FunctionDefinitionBuilder {
	var bld FunctionDefinitionBuilder

	function, err := NewFunction(fn, lo.Map(args, func(arg any, _ int) Argument {
		if builder, ok := arg.(*ArgumentBuilder); ok {
			return builder.Build()
		}
		return Val(arg).Build()
	})...)
	if err != nil {
		bld.err = multierror.Append(bld.err, fmt.Errorf("invalid function: %w", err))
	}

	bld.def = NewFunctionDefinition(id, function)

	return &bld
}

func (b *FunctionDefinitionBuilder) Build() (*FunctionDefinition, error) {
	return b.def, b.err.ErrorOrNil()
}
