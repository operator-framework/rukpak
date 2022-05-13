/*
Copyright 2021.

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

package controllers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/nlepage/go-tarfs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/git"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/updater"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	bundleUnpackContainerName  = "bundle"
	plainBundleProvisionerName = "plain"
)

// BundleReconciler reconciles a Bundle object
type BundleReconciler struct {
	client.Client
	KubeClient kubernetes.Interface
	Scheme     *runtime.Scheme
	Storage    storage.Storage
	Finalizers finalizer.Finalizers

	PodNamespace    string
	UnpackImage     string
	CopyBundleImage string
	GitClientImage  string
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *BundleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reconcileErr error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")
	bundle := &rukpakv1alpha1.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, bundle); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	u := updater.New(r.Client)
	defer func() {
		if err := u.Apply(ctx, bundle); err != nil {
			l.Error(err, "failed to update status")
		}
	}()
	u.UpdateStatus(updater.EnsureObservedGeneration(bundle.Generation))

	res, err := r.Finalizers.Finalize(ctx, bundle)
	if err != nil {
		u.UpdateStatus(
			updater.EnsureBundleDigest(""),
			updater.EnsureContentURL(""),
			updater.SetPhase(rukpakv1alpha1.PhaseFailing),
			updater.EnsureCondition(metav1.Condition{
				Type:    rukpakv1alpha1.TypeUnpacked,
				Status:  metav1.ConditionUnknown,
				Reason:  rukpakv1alpha1.ReasonProcessingFinalizerFailed,
				Message: err.Error(),
			}),
		)
		return ctrl.Result{}, err
	}
	var (
		finalizerUpdateErrs []error
	)
	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if res.StatusUpdated {
		finalizerUpdateErrs = append(finalizerUpdateErrs, r.Status().Update(ctx, bundle))
	}
	if res.Updated {
		finalizerUpdateErrs = append(finalizerUpdateErrs, r.Update(ctx, bundle))
	}
	if res.Updated || res.StatusUpdated || !bundle.GetDeletionTimestamp().IsZero() {
		err := apimacherrors.NewAggregate(finalizerUpdateErrs)
		if err != nil {
			u.UpdateStatus(
				updater.EnsureBundleDigest(""),
				updater.EnsureContentURL(""),
				updater.SetPhase(rukpakv1alpha1.PhaseFailing),
				updater.EnsureCondition(metav1.Condition{
					Type:    rukpakv1alpha1.TypeUnpacked,
					Status:  metav1.ConditionUnknown,
					Reason:  rukpakv1alpha1.ReasonProcessingFinalizerFailed,
					Message: err.Error(),
				}),
			)
		}
		return ctrl.Result{}, err
	}

	pod := &corev1.Pod{}
	if op, err := r.ensureUnpackPod(ctx, bundle, pod); err != nil {
		u.UpdateStatus(
			updater.EnsureBundleDigest(""),
			updater.EnsureContentURL(""),
		)
		return ctrl.Result{}, updateStatusUnpackFailing(&u, fmt.Errorf("ensure unpack pod: %w", err))
	} else if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated || pod.DeletionTimestamp != nil {
		updateStatusUnpackPending(&u)
		return ctrl.Result{}, nil
	}

	switch phase := pod.Status.Phase; phase {
	case corev1.PodPending:
		r.handlePendingPod(&u, pod)
		return ctrl.Result{}, nil
	case corev1.PodRunning:
		r.handleRunningPod(&u)
		return ctrl.Result{}, nil
	case corev1.PodFailed:
		return ctrl.Result{}, r.handleFailedPod(ctx, &u, pod)
	case corev1.PodSucceeded:
		return ctrl.Result{}, r.handleCompletedPod(ctx, &u, bundle, pod)
	default:
		return ctrl.Result{}, r.handleUnexpectedPod(ctx, &u, pod)
	}
}

func (r *BundleReconciler) handleUnexpectedPod(ctx context.Context, u *updater.Updater, pod *corev1.Pod) error {
	err := fmt.Errorf("unexpected pod phase: %v", pod.Status.Phase)
	_ = r.Delete(ctx, pod)
	return updateStatusUnpackFailing(u, err)
}

func (r *BundleReconciler) handlePendingPod(u *updater.Updater, pod *corev1.Pod) {
	var messages []string
	for _, cStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
		if cStatus.State.Waiting != nil && cStatus.State.Waiting.Reason == "ErrImagePull" {
			messages = append(messages, cStatus.State.Waiting.Message)
		}
		if cStatus.State.Waiting != nil && cStatus.State.Waiting.Reason == "ImagePullBackOff" {
			messages = append(messages, cStatus.State.Waiting.Message)
		}
	}
	u.UpdateStatus(
		updater.EnsureBundleDigest(""),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackPending,
			Message: strings.Join(messages, "; "),
		}),
	)
}

func (r *BundleReconciler) handleRunningPod(u *updater.Updater) {
	u.UpdateStatus(
		updater.EnsureBundleDigest(""),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacking),
		updater.EnsureCondition(metav1.Condition{
			Type:   rukpakv1alpha1.TypeUnpacked,
			Status: metav1.ConditionFalse,
			Reason: rukpakv1alpha1.ReasonUnpacking,
		}),
	)
}

func (r *BundleReconciler) handleFailedPod(ctx context.Context, u *updater.Updater, pod *corev1.Pod) error {
	u.UpdateStatus(
		updater.EnsureBundleDigest(""),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseFailing),
	)
	logs, err := r.getPodLogs(ctx, pod)
	if err != nil {
		err = fmt.Errorf("unpack failed: failed to retrieve failed pod logs: %w", err)
		u.UpdateStatus(
			updater.EnsureCondition(metav1.Condition{
				Type:    rukpakv1alpha1.TypeUnpacked,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonUnpackFailed,
				Message: err.Error(),
			}),
		)
		return err
	}
	logStr := string(logs)
	u.UpdateStatus(
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackFailed,
			Message: logStr,
		}),
	)
	_ = r.Delete(ctx, pod)
	return fmt.Errorf("unpack failed: %v", logStr)
}

func (r *BundleReconciler) ensureUnpackPod(ctx context.Context, bundle *rukpakv1alpha1.Bundle, pod *corev1.Pod) (controllerutil.OperationResult, error) {
	controllerRef := metav1.NewControllerRef(bundle, bundle.GroupVersionKind())
	automountServiceAccountToken := false
	pod.SetName(util.PodName(plainBundleProvisionerName, bundle.Name))
	pod.SetNamespace(r.PodNamespace)

	return util.CreateOrRecreate(ctx, r.Client, pod, func() error {
		pod.SetLabels(map[string]string{
			util.CoreOwnerKindKey: bundle.Kind,
			util.CoreOwnerNameKey: bundle.Name,
		})
		pod.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})
		pod.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
		pod.Spec.RestartPolicy = corev1.RestartPolicyNever

		switch bundle.Spec.Source.Type {
		case rukpakv1alpha1.SourceTypeImage:
			pod = bundleImagePod(pod, *bundle.Spec.Source.Image, r.UnpackImage)
			return nil
		case rukpakv1alpha1.SourceTypeGit:
			var err error
			pod, err = bundleGitRepoPod(pod, *bundle.Spec.Source.Git, r.UnpackImage, r.GitClientImage)
			if err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("unsupported bundle source type %s", bundle.Spec.Source.Type)
		}
	})
}

func updateStatusUnpackPending(u *updater.Updater) {
	u.UpdateStatus(
		updater.EnsureBundleDigest(""),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackPending,
			Message: "Waiting for the bundle to be unpacked",
		}),
	)
}

func updateStatusUnpackFailing(u *updater.Updater, err error) error {
	u.UpdateStatus(
		updater.SetPhase(rukpakv1alpha1.PhaseFailing),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackFailed,
			Message: err.Error(),
		}),
	)
	return err
}

func (r *BundleReconciler) handleCompletedPod(ctx context.Context, u *updater.Updater, bundle *rukpakv1alpha1.Bundle, pod *corev1.Pod) error {
	bundleFS, err := r.getBundleContents(ctx, pod)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get bundle contents: %w", err))
	}

	// TODO: generalize for other content sources
	// See https://github.com/operator-framework/rukpak/issues/164
	bundleImageDigest, err := r.getBundleImageDigest(pod)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get bundle image digest: %w", err))
	}
	objects, err := getObjects(bundleFS)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get objects from bundle manifests: %w", err))
	}
	if len(objects) == 0 {
		return updateStatusUnpackFailing(u, errors.New("invalid bundle: found zero objects: "+
			"plain+v0 bundles are required to contain at least one object"))
	}

	if err := r.Storage.Store(ctx, bundle, bundleFS); err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("persist bundle objects: %w", err))
	}

	contentURL, err := r.Storage.URLFor(ctx, bundle)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get content URL: %w", err))
	}

	u.UpdateStatus(
		updater.EnsureBundleDigest(bundleImageDigest),
		updater.EnsureContentURL(contentURL),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacked),
		updater.EnsureCondition(metav1.Condition{
			Type:   rukpakv1alpha1.TypeUnpacked,
			Status: metav1.ConditionTrue,
			Reason: rukpakv1alpha1.ReasonUnpackSuccessful,
		}),
	)

	return nil
}

func (r *BundleReconciler) getBundleContents(ctx context.Context, pod *corev1.Pod) (fs.FS, error) {
	bundleData, err := r.getPodLogs(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle contents: %w", err)
	}
	bd := struct {
		Content []byte `json:"content"`
	}{}

	if err := json.Unmarshal(bundleData, &bd); err != nil {
		return nil, fmt.Errorf("parse bundle data: %w", err)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(bd.Content))
	if err != nil {
		return nil, fmt.Errorf("read bundle content gzip: %w", err)
	}
	return tarfs.New(gzr)
}

func (r *BundleReconciler) getPodLogs(ctx context.Context, pod *corev1.Pod) ([]byte, error) {
	logReader, err := r.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pod logs: %w", err)
	}
	defer logReader.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, logReader); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *BundleReconciler) getBundleImageDigest(pod *corev1.Pod) (string, error) {
	for _, ps := range pod.Status.ContainerStatuses {
		if ps.Name == bundleUnpackContainerName && ps.ImageID != "" {
			return ps.ImageID, nil
		}
	}
	return "", fmt.Errorf("bundle image digest not found")
}

func getObjects(bundleFS fs.FS) ([]client.Object, error) {
	var objects []client.Object
	const manifestsDir = "manifests"

	entries, err := fs.ReadDir(bundleFS, manifestsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, e.Name()))
		}
		fileData, err := fs.ReadFile(bundleFS, filepath.Join(manifestsDir, e.Name()))
		if err != nil {
			return nil, err
		}

		dec := apimachyaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileData), 1024)
		for {
			obj := unstructured.Unstructured{}
			err := dec.Decode(&obj)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read %q: %w", e.Name(), err)
			}
			objects = append(objects, &obj)
		}
	}
	return objects, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			util.BundleProvisionerFilter(plain.ProvisionerID),
		)).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

func bundleImagePod(pod *corev1.Pod, source rukpakv1alpha1.ImageSource, unpackImage string) *corev1.Pod {
	if len(pod.Spec.InitContainers) != 1 {
		pod.Spec.InitContainers = make([]corev1.Container, 1)
	}

	pod = addUnpackerInitContainer(pod, unpackImage)

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	pod.Spec.Containers[0].Name = bundleUnpackContainerName
	pod.Spec.Containers[0].Image = source.Ref
	pod.Spec.Containers[0].Command = []string{"/bin/unpack", "--bundle-dir", "/"}
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/bin"}}

	if source.ImagePullSecretName != "" {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: source.ImagePullSecretName}}
	}
	pod.Spec.Volumes = []corev1.Volume{
		{Name: "util", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	return pod
}

func bundleGitRepoPod(pod *corev1.Pod, source rukpakv1alpha1.GitSource, unpackImage, gitClientImage string) (*corev1.Pod, error) {
	if len(pod.Spec.InitContainers) != 2 {
		pod.Spec.InitContainers = make([]corev1.Container, 2)
	}

	pod = addUnpackerInitContainer(pod, unpackImage)

	// Note: initContainer so we can ensure the repository has been cloned
	// at the bundle.Spec.Source.Git.Ref before we unpack the Bundle contents
	// that are stored in the repository.
	pod.Spec.InitContainers[1].Name = "clone-repository"
	// r.GitClientImage configures which git-based container image to use to clone the provided repository
	// r.GitClientImage currently defaults to alpine/git:v2.32.0
	pod.Spec.InitContainers[1].Image = gitClientImage
	pod.Spec.InitContainers[1].ImagePullPolicy = corev1.PullIfNotPresent
	cmd, err := git.CloneCommandFor(source)
	if err != nil {
		return nil, err
	}
	pod.Spec.InitContainers[1].Command = []string{"/bin/sh", "-c", cmd}
	pod.Spec.InitContainers[1].VolumeMounts = []corev1.VolumeMount{{Name: "bundle", MountPath: "/bundle"}}

	if len(pod.Spec.Containers) != 1 {
		pod.Spec.Containers = make([]corev1.Container, 1)
	}

	pod.Spec.Containers[0].Name = bundleUnpackContainerName
	// Note: the image for this pod is not relevant, as it exists only to run the unpacker against the bundle directory.
	pod.Spec.Containers[0].Image = unpackImage
	pod.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
	pod.Spec.Containers[0].Command = []string{"/bin/unpack", "--bundle-dir", "/bundle"}
	pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/bin"}, {Name: "bundle", MountPath: "/bundle"}}

	pod.Spec.Volumes = []corev1.Volume{
		{Name: "util", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "bundle", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
	return pod, nil
}

// addUnpackerInitContainer injects the install-unpacker init container into the given pod.
// addUnpackerInitContainer assumes the pod has an array of init containers initialized.
func addUnpackerInitContainer(pod *corev1.Pod, unpackImage string) *corev1.Pod {
	pod.Spec.InitContainers[0].Name = "install-unpacker"
	pod.Spec.InitContainers[0].Image = unpackImage
	pod.Spec.InitContainers[0].ImagePullPolicy = corev1.PullIfNotPresent
	pod.Spec.InitContainers[0].Command = []string{"cp", "-Rv", "/unpack", "/bin/unpack"}
	pod.Spec.InitContainers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/bin"}}
	return pod
}
