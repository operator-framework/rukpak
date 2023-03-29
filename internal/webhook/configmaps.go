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
	cl client.Client
}

func (w *ConfigMap) ValidateCreate(_ context.Context, _ runtime.Object) error {
	return nil
}

func (w *ConfigMap) ValidateUpdate(_ context.Context, _, _ runtime.Object) error {
	return nil
}

func (w *ConfigMap) ValidateDelete(ctx context.Context, obj runtime.Object) error {
	cm := obj.(*corev1.ConfigMap)

	bundleList := &rukpakv1alpha1.BundleList{}
	if err := w.cl.List(ctx, bundleList); err != nil {
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
	w.cl = mgr.GetClient()
	mgr.GetWebhookServer().Register("/validate-core-v1-configmap", admission.WithCustomValidator(&corev1.ConfigMap{}, w).WithRecoverPanic(true))
	return nil
}

var _ webhook.CustomValidator = &ConfigMap{}
