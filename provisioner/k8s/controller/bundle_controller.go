package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/provisioner/k8s/controller/manifests"
)

const (
	defaultStorageNamespace = "rukpak"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=get;list;watch;create;update;patch

type bundleController struct {
	client.Client
	log logr.Logger

	unpackImage string
}

func (b *bundleController) manageWith(mgr ctrl.Manager) error {
	// Create multiple controllers for resource types that require automatic adoption
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to Bundles that reference a ProvisionerClass that references ProvisionerID
		For(&v1alpha1.Bundle{}).
		Complete(b)
}

func (b *bundleController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log := b.log.WithValues("request", req)
	log.Info("reconciling bundle")
	defer log.Info("finished reconciling bundle")

	// TODO(tflannag): Need to work towards building up better requeue behavior
	in := &v1alpha1.Bundle{}
	if err := b.Get(ctx, req.NamespacedName, in); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if err := b.ensureBundleVolume(ctx, in); err != nil {
		log.Error(err, "failed ensuring that the bundle has a volume created")
		return ctrl.Result{}, err
	}
	if err := b.ensureBundleUnpack(ctx, in); err != nil {
		log.Error(err, "failed to unpack bundle contents")
		return ctrl.Result{}, err
	}
	if err := b.ensureBundleContents(ctx, in, b.byteHandler(ctx, in)); err != nil {
		log.Error(err, "failed to successfully create bundle contents")
		return ctrl.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (b *bundleController) ensureBundleVolume(ctx context.Context, bundle *v1alpha1.Bundle) error {
	log := b.log.WithValues("volume handler", bundle.GetName())
	// TODO: stop gap for now but still need to verify this state is correct
	// TODO: only alter state at the call site (when updating status)?
	if bundle.Status.Volume != nil {
		return nil
	}

	cm, err := b.getBundleStorage(ctx, bundle)
	if apierrors.IsNotFound(err) {
		cm.SetName(bundle.GetName())
		cm.SetNamespace(defaultStorageNamespace)
		if err := controllerutil.SetOwnerReference(bundle, cm, b.Scheme()); err != nil {
			log.Error(err, "failed to set configmap owner reference")
		}
		if err := b.Client.Create(ctx, cm); err != nil {
			log.Error(err, "failed to create bundle configmap", "bundle name", bundle.GetName())
			return err
		}
	}

	log.Info("configmap has been created for bundle")
	bundle.SetVolumeRef(&corev1.LocalObjectReference{Name: cm.GetName()})
	bundle.SetUnpackedStatus(v1alpha1.BundleNeedsUnpacking)
	if err := b.Client.Status().Update(ctx, bundle); err != nil {
		log.Error(err, "failed to update status")
		return err
	}
	return nil
}

func (b *bundleController) ensureBundleUnpack(ctx context.Context, bundle *v1alpha1.Bundle) error {
	log := b.log.WithValues("unpacker", bundle.GetName())
	// TODO(tflannag): Stop gap for now but still need to verify this state
	if bundle.Status.Unpacked == v1alpha1.BundleUnpacked {
		return nil
	}

	log.Info("attempting to unpack bundle contents")
	job := &batchv1.Job{}
	jobName := fmt.Sprintf("%s-bundle-unpack", bundle.GetName())

	err := b.Client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: defaultStorageNamespace}, job)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "failed to query for the bundle unpack job")
		bundle.SetUnpackedStatus(v1alpha1.BundleNeedsUnpacking)
		return b.Client.Status().Update(ctx, bundle)
	}
	if apierrors.IsNotFound(err) {
		log.Info("creating bundle unpack job")
		config := manifests.BundleUnpackJobConfig{
			JobName:      jobName,
			JobNamespace: defaultStorageNamespace,
			BundleName:   bundle.GetName(),
			UnpackImage:  b.unpackImage,
		}
		job, err := manifests.NewJobManifest(config)
		if err != nil {
			log.Error(err, "failed to create a new job manifest")
			return err
		}
		if err := controllerutil.SetOwnerReference(bundle, job, b.Scheme()); err != nil {
			log.Error(err, "failed to set owner reference on job resource")
			return err
		}
		if err := b.Client.Create(ctx, job); err != nil {
			log.Error(err, "failed to create bundle unpack job")
			return err
		}
		return nil
	}

	// TODO(tflannag): Clean this up - make a unpackReadyHandler function?
	jobIsReady := func(job *batchv1.Job) bool {
		// TODO(tflannag): These likely aren't sufficient enough checks, but will do the trick
		// for the most part in the current implementation.
		if job.Status.Failed != 0 {
			return false
		}
		if job.Status.Succeeded == 0 {
			return false
		}
		return true
	}
	if !jobIsReady(job) {
		log.Info("bundle unpack job not yet ready", "job name", job.GetName())
		return nil
	}

	log.Info("bundle unpacked successfully updating status", "name", bundle.GetName())
	bundle.SetUnpackedStatus(v1alpha1.BundleUnpacked)
	if err := b.Client.Status().Update(ctx, bundle); err != nil {
		log.Error(err, "failed to update status")
		return err
	}

	return nil
}

type handlerFn func(file string, data []byte) error

// TODO(tflannag): Cleanup this name/implementation/etc.
func (b *bundleController) byteHandler(ctx context.Context, bundle *v1alpha1.Bundle) handlerFn {
	return func(file string, data []byte) error {
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(data, u); err != nil {
			return err
		}
		err := b.Client.Get(ctx, client.ObjectKeyFromObject(u), u)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if apierrors.IsNotFound(err) {
			if err := controllerutil.SetOwnerReference(bundle, u, b.Scheme()); err != nil {
				return err
			}
			return b.Client.Create(ctx, u)
		}
		return nil
	}
}

func (b *bundleController) getBundleStorage(ctx context.Context, bundle *v1alpha1.Bundle) (*corev1.ConfigMap, error) {
	nn := types.NamespacedName{
		Name:      bundle.Status.Volume.Name,
		Namespace: defaultStorageNamespace,
	}
	cm := &corev1.ConfigMap{}
	if err := b.Client.Get(ctx, nn, cm); err != nil {
		return nil, err
	}

	return cm, nil
}

func (b *bundleController) ensureBundleContents(ctx context.Context, bundle *v1alpha1.Bundle, handler handlerFn) error {
	log := b.log.WithValues("bundle", bundle.GetName(), "source", bundle.Spec.Source)

	log.Info("creating bundle contents")
	cm, err := b.getBundleStorage(ctx, bundle)
	if apierrors.IsNotFound(err) {
		log.Error(err, "bundle volume does not exist")
		bundle.SetUnpackedStatus(v1alpha1.BundleNeedsUnpacking)
		bundle.SetVolumeRef(nil)
		return b.Client.Status().Update(ctx, bundle)
	}
	if len(cm.Data) == 0 {
		log.Error(err, "configmap data is empty")
		bundle.SetUnpackedStatus(v1alpha1.BundleNeedsUnpacking)
		return b.Client.Status().Update(ctx, bundle)
	}
	numRetries := 3
	resources := cm.Data

	for numRetries > 0 {
		for filename, data := range resources {
			log.Info("processing resource", "filename", filename)
			if err := handler(filename, []byte(data)); err != nil {
				log.Error(err, "failed to create or update resource")
				continue
			}
			delete(resources, filename)
			bundle.Status.Contents = append(bundle.Status.Contents, v1alpha1.Content(filename))
		}
		numRetries--
	}

	log.Info("bundle contents have been created")
	return b.Client.Status().Update(ctx, bundle)
}
