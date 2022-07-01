/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/homedir"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

var scheme *runtime.Scheme

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "rukpakctl",
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	if home := homedir.HomeDir(); home != "" {
		rootCmd.PersistentFlags().String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		rootCmd.PersistentFlags().String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	rootCmd.PersistentFlags().String("namespace", "rukpak-system", "namespace for target or work resources")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	scheme = runtime.NewScheme()
	err := rukpakv1alpha1.AddToScheme(scheme)
	if err != nil {
		fmt.Printf("failed to add schema: %+v\n", err)
		return
	}
}
