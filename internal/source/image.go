package source

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/nlepage/go-tarfs"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

type Image struct {
	Client       client.Client
	KubeClient   kubernetes.Interface
	JobNamespace string
	UnpackImage  string
}

const imageBundleUnpackContainerName = "bundle"

func (i *Image) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeImage {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.Image == nil {
		return nil, fmt.Errorf("bundle source image configuration is unset")
	}

	job := &batchv1.Job{}
	op, err := i.ensureUnpackJob(ctx, bundle, job)
	if err != nil {
		return nil, err
	} else if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated || job.DeletionTimestamp != nil {
		return &Result{State: StatePending}, nil
	}

	pod, err := i.getPod(ctx, job)
	if err != nil {
		return nil, err
	}

	// If the pod is nil, it means one of two things:
	//  1. the job controller has not yet created a pod. In this case, we set state to Pending
	//     and wait for pod watch to feed more requests to the bundle controller reconciler.
	//  2. the pod was deleted. In this case, we no longer have the pod logs that we need, so
	//     we'll delete the Job and report an error, which should cause us to create a new Job
	//     in a subsequent reconciliation.
	if pod == nil {
		if job.Status.Active > 0 || job.Status.Failed > 0 || job.Status.Succeeded > 0 {
			defer i.tryDeleteJob(ctx, job)
			return nil, errors.New("unpack pod does not exist")
		}
		return &Result{State: StatePending, Message: "waiting for unpack pod to be created"}, nil
	}

	switch phase := pod.Status.Phase; phase {
	case corev1.PodPending:
		return pendingImagePodResult(pod), nil
	case corev1.PodRunning:
		return &Result{State: StateUnpacking}, nil
	case corev1.PodFailed:
		defer i.tryDeleteJob(ctx, job)
		return nil, i.failedPodResult(ctx, pod)
	case corev1.PodSucceeded:
		return i.succeededPodResult(ctx, pod)
	default:
		defer i.tryDeleteJob(ctx, job)
		return nil, fmt.Errorf("unexpected pod phase: %v", phase)
	}
}

func (i *Image) ensureUnpackJob(ctx context.Context, bundle *rukpakv1alpha1.Bundle, job *batchv1.Job) (controllerutil.OperationResult, error) {
	ref := metav1.NewControllerRef(bundle, bundle.GroupVersionKind())
	refs := []metav1.OwnerReference{*ref}
	job.SetName(bundle.Name)
	job.SetNamespace(i.JobNamespace)

	bundleLabels := map[string]string{
		util.CoreOwnerKindKey: bundle.Kind,
		util.CoreOwnerNameKey: bundle.Name,
	}

	return util.CreateOrRecreate(ctx, i.Client, job, func() error {
		job.SetLabels(bundleLabels)
		job.SetOwnerReferences(refs)
		job.Spec.ManualSelector = pointer.Bool(true)
		job.Spec.Selector = metav1.SetAsLabelSelector(bundleLabels)
		// The goal with BackoffLimit: 0 is to ensure that there's never more than one
		// pod for a job. If the pod fails, the job controller will not retry. However,
		// we'll see this and re-spin the entire job with the controller's error
		// handling backoff.
		job.Spec.BackoffLimit = pointer.Int32(0)
		job.Spec.Template.SetLabels(bundleLabels)

		mutatePodSpec(&job.Spec.Template.Spec, i.UnpackImage, bundle)
		return nil
	})
}

func mutatePodSpec(podSpec *corev1.PodSpec, unpackImage string, bundle *rukpakv1alpha1.Bundle) {
	podSpec.AutomountServiceAccountToken = pointer.Bool(false)
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	if len(podSpec.InitContainers) != 1 {
		podSpec.InitContainers = make([]corev1.Container, 1)
	}

	podSpec.InitContainers[0].Name = "install-unpacker"
	podSpec.InitContainers[0].Image = unpackImage
	podSpec.InitContainers[0].ImagePullPolicy = corev1.PullIfNotPresent
	podSpec.InitContainers[0].Command = []string{"cp", "-Rv", "/unpack", "/bin/unpack"}
	podSpec.InitContainers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/bin"}}

	if len(podSpec.Containers) != 1 {
		podSpec.Containers = make([]corev1.Container, 1)
	}

	podSpec.Containers[0].Name = imageBundleUnpackContainerName
	podSpec.Containers[0].Image = bundle.Spec.Source.Image.Ref
	podSpec.Containers[0].Command = []string{"/bin/unpack", "--bundle-dir", "/"}
	podSpec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "util", MountPath: "/bin"}}

	addSecurityContext(podSpec)

	if bundle.Spec.Source.Image.ImagePullSecretName != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: bundle.Spec.Source.Image.ImagePullSecretName}}
	}
	podSpec.Volumes = []corev1.Volume{
		{Name: "util", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
	}
}

func (i *Image) failedPodResult(ctx context.Context, pod *corev1.Pod) error {
	logs, err := i.getPodLogs(ctx, pod)
	if err != nil {
		return fmt.Errorf("unpack failed: failed to retrieve failed pod logs: %v", err)
	}

	return fmt.Errorf("unpack failed: %v", logs)
}

