/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd := &cobra.Command{
		Use:   "rukpakctl",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	}

	rootCmd.AddCommand(
		newContentCmd(),
		newCreateCmd(),
		newRunCmd(),
	)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
