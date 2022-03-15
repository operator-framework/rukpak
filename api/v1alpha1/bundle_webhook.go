/*
Copyright 2022.

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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Maximum length of bundle name
const maxNameLength = 40

// log is for logging in this package.
var bundlelog = logf.Log.WithName("bundle-resource")

func (r *Bundle) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-core-rukpak-io-v1alpha1-bundle,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.rukpak.io,resources=bundles,verbs=create;update,versions=v1alpha1,name=core.rukpak.io,admissionReviewVersions=v1

var _ webhook.Validator = &Bundle{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Bundle) ValidateCreate() error {
	bundlelog.V(1).Info("validate create", "name", r.Name)

	return checkNameLength(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Bundle) ValidateUpdate(old runtime.Object) error {
	bundlelog.V(1).Info("validate update", "name", r.Name)

	return checkNameLength(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Bundle) ValidateDelete() error {
	bundlelog.V(1).Info("validate delete", "name", r.Name)

	return nil
}

func checkNameLength(r *Bundle) error {
	if len(r.Name) > maxNameLength {
		return fmt.Errorf("bundle name %s is too long: maximum allowed name length is %d", r.GetName(), maxNameLength)
	}
	return nil
}
