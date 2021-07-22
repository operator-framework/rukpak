package provisioner

import (
	"context"
	"fmt"

	"github.com/operator-framework/rukpak/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) ReconcileProvisionerClass(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.V(1).Info("reconciling provisionerclass")

	pc := &v1alpha1.ProvisionerClass{}
	if err := r.Client.Get(ctx, req.NamespacedName, pc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	bundles := &v1alpha1.BundleList{}
	if err := r.Client.List(ctx, bundles, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "failed to list all the bundles in the req.Namespace")
		return ctrl.Result{}, err
	}

	filtered := []*v1alpha1.Bundle{}
	for _, b := range bundles.Items {
		log.Info("provisioner", "processing bundle name", b.Name)
		if string(b.Spec.Class) != pc.Name {
			log.Info("provisioner", "found bundle name that does not reference the current provisioner class", b.Name)
			continue
		}
		filtered = append(filtered, &b)
	}
	if len(filtered) == 0 {
		log.Info("no bundles found specifying the current provisionerclass")
		return ctrl.Result{}, nil
	}

	var errors []error
	for _, bundle := range filtered {
		log.Info("provisioner", "found bundle name that references the current provisionerclass", bundle.Name)
		if err := r.ensureBundleVolume(ctx, bundle.GetName()); err != nil {
			errors = append(errors, err)
		}
	}

	return ctrl.Result{}, utilerrors.NewAggregate(errors)
}

// ensureBundleVolume is a helper method responsible for ensuring the bundle's status
// references an existing volume that will the content stored in the bundle sources'
// filesystem.
func (r *Reconciler) ensureBundleVolume(ctx context.Context, bundleName string) error {
	fresh := &v1alpha1.Bundle{}
	if err := r.Client.Get(ctx, getNonNamespacedName(bundleName), fresh); err != nil {
		return err
	}

	if fresh.Status.Volume != nil {
		// TODO(tflannag): Check the status is still valid
		// TODO(tlfannag): Handle case where the object was just updated and requeued
		return nil
	}

	// TODO(tflannag): Avoid hardcoding -- likely can find the PVC by label selecting
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: "manifests", Namespace: r.globalNamespace}, pvc); err != nil {
		return client.IgnoreNotFound(err)
	}
	fresh.Status.Volume = &corev1.LocalObjectReference{
		Name: pvc.GetName(),
	}

	r.log.Info("volume", "attempting to update the bundle volume", fresh.GetName(), "with pvc name", pvc.GetName())
	if err := r.Client.Status().Update(ctx, fresh); err != nil {
		return err
	}
	r.log.Info("volume", "bundle status has been updated to point to a volume created", fresh.GetName())

	return nil
}

// TODO(tflannag): This logic should be abstracted further away as a newVolume
// type operation.
// TODO(tflannag): Need a way to avoid run-once operations that cannot be reconciled
// when state changes, e.g. avoid an InstallPlan-like scenario.
// TODO(tflannag): Avoid hardcoding the storage requests.
func (r *Reconciler) createPVC(bundleName string) (*corev1.PersistentVolumeClaim, error) {
	pvcName := fmt.Sprintf("%s-pvc", bundleName)
	pvc := &corev1.PersistentVolumeClaim{}
	ctx := context.Background()

	nn := types.NamespacedName{Name: pvcName, Namespace: r.globalNamespace}
	if err := r.Client.Get(ctx, nn, pvc); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		pvc.SetName(pvcName)
		pvc.SetNamespace(r.globalNamespace)
		volumeMode := corev1.PersistentVolumeFilesystem

		pvc.Spec = corev1.PersistentVolumeClaimSpec{
			VolumeMode:  &volumeMode,
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: apiresource.MustParse("2Gi"),
				},
			},
		}
		if err := r.Client.Create(ctx, pvc); err != nil {
			return nil, err
		}
	}

	return pvc, nil
}
