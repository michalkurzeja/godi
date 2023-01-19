package di

import (
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
)

// Print prints the contents of the container to the given writer.
func Print(c Container, w io.Writer) error {
	write := func(w io.Writer, s string) {
		_, _ = io.WriteString(w, s)
	}

	resolveAlias := func(cc *container, arg Argument) Argument {
		if ref, ok := arg.(*Reference); ok {
			return NewReference(cc.resolveAlias(ref.ID()), arg.Type())
		}
		return arg
	}

	cc := c.(*container)
	aliases := sortedAsc(lo.Values(cc.aliases), func(a Alias) ID {
		return a.ID()
	})
	definitions := sortedAsc(lo.Values(cc.definitions), func(d *Definition) ID {
		return d.ID()
	})

	if len(aliases) > 0 {
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		write(w, "\tAliases:\n")
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	}
	for _, alias := range aliases {
		write(w, fmt.Sprintf("%s -> %s\n", alias.ID(), alias.Target()))
	}

	if len(definitions) > 0 {
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		write(w, "\tServices:\n")
		write(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
	}
	for i, def := range definitions {
		if i != 0 {
			write(w, fmt.Sprintf("%s\n", strings.Repeat("-", 80)))
		}
		write(w, fmt.Sprintf("ID:\t\t%s\n", def))
		write(w, fmt.Sprintf("Autowire:\t%t\n", def.IsAutowired()))
		write(w, fmt.Sprintf("Public:\t\t%t\n", def.IsPublic()))
		write(w, fmt.Sprintf("Shared:\t\t%t\n", def.IsShared()))
		write(w, fmt.Sprintf("Lazy:\t\t%t\n", def.IsLazy()))

		if len(def.GetFactory().GetArgs()) > 0 {
			write(w, "Arguments:\n")
		}
		for _, arg := range def.GetFactory().GetArgs() {
			write(w, fmt.Sprintf(" - %s\n", resolveAlias(cc, arg.Argument())))
		}

		if len(def.GetMethodCalls()) > 0 {
			write(w, "Method calls:\n")
		}
		for _, method := range def.GetMethodCalls() {
			if len(method.GetArgs()) > 0 {
				write(w, fmt.Sprintf(" - %s:\n", method.Name()))
			}
			for _, arg := range method.GetArgs()[1:] {
				write(w, fmt.Sprintf("\t- %s\n", resolveAlias(cc, arg.Argument())))
			}
		}
		if len(def.GetTags()) > 0 {
			write(w, "Tags:\n")
		}
		for _, tag := range def.GetTags() {
			params := lo.Map(lo.Entries(tag.Params()), func(e lo.Entry[string, any], _ int) string {
				return fmt.Sprintf("%s:%v", e.Key, e.Value)
			})
			write(w, fmt.Sprintf(" - %s %s\n", tag, params))
		}
	}

	return nil
}
