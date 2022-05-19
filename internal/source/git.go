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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/git"
	"github.com/operator-framework/rukpak/internal/util"
)

type Git struct {
	Client          client.Client
	KubeClient      kubernetes.Interface
	ProvisionerName string
	PodNamespace    string
	UnpackImage     string
	GitClientImage  string
}

const gitBundleUnpackContainerName = "bundle"

func (g *Git) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeGit {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.Git == nil {
		return nil, fmt.Errorf("bundle source git configuration is unset")
	}

	pod := &corev1.Pod{}
	op, err := g.ensureUnpackPod(ctx, bundle, pod)
	if err != nil {
		return nil, err
	}
	if op == controllerutil.OperationResultCreated || op == controllerutil.OperationResultUpdated || pod.DeletionTimestamp != nil {
		return &Result{State: StatePending}, nil
	}

	switch phase := pod.Status.Phase; phase {
	case corev1.PodPending:
		return pendingGitPodResult(pod), nil
	case corev1.PodRunning:
		return &Result{State: StateUnpacking}, nil
	case corev1.PodFailed:
		return nil, g.failedPodResult(ctx, pod)
	case corev1.PodSucceeded:
		return g.succeededPodResult(ctx, bundle, pod)
	default:
		return nil, g.handleUnexpectedPod(ctx, pod)
	}
}

func (g *Git) ensureUnpackPod(ctx context.Context, bundle *rukpakv1alpha1.Bundle, pod *corev1.Pod) (controllerutil.OperationResult, error) {
	controllerRef := metav1.NewControllerRef(bundle, bundle.GroupVersionKind())
	automountServiceAccountToken := false
	pod.SetName(util.PodName(g.ProvisionerName, bundle.Name))
	pod.SetNamespace(g.PodNamespace)

	return util.CreateOrRecreate(ctx, g.Client, pod, func() error {
		pod.SetLabels(map[string]string{
			util.CoreOwnerKindKey: bundle.Kind,
			util.CoreOwnerNameKey: bundle.Name,
		})
		pod.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})
		pod.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
		pod.Spec.RestartPolicy = corev1.RestartPolicyNever

		if len(pod.Spec.InitContainers) != 1 {
			pod.Spec.InitContainers = make([]corev1.Container, 1)
		}

		// Note: initContainer so we can ensure the repository has been cloned
		// at the bundle.Spec.Source.Git.Ref before we unpack the Bundle contents
		// that are stored in the repository.
		pod.Spec.InitContainers[0].Name = "clone-repository"
		// r.GitClientImage configures which git-based container image to use to clone the provided repository
		// r.GitClientImage currently defaults to alpine/git:v2.32.0
		pod.Spec.InitContainers[0].Image = g.GitClientImage
		pod.Spec.InitContainers[0].ImagePullPolicy = corev1.PullIfNotPresent
		cmd, err := git.CloneCommandFor(*bundle.Spec.Source.Git)
		if err != nil {
			return err
		}
		pod.Spec.InitContainers[0].Command = []string{"/bin/sh", "-c", cmd}
		pod.Spec.InitContainers[0].VolumeMounts = []corev1.VolumeMount{{Name: "bundle", MountPath: "/bundle"}}

		if len(pod.Spec.Containers) != 1 {
			pod.Spec.Containers = make([]corev1.Container, 1)
		}

		pod.Spec.Containers[0].Name = gitBundleUnpackContainerName
		pod.Spec.Containers[0].Image = g.UnpackImage
		pod.Spec.Containers[0].ImagePullPolicy = corev1.PullIfNotPresent
		pod.Spec.Containers[0].Command = []string{"/unpack", "--bundle-dir", "/bundle"}
		pod.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "bundle", MountPath: "/bundle"}}

		pod.Spec.Volumes = []corev1.Volume{
			{Name: "bundle", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		}
		return nil
	})
}

func pendingGitPodResult(pod *corev1.Pod) *Result {
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

func (g *Git) failedPodResult(ctx context.Context, pod *corev1.Pod) error {
	logs, err := g.getPodLogs(ctx, pod)
	if err != nil {
		return fmt.Errorf("unpack failed: failed to retrieve failed pod logs: %w", err)
	}
	_ = g.Client.Delete(ctx, pod)
	return fmt.Errorf("unpack failed: %v", string(logs))
}

func (g *Git) succeededPodResult(ctx context.Context, bundle *rukpakv1alpha1.Bundle, pod *corev1.Pod) (*Result, error) {
	bundleFS, err := g.getBundleContents(ctx, pod)
	if err != nil {
		return nil, fmt.Errorf("get bundle contents: %w", err)
	}
	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type: rukpakv1alpha1.SourceTypeGit,
		// TODO: improve git source implementation to return result with commit hash.
		Git: bundle.Spec.Source.Git.DeepCopy(),
	}

	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked}, nil
}

func (g *Git) getBundleContents(ctx context.Context, pod *corev1.Pod) (fs.FS, error) {
	bundleData, err := g.getPodLogs(ctx, pod)
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

func (g *Git) getPodLogs(ctx context.Context, pod *corev1.Pod) ([]byte, error) {
	logReader, err := g.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
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

func (g *Git) handleUnexpectedPod(ctx context.Context, pod *corev1.Pod) error {
	_ = g.Client.Delete(ctx, pod)
	return fmt.Errorf("unexpected pod phase: %v", pod.Status.Phase)
}
