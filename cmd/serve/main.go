package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const port = ":8081"

func main() {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Servers static filesystem content through an HTTP server",
		RunE:  run,
	}

	cmd.Flags().StringP("directory", "d", ".", "configures the top-level directory for serving static filesystem content")

	if err := cmd.Execute(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	root, err := cmd.Flags().GetString("directory")
	if err != nil {
		return err
	}
	dir := http.Dir(root)

	logger := logrus.New()
	logger.Infof("serving filesystem content from %s directory on port %v", dir, port)
	http.Handle("/", http.FileServer(dir))
	logger.Fatal(http.ListenAndServe(port, nil))

	return nil
}
