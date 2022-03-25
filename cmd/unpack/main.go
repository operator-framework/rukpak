package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/operator-framework/rukpak/internal/version"
)

func main() {
	var bundleDir string
	var rukpakVersion bool

	skipRootPaths := sets.NewString(
		"/dev",
		"/etc",
		"/proc",
		"/product_name",
		"/product_uuid",
		"/sys",
		"/bin",
	)
	cmd := &cobra.Command{
		Use:  "unpack",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if rukpakVersion {
				fmt.Printf("Git commit: %s\n", version.String())
				os.Exit(0)
			}
			var err error
			bundleDir, err = filepath.Abs(bundleDir)
			if err != nil {
				log.Fatalf("get absolute path of bundle directory %q: %v", bundleDir, err)
			}

			bundleFS := os.DirFS(bundleDir)
			bundleContents := map[string][]byte{}
			if err := fs.WalkDir(bundleFS, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if bundleDir == "/" {
					// If bundleDir is the filesystem root, skip some known unrelated directories
					fullPath := filepath.Join(bundleDir, path)
					if skipRootPaths.Has(fullPath) {
						return filepath.SkipDir
					}
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
	cmd.Flags().BoolVar(&rukpakVersion, "version", false, "displays rukpak version information")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
