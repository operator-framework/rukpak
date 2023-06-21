package source

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/nlepage/go-tarfs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfigurationcorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	v1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//TODO: Add comments to improve the godocs

type ImageUnpackerOption func(i *image)

// WithPodNamespace configures the namespace used
// by the Image Unpacker when creating a Pod
func WithPodNamespace(ns string) ImageUnpackerOption {
	return func(i *image) {
		i.PodNamespace = ns
	}
}

// WithUnpackImage configures the image used by
// the Image Unpacker Pod to unpack an image source
func WithUnpackImage(img string) ImageUnpackerOption {
	return func(i *image) {
		i.UnpackImage = img
	}
}

// WithFieldManager configures the field manager used
// by the Image Unpacker to set the Pod field manager
func WithFieldManager(manager string) ImageUnpackerOption {
	return func(i *image) {
		i.FieldManager = manager
	}
}

// WithDir configures the directory passed to the
// Unpack Pod by the Image Unpacker. This dictates
// which directory of information should be unpacked
func WithDir(dir string) ImageUnpackerOption {
	return func(i *image) {
		i.BundleDir = dir
	}
}

type LabelFunc func(client.Object) map[string]string

// WithLabelsFunc configures the function used by the Image
// Unpacker to add labels to the Unpacker Pod.
func WithLabelsFunc(labelFunc LabelFunc) ImageUnpackerOption {
	return func(i *image) {
		i.LabelsFunc = labelFunc
	}
}

// NewImageUnpacker returns a new Unpacker for unpacking sources of type "image"
func NewImageUnpacker(cli client.Client, kubeCli kubernetes.Interface, opts ...ImageUnpackerOption) Unpacker {
	image := &image{
		Client:       cli,
		KubeClient:   kubeCli,
		PodNamespace: "default",
		UnpackImage:  "quay.io/operator-framework/rukpak:main",
		FieldManager: "unpacker",
		BundleDir:    "/",
		LabelsFunc:   func(o client.Object) map[string]string { return map[string]string{} },
	}

	for _, opt := range opts {
		opt(image)
	}

	return image
}

type image struct {
	Client       client.Client
	KubeClient   kubernetes.Interface
	PodNamespace string
	UnpackImage  string
	FieldManager string
	BundleDir    string
	LabelsFunc   LabelFunc
}

const imageBundleUnpackContainerName = "src"

func (i *image) Unpack(ctx context.Context, src *Source, obj client.Object) (*Result, error) {
	if src.Type != SourceTypeImage {
		return nil, fmt.Errorf("source type %q not supported", src.Type)
	}
	if src.Image == nil {
		return nil, fmt.Errorf("source image configuration is unset")
	}

	pod := &corev1.Pod{}
	op, err := i.ensureUnpackPod(ctx, src, pod, obj)
	if err != nil {
		return nil, err
	} else if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated || pod.DeletionTimestamp != nil {
		return &Result{State: StatePending}, nil
	}

	switch phase := pod.Status.Phase; phase {
	case corev1.PodPending:
		return pendingImagePodResult(pod), nil
	case corev1.PodRunning:
		return &Result{State: StateUnpacking}, nil
	case corev1.PodFailed:
		return nil, i.failedPodResult(ctx, pod)
	case corev1.PodSucceeded:
		return i.succeededPodResult(ctx, pod)
	default:
		return nil, i.handleUnexpectedPod(ctx, pod)
	}
}

