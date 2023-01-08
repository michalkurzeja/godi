package di

import (
	"fmt"
	"io"
	"strings"

	"github.com/samber/lo"
)

// Print prints the contents of the container to the given writer.
func Print(c Container, w io.Writer) error {
	cc := c.(*container)
	aliases := sorted(lo.Values(cc.aliases), func(a Alias) ID {
		return a.ID()
	})
	definitions := sorted(lo.Values(cc.definitions), func(d *Definition) ID {
		return d.ID()
	})

	if len(aliases) > 0 {
		io.WriteString(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		io.WriteString(w, "Aliases:\n")
	}
	for _, alias := range aliases {
		io.WriteString(w, fmt.Sprintf("%s -> %s\n", alias.ID(), alias.Target()))
	}

	if len(definitions) > 0 {
		io.WriteString(w, fmt.Sprintf("%s\n", strings.Repeat("=", 80)))
		io.WriteString(w, "Services:\n")
	}
	for _, def := range definitions {
		io.WriteString(w, fmt.Sprintf("%s\n", strings.Repeat("-", 80)))
		io.WriteString(w, fmt.Sprintf("Name:\t\t%s\n", def))
		io.WriteString(w, fmt.Sprintf("Autowire:\t%t\n", def.IsAutowire()))
		io.WriteString(w, fmt.Sprintf("Public:\t\t%t\n", def.IsPublic()))
		io.WriteString(w, fmt.Sprintf("Cached:\t\t%t\n", def.IsCached()))
		io.WriteString(w, fmt.Sprintf("Lazy:\t\t%t\n", def.IsLazy()))

		if len(def.GetFactory().GetArgs()) > 0 {
			io.WriteString(w, "Arguments:\n")
		}
		for _, arg := range def.GetFactory().GetArgs() {
			io.WriteString(w, fmt.Sprintf(" - %s\n", arg.Argument()))
		}

		if len(def.GetMethodCalls()) > 0 {
			io.WriteString(w, "Method calls:\n")
		}
		for _, method := range def.GetMethodCalls() {
			if len(method.GetArgs()) > 0 {
				io.WriteString(w, fmt.Sprintf(" - %s:\n", method.Name()))
			}
			for _, arg := range method.GetArgs()[1:] {
				io.WriteString(w, fmt.Sprintf("\t- %s\n", arg.Argument()))
			}
		}
	}

	return nil
}
