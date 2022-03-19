package webhooks

import (
	"context"
	"fmt"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

func NewValidatingConfigMapWebhook(mgr manager.Manager) (*webhook.Admission, error) {
	cl := mgr.GetClient()
	decoder, err := admission.NewDecoder(mgr.GetScheme())
	if err != nil {
		return nil, err
	}
	return &webhook.Admission{
		Handler: admission.HandlerFunc(func(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
			switch req.Operation {
			case admissionv1.Update:
				oldCm := &corev1.ConfigMap{}
				if err := decoder.DecodeRaw(req.OldObject, oldCm); err != nil {
					return webhook.Denied(err.Error())
				}
				newCm := &corev1.ConfigMap{}
				if err := decoder.DecodeRaw(req.Object, newCm); err != nil {
					return webhook.Denied(err.Error())
				}
				oldCmLabels := keys(oldCm.Labels)
				newCmLabels := keys(newCm.Labels)
				dropped := oldCmLabels.Difference(newCmLabels)

				labels := []string{}
				for _, l := range dropped.List() {
					if strings.HasPrefix(l, "core.rukpak.io/") {
						labels = append(labels, l)
					}
				}
				if len(labels) > 0 {
					return webhook.Denied(fmt.Errorf("cannot delete labels %s", labels).Error())
				}
				return webhook.Allowed("")
			case admissionv1.Delete:
				cm := &corev1.ConfigMap{}
				if err := cl.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, cm); err != nil {
					return webhook.Denied(err.Error())
				}
				bundleName := cm.Labels["core.rukpak.io/owner-name"]
				bundle := &rukpakv1alpha1.Bundle{}
				err := cl.Get(ctx, types.NamespacedName{Name: bundleName}, bundle)
				if err == nil {
					return webhook.Denied(fmt.Sprintf("deletion denied when %s %q still exists", bundle.GroupVersionKind(), bundle.Name))
				}
				if !apierrors.IsNotFound(err) {
					return webhook.Denied(err.Error())
				}
			}
			return webhook.Allowed("")
		}),
	}, nil
}

func keys(m map[string]string) sets.String {
	s := sets.NewString()
	for k := range m {
		s.Insert(k)
	}
	return s
}
