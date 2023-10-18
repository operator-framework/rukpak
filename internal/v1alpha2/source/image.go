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

package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfigurationcorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	v1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/operator-framework/rukpak/internal/util"

	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
)

// Image implements unpack interface.
type image struct {
	Client       client.Client
	KubeClient   kubernetes.Interface
	PodNamespace string
	UnpackImage  string
}

const imageUnpackContainerName = "bundle"

type logfn func(ctx context.Context, pod *corev1.Pod) ([]byte, error)

func (i *image) Unpack(ctx context.Context, source v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error) {
	// Validate inputs
	if err := i.validate(&source, opts); err != nil {
		return nil, fmt.Errorf("validation unsuccessful during unpacking %v", err)
	}
	return i.unpack(ctx, source, store, opts)
}

func (i *image) unpack(ctx context.Context, source v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error) {
	pod := &corev1.Pod{}
	op, err := i.ensureUnpackPod(ctx, store.GetBundleDeploymentName(), source, pod, opts)
	if err != nil {
		return nil, err
	} else if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated || pod.DeletionTimestamp != nil {
		return &Result{State: StateUnpackPending}, nil
	}
	switch phase := pod.Status.Phase; phase {
	case corev1.PodPending:
		return pendingImagePodResult(pod), nil
	case corev1.PodRunning:
		return &Result{State: StateUnpacking}, nil
	case corev1.PodFailed:
		return nil, i.failedPodResult(ctx, pod)
	case corev1.PodSucceeded:
		return i.succeededPodResult(ctx, pod, source, store)
	default:
		return nil, i.handleUnexpectedPod(ctx, pod)
	}
}

func (i *image) ensureUnpackPod(ctx context.Context, bdName string, bundleSrc v1alpha2.BundleDeplopymentSource, pod *corev1.Pod, opts UnpackOption) (controllerutil.OperationResult, error) {
	// The name of the unpack pod is same as the bundle deployment. Since only one image source per bundle deployment
	// is acceptable, this would not be an issue.
	existingPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: i.PodNamespace, Name: bdName}}
	if err := i.Client.Get(ctx, client.ObjectKeyFromObject(existingPod), existingPod); client.IgnoreNotFound(err) != nil {
		return controllerutil.OperationResultNone, err
	}

	podApplyConfig := i.getDesiredPodApplyConfig(bdName, &bundleSrc, opts)
	updatedPod, err := i.KubeClient.CoreV1().Pods(i.PodNamespace).Apply(ctx, podApplyConfig, metav1.ApplyOptions{Force: true, FieldManager: "rukpak-core"})
	if err != nil {
		if !apierrors.IsInvalid(err) {
			return controllerutil.OperationResultNone, err
		}
		if err := i.Client.Delete(ctx, existingPod); err != nil {
			return controllerutil.OperationResultNone, err
		}
		updatedPod, err = i.KubeClient.CoreV1().Pods(i.PodNamespace).Apply(ctx, podApplyConfig, metav1.ApplyOptions{Force: true, FieldManager: "rukpak-core"})
		if err != nil {
			return controllerutil.OperationResultNone, err
		}
	}

	// make sure the passed in pod value is updated with the latest
	// version of the pod
	*pod = *updatedPod

	// compare existingPod to newPod and return an appropriate
	// OperatorResult value.
	newPod := updatedPod.DeepCopy()
	unsetNonComparedPodFields(existingPod, newPod)
	if equality.Semantic.DeepEqual(existingPod, newPod) {
		return controllerutil.OperationResultNone, nil
	}
	return controllerutil.OperationResultUpdated, nil
}

func (i *image) validate(source *v1alpha2.BundleDeplopymentSource, opts UnpackOption) error {
	if source == nil || source.Image == nil {
		return errors.New("source type image empty")
	}
	if opts.BundleDeploymentUID == "" {
		return errors.New("bundle deployment UID required")
	}
	return nil
}

