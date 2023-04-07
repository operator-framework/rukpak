package webhook

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch
//+kubebuilder:webhook:path=/validate-core-v1-configmap,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=configmaps,verbs=create;delete,versions=v1,name=vconfigmaps.core.rukpak.io,admissionReviewVersions=v1

type ConfigMap struct {
	Client client.Client
}

func (w *ConfigMap) ValidateCreate(ctx context.Context, obj runtime.Object) error {
	// Only allow configmap to be created if either of the following is true:
	//   1. The configmap is immutable.
	//   2. The configmap is not referenced by a bundle.

	cm := obj.(*corev1.ConfigMap)
	if cm.Immutable != nil && *cm.Immutable {
		return nil
	}

	bundleList := &rukpakv1alpha1.BundleList{}
	if err := w.Client.List(ctx, bundleList); err != nil {
		return err
	}
	bundleReferrers := []string{}
	for _, bundle := range bundleList.Items {
		if bundle.Spec.Source.Type == rukpakv1alpha1.SourceTypeConfigMaps {
			for _, bundleConfigMapRef := range bundle.Spec.Source.ConfigMaps {
				if bundleConfigMapRef.ConfigMap.Name == cm.Name {
					bundleReferrers = append(bundleReferrers, bundle.Name)
				}
			}
		}
	}
	if len(bundleReferrers) > 0 {
		return fmt.Errorf("configmap %q is referenced in .spec.source.configMaps[].configMap.name by bundles %v; referenced configmaps must have .immutable == true", cm.Name, bundleReferrers)
	}
	return nil
}

func (w *ConfigMap) ValidateUpdate(_ context.Context, _, _ runtime.Object) error {
	return nil
}

func (w *ConfigMap) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	cm := obj.(*corev1.ConfigMap)

	bundleList := &rukpakv1alpha1.BundleList{}
	if err := w.Client.List(ctx, bundleList); err != nil {
		return err
	}
	for _, b := range bundleList.Items {
		for _, cmSource := range b.Spec.Source.ConfigMaps {
			if cmSource.ConfigMap.Name == cm.Name {
				return fmt.Errorf("configmap %q is in-use by bundle %q", cm.Name, b.Name)
			}
		}
	}
	return nil
}

func (w *ConfigMap) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-core-v1-configmap", admission.WithCustomValidator(&corev1.ConfigMap{}, w).WithRecoverPanic(true))
	return nil
}

var _ webhook.CustomValidator = &ConfigMap{}
