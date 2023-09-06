package bundledeployment

import (
	"github.com/operator-framework/rukpak/api/v1alpha2"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setUnpackStatusPending sets the resolved status condition to success.
func setUnpackStatusPending(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonUnpacking,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setUnpackStatusFailing sets the resolved status condition to success.
func setUnpackStatusFailing(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonUnpackFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setUnpackStatusSuccess sets the resolved status condition to success.
func setUnpackStatusSuccess(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha2.ReasonUnpackSuccessful,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setValidatePending sets the resolved status condition to success.
func setValidatePending(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonValidating,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setValidateFailing sets the resolved status condition to success.
func setValidateFailing(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonValidateFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setValidateSuccess sets the resolved status condition to success.
func setValidateSuccess(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha2.ReasonValidateSuccessful,
		Message:            message,
		ObservedGeneration: generation,
	})
}

func setDynamicWatchFailed(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonCreateDynamicWatchFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}
