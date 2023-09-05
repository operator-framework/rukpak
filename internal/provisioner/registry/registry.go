package registry

import (
	"context"
	"io/fs"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	// ProvisionerID is the unique registry provisioner ID
	ProvisionerID = "core-rukpak-io-registry"
)

func HandleBundle(_ context.Context, fsys fs.FS, _ *rukpakv1alpha1.Bundle) (fs.FS, error) {
	return nil, nil
}
