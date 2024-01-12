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

package webhook

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
)

type BundleDeployment struct {
	Client          client.Client
	SystemNamespace string
}

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=list;watch
//+kubebuilder:webhook:path=/validate-core-rukpak-io-v1alpha2-bundledeployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.rukpak.io,resources=bundledeployments,verbs=create;update,versions=v1alpha2,name=vbundles.core.rukpak.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (b *BundleDeployment) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	bundleDeployment := obj.(*rukpakv1alpha2.BundleDeployment)
	return b.checkBundleDeploymentSource(ctx, bundleDeployment)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (b *BundleDeployment) ValidateUpdate(ctx context.Context, _ runtime.Object, newObj runtime.Object) (admission.Warnings, error) {
	newBundle := newObj.(*rukpakv1alpha2.BundleDeployment)
	return b.checkBundleDeploymentSource(ctx, newBundle)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (b *BundleDeployment) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (b *BundleDeployment) checkBundleDeploymentSource(ctx context.Context, bundleDeployment *rukpakv1alpha2.BundleDeployment) (admission.Warnings, error) {
	switch typ := bundleDeployment.Spec.Source.Type; typ {
	case rukpakv1alpha2.SourceTypeImage:
		if bundleDeployment.Spec.Source.Image == nil {
			return nil, fmt.Errorf("bundledeployment.spec.source.image must be set for source type \"image\"")
		}
	case rukpakv1alpha2.SourceTypeGit:
		if bundleDeployment.Spec.Source.Git == nil {
			return nil, fmt.Errorf("bundledeployment.spec.source.git must be set for source type \"git\"")
		}
		if strings.HasPrefix(filepath.Clean(bundleDeployment.Spec.Source.Git.Directory), "../") {
			return nil, fmt.Errorf(`bundledeployment.spec.source.git.directory begins with "../": directory must define path within the repository`)
		}
	case rukpakv1alpha2.SourceTypeConfigMaps:
		if len(bundleDeployment.Spec.Source.ConfigMaps) == 0 {
			return nil, fmt.Errorf(`bundledeployment.spec.source.configmaps must be set for source type "configmaps"`)
		}
		errs := []error{}
		for i, cmSource := range bundleDeployment.Spec.Source.ConfigMaps {
			if strings.HasPrefix(filepath.Clean(cmSource.Path), ".."+string(filepath.Separator)) {
				errs = append(errs, fmt.Errorf("bundledeployment.spec.source.configmaps[%d].path is invalid: %q is outside bundle root", i, cmSource.Path))
			}
			if err := b.verifyConfigMapImmutable(ctx, cmSource.ConfigMap.Name); err != nil {
				errs = append(errs, fmt.Errorf("bundledeployment.spec.source.configmaps[%d].configmap.name is invalid: %v", i, err))
			}
		}
		if len(errs) > 0 {
			return nil, utilerrors.NewAggregate(errs)
		}
	}
	return nil, nil
}

func (b *BundleDeployment) verifyConfigMapImmutable(ctx context.Context, configMapName string) error {
	var cm corev1.ConfigMap
	err := b.Client.Get(ctx, client.ObjectKey{Namespace: b.SystemNamespace, Name: configMapName}, &cm)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	if cm.Immutable == nil || !*cm.Immutable {
		return fmt.Errorf("configmap %q is not immutable", configMapName)
	}
	return nil
}

func (b *BundleDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-core-rukpak-io-v1alpha2-bundledeployment", admission.WithCustomValidator(mgr.GetScheme(), &rukpakv1alpha2.BundleDeployment{}, b).WithRecoverPanic(true))
	return nil
}

var _ webhook.CustomValidator = &BundleDeployment{}
