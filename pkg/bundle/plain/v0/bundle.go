package v0

import (
	"io/fs"

	"github.com/operator-framework/rukpak/pkg/bundle"
)

// Bundle holds the contents of a plain+v0 bundle.
type Bundle struct {
	bundle.FS

	// external settings
	installNamespace string
	targetNamespaces []string

	// Keep track of service accounts to avoid duplicates
	createdSvcAccs map[string]struct{}
}

// New creates a new plain+v0 bundle at the root of the given filesystem.
func New(fsys fs.FS, opts ...func(*Bundle)) Bundle {
	b := Bundle{createdSvcAccs: make(map[string]struct{})}
	for _, opt := range opts {
		opt(&b)
	}

	if bundleFS, ok := fsys.(bundle.FS); ok {
		b.FS = bundleFS
	} else {
		b.FS = bundle.New(fsys, bundle.WithManifestDirs("manifests"))
	}

	return b
}
