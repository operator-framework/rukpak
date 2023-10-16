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

package bundledeployment

import (
	"github.com/operator-framework/rukpak/api/v1alpha2"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// setUnpackStatusFailing sets the unpack status condition to failing.
func setUnpackStatusFailing(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonUnpackFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setUnpackStatusPending sets the unpack status condition to pending.
func setUnpackStatusPending(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonUnpackPending,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setUnpackStatusSuccessful sets the unpack status condition to success.
func setUnpackStatusSuccessful(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeUnpacked,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha2.ReasonUnpackSuccessful,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setValidateStatusFailing sets the validate status condition to failing.
func setValidateStatusFailing(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonValidateFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setValidateStatusSuccess sets the validate status condition to success.
func setValidateStatusSuccess(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha2.ReasonValidateSuccessful,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setInstallStatusFailed sets the installed success to failing.
func setInstallStatusFailed(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonInstallFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setUnpgradeStatusFailed sets the installed success to failing as there is an error while patching
// objects on cluster.
func setUnpgradeStatusFailed(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonUpgradeFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setReconcileStatusFailed sets the installed success to failing as there is an error while reconciling
// existing objects on cluster.
func setReconcileStatusFailed(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonReconcileFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setInstallStatusSuccess sets the installed success to success.
func setInstallStatusSuccess(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha2.ReasonInstallSucceeded,
		Message:            message,
		ObservedGeneration: generation,
	})
}

// setDynamicWatchFailed sets the installed status to failing with the appropriate reason.
// This status appears when there is an error while fetching the applied objects from cluster
// after the deployer has returned so as to set watches on them.
func setDynamicWatchFailed(conditions *[]metav1.Condition, message string, generation int64) {
	apimeta.SetStatusCondition(conditions, metav1.Condition{
		Type:               v1alpha2.TypeInstalled,
		Status:             metav1.ConditionFalse,
		Reason:             v1alpha2.ReasonCreateDynamicWatchFailed,
		Message:            message,
		ObservedGeneration: generation,
	})
}
