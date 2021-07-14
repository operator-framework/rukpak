package provisioner

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/k8s/manifests"
)

var (
	localSchemeBuilder = runtime.NewSchemeBuilder(
		kscheme.AddToScheme,
		v1alpha1.AddToScheme,
	)
	// AddToScheme adds all types necessary for the controller to operate.
	AddToScheme = localSchemeBuilder.AddToScheme
)

const (
	// ID is the rukpak provisioner's unique ID. Only ProvisionerClass(es) that specify
	// this unique ID will be managed by this provisioner controller.
	ID v1alpha1.ProvisionerID = "rukpack.io/k8s"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=provisionerclasses,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch

// SetupWithManager adds the operator reconciler to the given controller manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Bundle{}).
		Complete(reconcile.Func(r.ReconcileBundle))
	if err != nil {
		return err
	}

	predicateProvisionerIDFilter := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pc, ok := obj.(*v1alpha1.ProvisionerClass)
		if !ok {
			return false
		}
		return pc.Spec.Provisioner == ID
	})
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProvisionerClass{}, builder.WithPredicates(predicateProvisionerIDFilter)).
		Watches(&source.Kind{Type: &v1alpha1.Bundle{}}, handler.EnqueueRequestsFromMapFunc(r.bundleHandler)).
		Complete(reconcile.Func(r.ReconcileProvisionerClass))
	if err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) bundleHandler(obj client.Object) []reconcile.Request {
	log := r.log.WithValues("bundle", obj.GetName())

	bundle := &v1alpha1.Bundle{}
	if err := r.Client.Get(context.TODO(), getNonNamespacedName(obj.GetName()), bundle); err != nil {
		return []reconcile.Request{}
	}

	provisioners := &v1alpha1.ProvisionerClassList{}
	if err := r.Client.List(context.TODO(), provisioners); err != nil {
		return []reconcile.Request{}
	}
	if len(provisioners.Items) == 0 {
		return []reconcile.Request{}
	}

	res := []reconcile.Request{}
	for _, provisioner := range provisioners.Items {
		if provisioner.GetName() != string(bundle.Spec.Class) {
			continue
		}
		res = append(res, reconcile.Request{NamespacedName: getNonNamespacedName(provisioner.GetName())})
	}
	if len(res) == 0 {
		log.Info("no provisionerclass(es) need to be requeued after encountering a bundle event")
		return []reconcile.Request{}
	}

	log.Info("handler", "requeueing provisionerclass(es) after encountering a bundle event", obj.GetName())
	return res
}

func (r *Reconciler) ReconcileBundle(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.V(1).Info("reconciling bundle")

	bundle := &v1alpha1.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, bundle); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO(tflannag): Remove this filtering once we support other installation methods
	// besides remote content.
	bundleType := bundle.Spec.Source.Ref
	if !strings.Contains(bundleType, "docker") {
		log.Info("bundle", "non-docker bundle types are not supported yet", bundleType)
		return ctrl.Result{}, nil
	}
	if bundle.Status.Volume == nil {
		log.Info("bundle", "waiting for the provisioner to create a volume for the bundle", bundle.Name)
		return ctrl.Result{}, nil
	}

	if !strings.Contains(string(bundle.Spec.Source.Ref), "docker://") {
		log.Info("bundle", "cannot process non-docker bundle sources", bundle.Name, "source", bundle.Spec.Class)
		return ctrl.Result{}, nil
	}

	// TODO(tflannag): Investigate partitioning the filesystem content better so it's namespaced
	// at least by bundle name to avoid blindly overwriting manifest content.
	// TODO(tflannag): Need a way to rotate the serving Pod when changes have been made
	// to the underlying sub-directory filesystem.
	if err := r.unpackBundle(bundle); err != nil {
		log.Error(err, "failed to unpack bunde")
		return ctrl.Result{}, err
	}
	if err := r.ensureManifestService(8081, bundle); err != nil {
		log.Error(err, "failed to ensure the manifest serving service exists")
		return ctrl.Result{}, err
	}
	// TODO(tflannag): Need to wait until the job that's unpacking contents has become
	// ready before serving filesystem content
	if err := r.createManifestServingPod("/manifests", bundle); err != nil {
		log.Error(err, "failed to create a manifest filesystem serving pod")
		return ctrl.Result{}, err
	}
	// TODO(tflannag): Handle updating status

	return reconcile.Result{}, nil
}

func (r *Reconciler) ensureManifestService(port int32, bundle *v1alpha1.Bundle) error {
	r.log.Info("serve", "ensuring a service exists for serving bundle manifest content", bundle.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", bundle.Name),
			Namespace: r.globalNamespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name: "serving",
				Port: port,
			}},
			Selector: map[string]string{
				// TODO(tflannag): Use a better label in the Pod manifest template
				"name": bundle.Name,
			},
		},
	}
	if err := controllerutil.SetOwnerReference(bundle, service, r.Scheme()); err != nil {
		return err
	}
	if err := r.Client.Create(context.Background(), service); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	// TODO: reconcile state as needed

	return nil
}

func (r *Reconciler) createManifestServingPod(mountpath string, bundle *v1alpha1.Bundle) error {
	pvcName := bundle.Status.Volume.Name
	r.log.Info("serve", "creating a pod that serves the manifest bundle content", bundle.Name, "using pvc name", pvcName, "and mountpath", mountpath)
	// TODO(tflannag): Use a better PodName here.
	config := manifests.ManifestServingPod{
		PodName:      fmt.Sprintf("%s-serve", bundle.Name),
		PodNamespace: r.globalNamespace,
		ServeImage:   r.serveImage,
		PVCName:      pvcName,
		PVCMountPath: mountpath,
	}

	pod, err := manifests.NewManifestServingPod(config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetOwnerReference(bundle, pod, r.Scheme()); err != nil {
		return err
	}
	if err := r.Client.Create(context.Background(), pod); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	return nil
}

func (r *Reconciler) unpackBundle(bundle *v1alpha1.Bundle) error {
	bundleSource := bundle.Spec.Source.Ref
	if strings.Contains(bundleSource, "docker://") {
		bundleSource = strings.TrimPrefix(bundleSource, "docker://")
	}
	// TODO(tflannag): Investigate whether we need a reloading sidecar container
	// when the underlying filesystem has been updated.
	if err := r.newUnpackJob(bundleSource, bundle); err != nil {
		return err
	}

	return nil
}

func (r *Reconciler) newUnpackJob(image string, bundle *v1alpha1.Bundle) error {
	r.log.Info("creating job", "job namespace", r.globalNamespace, "job unpack image", r.unpackImage)

	// setup owner references
	// setup ttl delete
	jobName := fmt.Sprintf("%s-bundle-job", bundle.Name)
	config := manifests.BundleUnpackJobConfig{
		JobName:      jobName,
		JobNamespace: r.globalNamespace,
		UnpackImage:  r.unpackImage,
		BundleImage:  image,
		PVCName:      bundle.Status.Volume.Name,
	}
	job, err := manifests.NewJobManifest(config)
	if err != nil {
		return err
	}
	if err := controllerutil.SetOwnerReference(bundle, job, r.Scheme()); err != nil {
		return err
	}
	if err := r.Client.Create(context.Background(), job); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	return nil
}
