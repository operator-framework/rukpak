package cmd

import "github.com/spf13/cobra"

func newAlphaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "alpha",
		Short:  "unstable or experimental subcommands",
		Hidden: false,
	}

	cmd.AddCommand(
		newAlphaBootstrapCmd(),
	)

	if !cmd.HasSubCommands() {
		return nil
	}

	// If all of the 'alpha' subcommands are hidden, hide the alpha command
	// and unhide the alpha subcommands.
	if allHidden(cmd.Commands()) {
		cmd.Hidden = true
		for _, c := range cmd.Commands() {
			c.Hidden = false
		}
	}

	return cmd
}

func allHidden(cs []*cobra.Command) bool {
	for _, c := range cs {
		if !c.Hidden {
			return false
		}
	}
	return true
}