func (i *Image) succeededPodResult(ctx context.Context, pod *corev1.Pod) (*Result, error) {
	bundleFS, err := i.getBundleContents(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle contents: %v", err)
	}

	digest, err := i.getBundleImageDigest(pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle image digest: %v", err)
	}

	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:  rukpakv1alpha1.SourceTypeImage,
		Image: &rukpakv1alpha1.ImageSource{Ref: digest},
	}

	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked}, nil
}

func (i *Image) getBundleContents(ctx context.Context, pod *corev1.Pod) (fs.FS, error) {
	podLogs, err := i.getPodLogs(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle contents: %v", err)
	}
	bd := struct {
		Content []byte `json:"content"`
	}{}

	if err := json.Unmarshal(podLogs[imageBundleUnpackContainerName], &bd); err != nil {
		return nil, fmt.Errorf("parse bundle data: %v", err)
	}

	gzr, err := gzip.NewReader(bytes.NewReader(bd.Content))
	if err != nil {
		return nil, fmt.Errorf("read bundle content gzip: %v", err)
	}
	return tarfs.New(gzr)
}

func (i *Image) getBundleImageDigest(pod *corev1.Pod) (string, error) {
	for _, ps := range pod.Status.ContainerStatuses {
		if ps.Name == imageBundleUnpackContainerName && ps.ImageID != "" {
			return ps.ImageID, nil
		}
	}
	return "", fmt.Errorf("bundle image digest not found")
}

func (i *Image) getPod(ctx context.Context, job *batchv1.Job) (*corev1.Pod, error) {
	podList := corev1.PodList{}
	ls, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		return nil, err
	}
	if err := i.Client.List(ctx, &podList, client.InNamespace(job.Namespace), client.MatchingLabelsSelector{Selector: ls}); err != nil {
		return nil, err
	}
	if len(podList.Items) == 0 {
		return nil, nil
	}
	if len(podList.Items) != 1 {
		return nil, fmt.Errorf("expected exactly 1 job pod, found %d", len(podList.Items))
	}
	return &podList.Items[0], nil
}

func (i *Image) getPodLogs(ctx context.Context, pod *corev1.Pod) (map[string][]byte, error) {
	containerNames := []string{}
	for _, ic := range pod.Spec.InitContainers {
		containerNames = append(containerNames, ic.Name)
	}
	for _, c := range pod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}

	podLogs := map[string][]byte{}
	for _, containerName := range containerNames {
		if err := func() error {
			logReader, err := i.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
			if err != nil {
				return fmt.Errorf("get pod logs for container %q: %v", containerName, err)
			}
			defer logReader.Close()
			buf := &bytes.Buffer{}
			if _, err := io.Copy(buf, logReader); err != nil {
				return fmt.Errorf("read pod logs for container %q: %v", containerName, err)
			}
			podLogs[containerName] = buf.Bytes()
			return nil
		}(); err != nil {
			return nil, err
		}
	}

	return podLogs, nil
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

// addSecurityContext is responsible for taking a container and defining the
// relevant security context values. By having a function do this, we can keep
// that configuration easily consistent and maintainable.
func addSecurityContext(podSpec *corev1.PodSpec) {
	// Check that pod spec is defined before proceeding
	if podSpec == nil {
		return
	}

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
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{}
	}
	podSpec.SecurityContext.RunAsNonRoot = pointer.Bool(false)

	if podSpec.SecurityContext.SeccompProfile == nil {
		podSpec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{}
	}
	podSpec.SecurityContext.SeccompProfile.Type = corev1.SeccompProfileTypeRuntimeDefault

	// Add security context for init containers
	for i := range podSpec.InitContainers {
		if podSpec.InitContainers[i].SecurityContext == nil {
			podSpec.InitContainers[i].SecurityContext = &corev1.SecurityContext{}
		}
		podSpec.InitContainers[i].SecurityContext.AllowPrivilegeEscalation = pointer.Bool(false)

		if podSpec.InitContainers[i].SecurityContext.Capabilities == nil {
			podSpec.InitContainers[i].SecurityContext.Capabilities = &corev1.Capabilities{}
		}
		podSpec.InitContainers[i].SecurityContext.Capabilities.Drop = []corev1.Capability{"ALL"}
	}

	// Add security context for containers
	for i := range podSpec.Containers {
		if podSpec.Containers[i].SecurityContext == nil {
			podSpec.Containers[i].SecurityContext = &corev1.SecurityContext{}
		}
		podSpec.Containers[i].SecurityContext.AllowPrivilegeEscalation = pointer.Bool(false)

		if podSpec.Containers[i].SecurityContext.Capabilities == nil {
			podSpec.Containers[i].SecurityContext.Capabilities = &corev1.Capabilities{}
		}
		podSpec.Containers[i].SecurityContext.Capabilities.Drop = []corev1.Capability{"ALL"}
	}
}

func (i *Image) tryDeleteJob(ctx context.Context, job *batchv1.Job) {
	_ = i.Client.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
}
