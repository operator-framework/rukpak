package v1alpha2

import (
	"context"
	"io"
	"io/fs"

	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultBundleCacheDir = "/var/cache"

type Storage interface {
	Loader
	Storer
}

type Loader interface {
}

type Storer interface {
	// CreateParentDir creates a base filesystem path where all the contents
	// for a single bundle entity needs to be unpacked.
	CreateParentDir(name string) (string, error)

	// Copy copies content from source to destination path.
	// The existence of both the location in the filesystem is the
	// responsibility of the caller. This is useful for git source.
	// Remove this if not needed.
	Copy(context context.Context, source, destination string) error

	// Store reads the contents from reader and stores the contents
	// on the destination filesystem at the destination. The destination
	// path is defined with respect to the root of Fs.
	Store(context context.Context, reader io.Reader, destinationFs afero.Fs, destination string) error
}

type LocalDirectory struct {
	RootDirectory string
}

func (s *LocalDirectory) Load(_ context.Context, owner client.Object) (fs.FS, error) {
	return nil, nil
}