func (i *image) getDesiredPodApplyConfig(bdName string, bundleSrc *v1alpha2.BundleDeplopymentSource, opts UnpackOption) *applyconfigurationcorev1.PodApplyConfiguration {
	// TODO (tyslaton): Address unpacker pod allowing root users for image sources
	//
	// In our current implementation, we are creating a pod that uses the image
	// provided by an image source. This pod is not always guaranteed to run as a
	// non-root user and thus will fail to initialize if running as root in a PSA
	// restricted namespace due to violations. As it currently stands, our compliance
	// with PSA is baseline which allows for pods to run as root users. However,
	// all RukPak processes and resources, except this unpacker pod for image sources,
	// are runnable in a PSA restricted environment. We should consider ways to make
	// this PSA definition either configurable or workable in a restricted namespace.
	//
	// See https://github.com/operator-framework/rukpak/pull/539 for more detail.
	containerSecurityContext := applyconfigurationcorev1.SecurityContext().
		WithAllowPrivilegeEscalation(false).
		WithCapabilities(applyconfigurationcorev1.Capabilities().
			WithDrop("ALL"),
		)

	podApply := applyconfigurationcorev1.Pod(bdName, i.PodNamespace).
		WithLabels(map[string]string{
			util.CoreOwnerKindKey: v1alpha2.BundleDeploymentKind,
			util.CoreOwnerNameKey: bdName,
		}).
		WithOwnerReferences(v1.OwnerReference().
			WithName(bdName).
			WithKind(v1alpha2.BundleDeploymentKind).
			WithAPIVersion(v1alpha2.BundleDeploymentGVK.GroupVersion().String()).
			WithUID(opts.BundleDeploymentUID).
			WithController(true).
			WithBlockOwnerDeletion(true),
		).
		WithSpec(applyconfigurationcorev1.PodSpec().
			WithAutomountServiceAccountToken(false).
			WithRestartPolicy(corev1.RestartPolicyNever).
			WithInitContainers(applyconfigurationcorev1.Container().
				WithName("install-unpacker").
				WithImage(i.UnpackImage).
				WithImagePullPolicy(corev1.PullIfNotPresent).
				WithCommand("cp", "-Rv", "/unpack", "/util/bin/unpack").
				WithVolumeMounts(applyconfigurationcorev1.VolumeMount().
					WithName("util").
					WithMountPath("/util/bin"),
				).
				WithSecurityContext(containerSecurityContext),
			).
			WithContainers(applyconfigurationcorev1.Container().
				WithName(imageUnpackContainerName).
				WithImage(bundleSrc.Image.Ref).
				WithCommand("/bin/unpack", "--bundle-dir", "/").
				WithVolumeMounts(applyconfigurationcorev1.VolumeMount().
					WithName("util").
					WithMountPath("/bin"),
				).
				WithSecurityContext(containerSecurityContext),
			).
			WithVolumes(applyconfigurationcorev1.Volume().
				WithName("util").
				WithEmptyDir(applyconfigurationcorev1.EmptyDirVolumeSource()),
			).
			WithSecurityContext(applyconfigurationcorev1.PodSecurityContext().
				WithRunAsNonRoot(false).
				WithSeccompProfile(applyconfigurationcorev1.SeccompProfile().
					WithType(corev1.SeccompProfileTypeRuntimeDefault),
				),
			),
		)

	if bundleSrc.Image.Auth != nil && bundleSrc.Image.Auth.Secret.Name != "" {
		podApply.Spec = podApply.Spec.WithImagePullSecrets(
			applyconfigurationcorev1.LocalObjectReference().WithName(bundleSrc.Image.Auth.Secret.Name),
		)
	}
	return podApply
}

func unsetNonComparedPodFields(pods ...*corev1.Pod) {
	for _, p := range pods {
		p.APIVersion = ""
		p.Kind = ""
		p.Status = corev1.PodStatus{}
	}
}

func pendingImagePodResult(pod *corev1.Pod) *Result {
	var messages []string
	for _, cStatus := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
		if waiting := cStatus.State.Waiting; waiting != nil {
			if waiting.Reason == "ErrImagePull" || waiting.Reason == "ImagePullBackOff" {
				messages = append(messages, waiting.Message)
			}
		}
	}
	return &Result{State: StateUnpackPending, Message: strings.Join(messages, "; ")}
}

func (i *image) failedPodResult(ctx context.Context, pod *corev1.Pod) error {
	logs, err := i.getPodLogs(ctx, pod)
	if err != nil {
		return fmt.Errorf("unpack failed: failed to retrieve failed pod logs: %v", err)
	}
	_ = i.Client.Delete(ctx, pod)
	return fmt.Errorf("unpack failed: %v", string(logs))
}

func (i *image) getPodLogs(ctx context.Context, pod *corev1.Pod) ([]byte, error) {
	logReader, err := i.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("get pod logs: %v", err)
	}
	defer logReader.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, logReader); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (i *image) succeededPodResult(ctx context.Context, pod *corev1.Pod, bdSrc v1alpha2.BundleDeplopymentSource, store store.Store) (*Result, error) {
	err := i.getBundleContents(ctx, pod, &bdSrc, store, i.getPodLogs)
	if err != nil {
		return nil, fmt.Errorf("get bundle contents: %v", err)
	}

	digest, err := i.getBundleImageDigest(pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle image digest: %v", err)
	}

	resolvedSource := &v1alpha2.BundleDeplopymentSource{
		Image: &v1alpha2.ImageSource{Ref: digest},
	}

	return &Result{ResolvedSource: resolvedSource, State: StateUnpacked}, nil
}

func (i *image) getBundleContents(ctx context.Context, pod *corev1.Pod, bundleSrc *v1alpha2.BundleDeplopymentSource, store store.Store, getLogs logfn) error {
	bundleData, err := getLogs(ctx, pod)
	if err != nil {
		return fmt.Errorf("error getting bundle contents: %v", err)
	}
	bd := struct {
		Content []byte `json:"content"`
	}{}

	if err := json.Unmarshal(bundleData, &bd); err != nil {
		return fmt.Errorf("error parsing bundle data: %v", err)
	}

	if err := store.RemoveAll(filepath.Clean(bundleSrc.Destination)); err != nil {
		return fmt.Errorf("error removing dir %v", err)
	}

	if err := store.MkdirAll(bundleSrc.Destination, 0755); err != nil {
		return fmt.Errorf("error creating storagepath %q", err)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(bd.Content))
	if err != nil {
		return fmt.Errorf("error reading bundle content gzip: %v", err)
	}

	// create a tar reader to parse the decompressed data.
	tr := tar.NewReader(gzr)
	return store.CopyTarArchive(tr, bundleSrc.Destination)
}

func (i *image) getBundleImageDigest(pod *corev1.Pod) (string, error) {
	for _, ps := range pod.Status.ContainerStatuses {
		if ps.Name == imageUnpackContainerName && ps.ImageID != "" {
			return ps.ImageID, nil
		}
	}
	return "", errors.New("image digest not found")
}

func (i *image) handleUnexpectedPod(ctx context.Context, pod *corev1.Pod) error {
	_ = i.Client.Delete(ctx, pod)
	return fmt.Errorf("unexpected pod phase: %v", pod.Status.Phase)
}
