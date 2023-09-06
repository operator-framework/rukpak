package validator

import (
	"context"
	"errors"
	"fmt"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/controllers/v1alpha2/controllers/util"
	"github.com/operator-framework/rukpak/internal/convert"
	"github.com/spf13/afero"
)

type Validator interface {
	Validate(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error
}

type validator struct {
	formats map[v1alpha2.FormatType]Validator
}

func (v *validator) Validate(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error {
	format, ok := v.formats[bundleDeployment.Spec.Format]
	if !ok {
		return fmt.Errorf("format type not supported %q", bundleDeployment.Spec.Format)
	}
	return format.Validate(ctx, fs, bundleDeployment)
}

func NewDefaultValidator() Validator {
	return &validator{
		formats: map[v1alpha2.FormatType]Validator{
			v1alpha2.FormatRegistryV1: &registryV1Validator{},
			v1alpha2.FormatPlain:      &plainValidator{},
			v1alpha2.FormatHelm:       &helmValidator{},
		},
	}
}

type registryV1Validator struct{}

func (r *registryV1Validator) Validate(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error {
	plainFS, err := convert.RegistryV1ToPlain(fs)
	if err != nil {
		return fmt.Errorf("error converting registry+v1 bundle to plain+v0 bundle: %v", err)
	}
	return validateBundleObjects(plainFS)
}

type plainValidator struct{}

func (p *plainValidator) Validate(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error {
	return validateBundleObjects(fs)
}

type helmValidator struct{}

func (h *helmValidator) Validate(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error {
	// validate whether a single directory exists in its root and contains chart.yaml.
	return nil
}

func validateBundleObjects(fs afero.Fs) error {
	objects, err := util.GetBundleObjects(fs)
	if err != nil {
		return fmt.Errorf("error fetching objects from bundle manifests: %v", err)
	}
	if len(objects) == 0 {
		return errors.New("invalid bundle: found zero objects: plain+v0 bundles are required to contain at least one object")
	}
	return nil
}