func (i *image) ensureUnpackPod(ctx context.Context, src *Source, pod *corev1.Pod, obj client.Object) (controllerutil.OperationResult, error) {
	existingPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: i.PodNamespace, Name: obj.GetName()}}
	if err := i.Client.Get(ctx, client.ObjectKeyFromObject(existingPod), existingPod); client.IgnoreNotFound(err) != nil {
		return controllerutil.OperationResultNone, err
	}

	podApplyConfig := i.getDesiredPodApplyConfig(src, obj)
	updatedPod, err := i.KubeClient.CoreV1().Pods(i.PodNamespace).Apply(ctx, podApplyConfig, metav1.ApplyOptions{Force: true, FieldManager: i.FieldManager})
	if err != nil {
		if !apierrors.IsInvalid(err) {
			return controllerutil.OperationResultNone, err
		}
		if err := i.Client.Delete(ctx, existingPod); err != nil {
			return controllerutil.OperationResultNone, err
		}
		updatedPod, err = i.KubeClient.CoreV1().Pods(i.PodNamespace).Apply(ctx, podApplyConfig, metav1.ApplyOptions{Force: true, FieldManager: i.FieldManager})
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

func (i *image) getDesiredPodApplyConfig(src *Source, obj client.Object) *applyconfigurationcorev1.PodApplyConfiguration {
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

	podApply := applyconfigurationcorev1.Pod(obj.GetName(), i.PodNamespace).
		WithLabels(i.LabelsFunc(obj)).
		WithOwnerReferences(v1.OwnerReference().
			WithName(obj.GetName()).
			WithKind(obj.GetObjectKind().GroupVersionKind().Kind).
			WithAPIVersion(obj.GetObjectKind().GroupVersionKind().GroupVersion().String()).
			WithUID(obj.GetUID()).
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
				WithName(imageBundleUnpackContainerName).
				WithImage(src.Image.Ref).
				WithCommand("/bin/unpack", "--bundle-dir", i.BundleDir).
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

	if src.Image.ImagePullSecretName != "" {
		podApply.Spec = podApply.Spec.WithImagePullSecrets(
			applyconfigurationcorev1.LocalObjectReference().WithName(src.Image.ImagePullSecretName),
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

func (i *image) failedPodResult(ctx context.Context, pod *corev1.Pod) error {
	logs, err := i.getPodLogs(ctx, pod)
	if err != nil {
		return fmt.Errorf("unpack failed: failed to retrieve failed pod logs: %v", err)
	}
	_ = i.Client.Delete(ctx, pod)
	return fmt.Errorf("unpack failed: %v", string(logs))
}

func (i *image) succeededPodResult(ctx context.Context, pod *corev1.Pod) (*Result, error) {
	bundleFS, err := i.getBundleContents(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get src contents: %v", err)
	}

	digest, err := i.getBundleImageDigest(pod)
	if err != nil {
		return nil, fmt.Errorf("get src image digest: %v", err)
	}

	resolvedSource := &Source{
		Type:  SourceTypeImage,
		Image: &ImageSource{Ref: digest},
	}

	message := generateMessage("image")

	return &Result{FS: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}

func (i *image) getBundleContents(ctx context.Context, pod *corev1.Pod) (fs.FS, error) {
	bundleData, err := i.getPodLogs(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get src contents: %v", err)
	}
	bd := struct {
		Content []byte `json:"content"`
	}{}

	if err := json.Unmarshal(bundleData, &bd); err != nil {
		return nil, fmt.Errorf("parse src data: %v", err)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(bd.Content))
	if err != nil {
		return nil, fmt.Errorf("read src content gzip: %v", err)
	}
	return tarfs.New(gzr)
}

func (i *image) getBundleImageDigest(pod *corev1.Pod) (string, error) {
	for _, ps := range pod.Status.ContainerStatuses {
		if ps.Name == imageBundleUnpackContainerName && ps.ImageID != "" {
			return ps.ImageID, nil
		}
	}
	return "", fmt.Errorf("src image digest not found")
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

func (i *image) handleUnexpectedPod(ctx context.Context, pod *corev1.Pod) error {
	_ = i.Client.Delete(ctx, pod)
	return fmt.Errorf("unexpected pod phase: %v", pod.Status.Phase)
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
	return &Result{State: StatePending, Message: strings.Join(messages, "; ")}
}
