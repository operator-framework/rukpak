package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var bundleDir string

	cmd := &cobra.Command{
		Use:  "unpack",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			bundleFS := os.DirFS(bundleDir)
			bundleContents := map[string][]byte{}
			if err := fs.WalkDir(bundleFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				data, err := fs.ReadFile(bundleFS, path)
				if err != nil {
					return fmt.Errorf("read file %q: %w", path, err)
				}
				bundleContents[path] = data
				return nil
			}); err != nil {
				log.Fatalf("walk bundle dir %q: %v", bundleDir, err)
			}

			encoder := json.NewEncoder(os.Stdout)
			if err := encoder.Encode(bundleContents); err != nil {
				log.Fatal(err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bundleDir, "bundle-dir", "", "directory in which the bundle can be found")
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
