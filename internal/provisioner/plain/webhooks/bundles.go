package webhooks

import (
	"context"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

func NewValidatingBundleWebhook(mgr manager.Manager) (*webhook.Admission, error) {
	decoder, err := admission.NewDecoder(mgr.GetScheme())
	if err != nil {
		return nil, err
	}
	return &webhook.Admission{
		Handler: admission.HandlerFunc(func(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
			switch req.Operation {
			case admissionv1.Update:
				oldBundle := &rukpakv1alpha1.Bundle{}
				if err := decoder.DecodeRaw(req.OldObject, oldBundle); err != nil {
					return webhook.Denied(err.Error())
				}
				newBundle := &rukpakv1alpha1.Bundle{}
				if err := decoder.DecodeRaw(req.Object, newBundle); err != nil {
					return webhook.Denied(err.Error())
				}
				if !equality.Semantic.DeepEqual(oldBundle.Spec, newBundle.Spec) {
					return webhook.Denied("bundle.spec is immutable")
				}
				if oldBundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked && !equality.Semantic.DeepEqual(oldBundle.Status, newBundle.Status) {
					return webhook.Denied("bundle.status is immutable when bundle.status.phase==Unpacked")
				}
				return webhook.Allowed("")
			}
			return webhook.Allowed("")
		}),
	}, nil
}
