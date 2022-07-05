package v0

import (
	"io/fs"

	"github.com/operator-framework/rukpak/pkg/manifest"
)

// Bundle holds the contents of a plain+v0 bundle.
type Bundle struct {
	manifest.FS

	// external settings
	installNamespace string
	targetNamespaces []string

	// Keep track of service accounts to avoid duplicates
	createdSvcAccs map[string]struct{}
}

// New creates a new plain+v0 bundle at the root of the given filesystem.
func New(fsys fs.FS) Bundle {
	if manifestFS, ok := fsys.(manifest.FS); ok {
		return Bundle{FS: manifestFS, createdSvcAccs: make(map[string]struct{})}
	}

	return Bundle{FS: manifest.New(fsys, manifest.WithManifestDirs("manifests"))}
}
