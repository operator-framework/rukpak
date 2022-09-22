package registry

import (
	"context"
	"fmt"
	"io/fs"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/convert"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
)

const (
	// ProvisionerID is the unique registry provisioner ID
	ProvisionerID = "core-rukpak-io-registry"
)

func HandleBundle(_ context.Context, fsys fs.FS, _ *rukpakv1alpha1.Bundle) (fs.FS, error) {
	plainFS, err := convert.RegistryV1ToPlain(fsys)
	if err != nil {
		return nil, fmt.Errorf("convert registry+v1 bundle to plain+v0 bundle: %v", err)
	}

	if err := plain.ValidateBundle(plainFS); err != nil {
		return nil, fmt.Errorf("validate bundle: %v", err)
	}
	return plainFS, nil
}
