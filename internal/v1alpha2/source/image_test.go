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

package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/operator-framework/rukpak/internal/util"

	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
)

var _ = Describe("Image Suite", func() {
	testImage := image{}
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.TODO()
		k8sClient, err := kubernetes.NewForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		testImage = image{
			PodNamespace: "test",
			UnpackImage:  "testimage",
			Client:       kubeClient,
			KubeClient:   k8sClient,
		}
	})

	Describe("validate", func() {
		It("should validate successfully", func() {
			source := v1alpha2.BundleDeplopymentSource{
				Image: &v1alpha2.ImageSource{
					Ref: "test",
				},
			}

			Expect(testImage.validate(&source, UnpackOption{
				BundleDeploymentUID: types.UID("test"),
			})).To(Succeed())
		})
	})

	Describe("ensureUnpackedPod", func() {
		var (
			bdName = "test"
			pod    = corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bdName,
					Namespace: "test",
				},
			}
			bundleSrc = v1alpha2.BundleDeplopymentSource{
				Image: &v1alpha2.ImageSource{
					Ref: "quay.io/example/test",
				},
			}
		)
		pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})

		BeforeEach(func() {
			// create test namespace
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			}
			Expect(kubeClient.Create(ctx, &ns)).To(Succeed())

			res, err := testImage.ensureUnpackPod(ctx, bdName, bundleSrc, &pod, UnpackOption{types.UID("test")})
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(BeEquivalentTo(controllerutil.OperationResultUpdated))
		})

		AfterEach(func() {
			Expect(kubeClient.Delete(ctx, &pod)).To(Succeed())
			Expect(kubeClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}})).To(Succeed())
		})

		It("should create a new pod with updated pod apply configuration", func() {
			By("verifying if it contains the expected labels")
			Expect(len(pod.GetLabels())).To(BeEquivalentTo(2))
			Expect(pod.GetLabels()[util.CoreOwnerKindKey]).NotTo(BeNil())
			Expect(pod.GetLabels()[util.CoreOwnerNameKey]).NotTo(BeNil())

			By("verifying it contains the expected owner reference")
			Expect(pod.OwnerReferences).To(ContainElement(metav1.OwnerReference{
				Name:               bdName,
				Kind:               v1alpha2.BundleDeploymentKind,
				APIVersion:         v1alpha2.BundleDeploymentGVK.GroupVersion().String(),
				UID:                types.UID("test"),
				Controller:         getPtr(true),
				BlockOwnerDeletion: getPtr(true),
			}))

			By("verifying it contains the expected spec")
			Expect(pod.Spec.AutomountServiceAccountToken).To(BeEquivalentTo(getPtr(false)))
			Expect(pod.Spec.RestartPolicy).To(BeEquivalentTo(corev1.RestartPolicyNever))

			By("verifying if the init container with expected spec exists")
			initContainer := containInitContainerWithName(pod.Spec.InitContainers, "install-unpacker")
			Expect(initContainer).NotTo(BeNil())

			Expect(initContainer.Image).To(BeEquivalentTo(testImage.UnpackImage))
			Expect(initContainer.ImagePullPolicy).To(BeEquivalentTo(corev1.PullIfNotPresent))
			Expect(initContainer.Command).To(BeEquivalentTo([]string{"cp", "-Rv", "/unpack", "/util/bin/unpack"}))
			Expect(*initContainer.SecurityContext).To(BeEquivalentTo(corev1.SecurityContext{
				AllowPrivilegeEscalation: getPtr(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
			}))
			Expect(initContainer.VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      "util",
				MountPath: "/util/bin",
			}))
		})
	})

	Describe("pendingImagePodResult", func() {
		var (
			pod corev1.Pod
		)

		It("should have pending result when container status is waiting", func() {
			pod = corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason:  "ErrImagePull",
									Message: "test error",
								},
							},
						},
					},
				},
			}

			res := pendingImagePodResult(&pod)
			Expect(res).NotTo(BeNil())
			Expect(res.State).To(BeEquivalentTo(StateUnpackPending))
			Expect(res.Message).To(ContainSubstring("test error"))
		})

		It("should have pending result when container status is waiting", func() {
			pod = corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{
									Reason:  "ImagePullBackOff",
									Message: "test error",
								},
							},
						},
					},
				},
			}

			res := pendingImagePodResult(&pod)
			Expect(res).NotTo(BeNil())
			Expect(res.State).To(BeEquivalentTo(StateUnpackPending))
			Expect(res.Message).To(ContainSubstring("test error"))
		})
	})

	Describe("getBundleContents", func() {
		var (
			mockTestStore store.Store
			bdName        string
			destination   = "test/"
		)

		BeforeEach(func() {
			bdName = "test"
			mockTestStore = &MockStore{
				copyTarArchiveFunc: func(tr *tar.Reader, destination string) error {
					return nil
				},
				Fs:               afero.NewMemMapFs(),
				bundleDeployment: bdName,
			}
		})

		It("copies contents successfully", func() {
			err := testImage.getBundleContents(ctx, &corev1.Pod{}, &v1alpha2.BundleDeplopymentSource{
				Destination: destination,
			}, mockTestStore, mockGetPodLogs)
			Expect(err).NotTo(HaveOccurred())

			By("verify if destination has been created successfully in the fs")
			exists, err := afero.DirExists(mockTestStore, destination)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("errors when logs cannot be fetched", func() {
			errGetPods := errors.New("error getting logs")

			err := testImage.getBundleContents(ctx, &corev1.Pod{}, &v1alpha2.BundleDeplopymentSource{
				Destination: destination,
			}, mockTestStore, func(ctx context.Context, pod *corev1.Pod) ([]byte, error) {
				return nil, errGetPods
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errGetPods.Error()))

			By("verify if destination has not been created in the fs")
			exists, err := afero.DirExists(mockTestStore, destination)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("errors when logs cannot be read", func() {
			err := testImage.getBundleContents(ctx, &corev1.Pod{}, &v1alpha2.BundleDeplopymentSource{
				Destination: destination,
			}, mockTestStore, func(ctx context.Context, pod *corev1.Pod) ([]byte, error) {
				return []byte("errored data"), nil
			})
			Expect(err).To(HaveOccurred())

			By("verify if destination has not been created in the fs")
			exists, err := afero.DirExists(mockTestStore, destination)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

})

func getPtr(val bool) *bool {
	return &val
}

func containInitContainerWithName(containers []corev1.Container, name string) *corev1.Container {
	for _, c := range containers {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

func mockGetPodLogs(_ context.Context, _ *corev1.Pod) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	defer gz.Close()

	_, err := gz.Write([]byte("test logs"))
	if err != nil {
		return nil, err
	}

	bd := struct {
		Content []byte `json:"content"`
	}{Content: buf.Bytes()}

	return json.Marshal(bd)
}
