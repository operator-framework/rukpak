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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/nlepage/go-tarfs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/updater"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	bundleContainerName        = "bundle"
	plainBundleProvisionerName = "plain"
)

// BundleReconciler reconciles a Bundle object
type BundleReconciler struct {
	client.Client
	KubeClient kubernetes.Interface
	Scheme     *runtime.Scheme
	Storage    storage.Storage

	HTTPClient *http.Client

	PodNamespace    string
	UnpackImage     string
	CopyBundleImage string
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods;secrets;configmaps,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Bundle object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
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

	pod := &corev1.Pod{}
	if op, err := r.ensureUnpackPod(ctx, bundle, pod); err != nil {
		u.UpdateStatus(updater.SetBundleInfo(nil), updater.EnsureBundleDigest(""))
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
		return ctrl.Result{}, r.handleRunningPod(ctx, &u, bundle, pod)
	case corev1.PodFailed:
		return ctrl.Result{}, r.handleFailedPod(ctx, &u, pod)
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
		updater.SetBundleInfo(nil),
		updater.EnsureBundleDigest(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackPending,
			Message: strings.Join(messages, "; "),
		}),
	)
}

func (r *BundleReconciler) handleFailedPod(ctx context.Context, u *updater.Updater, pod *corev1.Pod) error {
	u.UpdateStatus(
		updater.SetBundleInfo(nil),
		updater.EnsureBundleDigest(""),
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
			"core.rukpak.io/owner-kind": bundle.Kind,
			"core.rukpak.io/owner-name": bundle.Name,
		})
		pod.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})
		pod.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
		pod.Spec.RestartPolicy = corev1.RestartPolicyNever
		if len(pod.Spec.InitContainers) != 1 {
			pod.Spec.InitContainers = make([]corev1.Container, 1)
		}
		pod.Spec.InitContainers[0].Name = "install-cpb"
		pod.Spec.InitContainers[0].Image = r.UnpackImage
		pod.Spec.InitContainers[0].ImagePullPolicy = corev1.PullIfNotPresent
		pod.Spec.InitContainers[0].Command = []string{"cp", "-Rv", "/unpack", "/util/unpack"}
		pod.Spec.InitContainers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/util"}}

		if len(pod.Spec.Containers) != 1 {
			pod.Spec.Containers = make([]corev1.Container, 1)
		}
		pod.Spec.Containers[0].Name = bundleContainerName
		pod.Spec.Containers[0].Image = bundle.Spec.Image
		pod.Spec.Containers[0].ImagePullPolicy = corev1.PullAlways
		pod.Spec.Containers[0].Command = []string{"/util/unpack", "--bundle-dir=/manifests", "--listen-addr=:8080"}
		pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/util"}}

		if len(pod.Spec.Volumes) != 1 {
			pod.Spec.Volumes = make([]corev1.Volume, 1)
		}

		pod.Spec.Volumes[0] = corev1.Volume{Name: "util", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
		return nil
	})
}

func updateStatusUnpackPending(u *updater.Updater) {
	u.UpdateStatus(
		updater.SetBundleInfo(nil),
		updater.EnsureBundleDigest(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:   rukpakv1alpha1.TypeUnpacked,
			Status: metav1.ConditionFalse,
			Reason: rukpakv1alpha1.ReasonUnpackPending,
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

func (r *BundleReconciler) handleRunningPod(ctx context.Context, u *updater.Updater, bundle *rukpakv1alpha1.Bundle, pod *corev1.Pod) error {
	bundleFS, err := r.getBundleContents(ctx, pod)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get bundle contents: %w", err))
	}

	bundleImageDigest, err := r.getBundleImageDigest(pod)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get bundle image digest: %w", err))
	}

	objects, err := getObjects(bundleFS)
	if err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("get objects from bundle manifests: %w", err))
	}

	if err := r.Storage.Store(ctx, bundle, objects); err != nil {
		return updateStatusUnpackFailing(u, fmt.Errorf("persist bundle objects: %w", err))
	}

	info := &rukpakv1alpha1.BundleInfo{}
	for _, obj := range objects {
		gvk := obj.GetObjectKind().GroupVersionKind()
		info.Objects = append(info.Objects, rukpakv1alpha1.BundleObject{
			Group:     gvk.Group,
			Version:   gvk.Version,
			Kind:      gvk.Kind,
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		})
	}

	u.UpdateStatus(
		updater.SetBundleInfo(info),
		updater.EnsureBundleDigest(bundleImageDigest),
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
	url := fmt.Sprintf("http://%s:8080/content", pod.Status.PodIP)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create get request for %q: %v", url, pod)
	}
	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %q: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http get %q: unexpected response status: %s", url, resp.Status)
	}
	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gzip response from %q: %w", url, err)
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
		if ps.Name == bundleContainerName && ps.ImageID != "" {
			return ps.ImageID, nil
		}
	}
	return "", fmt.Errorf("bundle image digest not found")
}

func getObjects(bundleFS fs.FS) ([]client.Object, error) {
	var objects []client.Object
	const manifestsDir = "."

	entries, err := fs.ReadDir(bundleFS, manifestsDir)
	if err != nil {
		return nil, fmt.Errorf("read manifests: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
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
				return nil, err
			}
			objects = append(objects, &obj)
		}
	}
	return objects, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.HTTPClient == nil {
		r.HTTPClient = http.DefaultClient
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			util.BundleProvisionerFilter(plainBundleProvisionerID),
		)).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
