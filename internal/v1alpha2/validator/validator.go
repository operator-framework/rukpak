/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validator

import (
	"context"
	"errors"
	"fmt"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/spf13/afero"

	"github.com/operator-framework/rukpak/internal/v1alpha2/convert"
	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
	"github.com/operator-framework/rukpak/internal/v1alpha2/util"
)

// validator will validate the plain, registry and helm formats.
type validator struct {
	formats map[v1alpha2.FormatType]Validator
}

func (v *validator) Validate(ctx context.Context, formatType v1alpha2.FormatType, store store.Store) error {
	format, ok := v.formats[formatType]
	if !ok {
		return fmt.Errorf("format type not supported %q", formatType)
	}
	return format.Validate(ctx, formatType, store)
}

// NewDefaultValidator returns the set of validators that can validate the formats registered to it.
func NewDefaultValidator() Validator {
	return &validator{
		formats: map[v1alpha2.FormatType]Validator{
			v1alpha2.FormatRegistryV1: &registryV1Validator{},
			v1alpha2.FormatPlain:      &plainValidator{},
			v1alpha2.FormatHelm:       &helmValidator{},
		},
	}
}

// registryV1Validator validates bundles of registry v1 format.
type registryV1Validator struct{}

func (r *registryV1Validator) Validate(_ context.Context, _ v1alpha2.FormatType, store store.Store) error {
	plainFS, err := convert.RegistryV1ToPlain(store)
	if err != nil {
		return fmt.Errorf("error converting registry+v1 bundle to plain+v0 bundle: %v", err)
	}
	return validateBundleObjects(plainFS)
}

// registryV1Validator validates bundles of plain format.
type plainValidator struct{}

func (p *plainValidator) Validate(_ context.Context, _ v1alpha2.FormatType, store store.Store) error {
	return validateBundleObjects(store)
}

// registryV1Validator validates helm bundles.
type helmValidator struct{}

func (h *helmValidator) Validate(_ context.Context, _ v1alpha2.FormatType, store store.Store) error {
	rootFSEntries, err := afero.ReadDir(store, ".")
	if err != nil {
		return err
	}

	if len(rootFSEntries) == 1 && rootFSEntries[0].IsDir() {
		return nil
	}

	// In certain cases, `chart.yaml` can also end up being in the current directory instead of
	// <chart-Dir>/chart.yaml. This can happen in scenarios where `rukpak run` is called from the
	// parent dir, and the uploaded contents contain the helm manifests in root dir.
	exists, err := afero.Exists(store, "Chart.yaml")
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("could not find Chart.yaml %v", err)
	}
	return nil
}

func validateBundleObjects(fs afero.Fs) error {
	objects, err := util.GetBundleObjects(fs)
	if err != nil {
		return fmt.Errorf("fetching objects from bundle manifests: %v", err)
	}
	if len(objects) == 0 {
		return errors.New("invalid bundle: found zero objects: plain+v0 bundles are required to contain at least one object")
	}
	return nil
}
