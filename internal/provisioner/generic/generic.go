package generic

import (
	"context"
	"io/fs"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	// ProvisionerID is the unique generic provisioner ID
	ProvisionerID = "core-rukpak-io-generic"
)

func HandleBundle(_ context.Context, fsys fs.FS, _ *rukpakv1alpha1.Bundle) (fs.FS, error) {
	return fsys, nil
}
