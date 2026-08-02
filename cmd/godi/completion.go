package main

import (
	"github.com/spf13/cobra"
)

// Cobra generates the completion scripts itself, so `godi completion zsh` and
// friends come for free. What it cannot know is what a flag's *value* may be.
// Without the two helpers here, the shell offers filenames where a choice
// belongs.

// completeWith offers a fixed set of values for a flag, and nothing else.
func completeWith(cmd *cobra.Command, flag string, values []string) {
	err := cmd.RegisterFlagCompletionFunc(flag,
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		})
	if err != nil {
		// The only way here is naming a flag that was never registered. That is a
		// typo a few lines above, and a test that builds the command catches it.
		panic(err)
	}
}

// completeGraphFiles offers .json files for the graph to read, and nothing once one
// has been named. Every command here takes a single file.
func completeGraphFiles(cmd *cobra.Command) {
	cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return []string{"json"}, cobra.ShellCompDirectiveFilterFileExt
	}
}
