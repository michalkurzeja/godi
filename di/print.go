package di

import (
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"

	"github.com/michalkurzeja/godi/v2/internal/util"
)

// Print prints the contents of the container to the given writer.
func Print(c *Container, w io.Writer) {
	write := func(w io.Writer, s string) {
		_, _ = io.WriteString(w, s)
	}

	resolveBinding := func(arg Arg) Arg {
		if boundTo, ok := c.GetBindingFor(arg.Type()); ok {
			return boundTo
		}
		return arg
	}

	bindings := util.SortedAsc(lo.Values(c.bindings), func(b *InterfaceBinding) string { return util.Signature(b.Interface()) })
	svcDefs := c.services.GetAll()
	funcDefs := c.functions.GetAll()

	if len(bindings) > 0 {
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		write(w, "\tInterface bindings:\n")
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	}
	for _, binding := range bindings {
		write(w, fmt.Sprintf("%s -> %s\n", util.Signature(binding.Interface()), binding.BoundTo()))
	}

	if len(svcDefs) > 0 {
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		write(w, "\tServices:\n")
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	}
	for i, def := range svcDefs {
		if i != 0 {
			write(w, fmt.Sprintf("%s\n", strings.Repeat("-", 80)))
		}
		write(w, fmt.Sprintf("Type:\t\t%s\n", def))
		write(w, fmt.Sprintf("Factory:\t%s\n", def.FactoryName()))
		write(w, fmt.Sprintf("Autowire:\t%t\n", def.IsAutowired()))
		write(w, fmt.Sprintf("Shared:\t\t%t\n", def.IsShared()))
		write(w, fmt.Sprintf("Lazy:\t\t%t\n", def.IsLazy()))

		if len(def.GetFactory().Args().Slots()) > 0 {
			write(w, "Arguments:\n")
		}
		for _, slot := range def.GetFactory().Args().Slots() {
			write(w, fmt.Sprintf(" - %s\n", resolveBinding(slot.Arg())))
		}

		if len(def.GetMethodCalls()) > 0 {
			write(w, "Method calls:\n")
		}
		for _, method := range def.GetMethodCalls() {
			if len(method.Args().Slots()) > 0 {
				write(w, fmt.Sprintf(" - %s:\n", method.Name()))
			}
			for _, slot := range method.Args().Slots()[1:] {
				write(w, fmt.Sprintf("\t- %s\n", resolveBinding(slot.Arg())))
			}
		}
	}

	if len(funcDefs) > 0 {
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		write(w, "\tFunctions:\n")
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	}
	for i, def := range funcDefs {
		if i != 0 {
			write(w, fmt.Sprintf("%s\n", strings.Repeat("-", 80)))
		}
		write(w, fmt.Sprintf("Name:\t\t%s\n", def))
		write(w, fmt.Sprintf("Autowire:\t%t\n", def.IsAutowired()))
		write(w, fmt.Sprintf("Lazy:\t\t%t\n", def.IsLazy()))

		if len(def.GetFunc().Args().Slots()) > 0 {
			write(w, "Arguments:\n")
		}
		for _, slot := range def.GetFunc().Args().Slots() {
			write(w, fmt.Sprintf(" - %s\n", resolveBinding(slot.Arg())))
		}
	}
}
