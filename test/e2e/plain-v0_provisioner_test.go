package e2e_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/operator-framework/rukpak/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TODO: make this is a CLI flag?
	defaultSystemNamespace = "rukpak-system"
)

func Logf(f string, v ...interface{}) {
	fmt.Fprintf(GinkgoWriter, f, v...)
}

var _ = Describe("plain-v0 provisioner", func() {
	When("a valid Bundle referencing a remote container image is created", func() {
		var (
			bundle *v1alpha1.Bundle
		)
		BeforeEach(func() {
			By("creating the testing Bundle resource")
			bundle = &v1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "olm-crds",
				},
				Spec: v1alpha1.BundleSpec{
					ProvisionerClassName: "core.rukpak.io/plain-v0",
					Image:                "quay.io/tflannag/olm-plain-bundle:olm-crds-v0.20.0",
				},
			}
			err := c.Create(context.Background(), bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Expect(c.Delete(context.Background(), bundle)).To(BeNil())
		})

		It("should eventually report a successful state", func() {
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() bool {
					if err := c.Get(context.Background(), client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return false
					}
					return bundle.Status.Phase == v1alpha1.PhaseUnpacked
				}).Should(BeTrue())
			})

			By("eventually writing a non-empty image digest to the status", func() {
				Eventually(func() bool {
					if err := c.Get(context.Background(), client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return false
					}
					// Note(tflannag): This may make the test fairly brittle over time. Let's re-evaluate
					// whether we want to test this kind of comparison if there's the potential for moving
					// tags for Bundle container images.
					expectedDigest := "quay.io/tflannag/olm-plain-bundle@sha256:2a72e4a7bc6c7598d1ecdb2f082c865a91dda35ab8e4a8e8bc128cf49a2a619b"
					return bundle.Status.Digest == expectedDigest
				}).Should(BeTrue())
			})

			By("eventually writing a non-empty list of unpacked objects to the status", func() {
				Eventually(func() bool {
					if err := c.Get(context.Background(), client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return false
					}
					if bundle.Status.Info == nil {
						return false
					}
					return len(bundle.Status.Info.Objects) == 8
				}).Should(BeTrue())
			})
		})

		// TODO(tflannag): add test specs for ensuring the object and metadata ConfigMaps get re-created when deleted
		It("should re-create underlying system resources", func() {
			var (
				pod *corev1.Pod
			)

			By("getting the underlying bundle unpacking pod")
			Eventually(func() bool {
				requirement, err := labels.NewRequirement("core.rukpak.io/owner-name", selection.Equals, []string{bundle.GetName()})
				if err != nil {
					return false
				}
				pods := &corev1.PodList{}
				if err := c.List(context.Background(), pods, &client.ListOptions{
					LabelSelector: labels.NewSelector().Add(*requirement),
					Namespace:     defaultSystemNamespace,
				}); err != nil {
					return false
				}
				if len(pods.Items) != 1 {
					return false
				}
				pod = &pods.Items[0]
				return true
			}).Should(BeTrue())

			By("storing the pod's original UID")
			originalUID := pod.GetUID()

			By("deleting the underlying pod and waiting for it to be re-created")
			err := client.IgnoreNotFound(c.Delete(context.Background(), pod))
			Expect(err).To(BeNil())

			By("verifying the pod's UID has changed")
			Eventually(func() (types.UID, error) {
				err := c.Get(context.Background(), client.ObjectKeyFromObject(pod), pod)
				return pod.GetUID(), err
			}).ShouldNot(Equal(originalUID))
		})
	})

	When("an invalid Bundle referencing a remote container image is created", func() {
		var (
			bundle *v1alpha1.Bundle
		)
		BeforeEach(func() {
			By("creating the testing Bundle resource")
			bundle = &v1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "olm-crds",
				},
				Spec: v1alpha1.BundleSpec{
					ProvisionerClassName: "core.rukpak.io/plain-v0",
					Image:                "quay.io/tflannag/olm-plain-bundle:non-existent-tag",
				},
			}
			err := c.Create(context.Background(), bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Expect(c.Delete(context.Background(), bundle)).To(BeNil())
		})

		It("checks the bundle's phase is stuck in pending", func() {
			By("waiting until the pod is reporting ImagePullBackOff state")
			Eventually(func() bool {
				pod := &corev1.Pod{}
				if err := c.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("plain-unpack-bundle-%s", bundle.GetName()),
					Namespace: "rukpak-system",
				}, pod); err != nil {
					return false
				}
				if pod.Status.Phase != corev1.PodPending {
					return false
				}
				for _, status := range pod.Status.ContainerStatuses {
					if status.State.Waiting != nil && status.State.Waiting.Reason == "ImagePullBackOff" {
						return true
					}
				}
				return false
			}).Should(BeTrue())

			By("waiting for the bundle to report back that state")
			Eventually(func() bool {
				err := c.Get(context.Background(), client.ObjectKeyFromObject(bundle), bundle)
				if err != nil {
					return false
				}
				if bundle.Status.Phase != v1alpha1.PhasePending {
					return false
				}
				unpackPending := meta.FindStatusCondition(bundle.Status.Conditions, v1alpha1.PhaseUnpacked)
				if unpackPending == nil {
					return false
				}
				if unpackPending.Message != fmt.Sprintf(`Back-off pulling image "%s"`, bundle.Spec.Image) {
					return false
				}
				return true
			}).Should(BeTrue())
		})
	})
})
