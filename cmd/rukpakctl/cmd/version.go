/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/operator-framework/rukpak/internal/version"
)

// newVersionCmd creates the version command
func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the rukpakctl version information.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.String())
		},
	}
	return cmd
}
