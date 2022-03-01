package e2e_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/operator-framework/rukpak/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("k8s provisioner", func() {
	var (
		bundle *v1alpha1.Bundle
	)
	BeforeEach(func() {
		By("creating the testing Bundle resource")
		bundle = &v1alpha1.Bundle{
			ObjectMeta: v1.ObjectMeta{
				Name: "olm-crds",
			},
			Spec: v1alpha1.BundleSpec{
				ProvisionerClassName: "core.rukpak.io/plain",
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

	When("a valid Bundle referencing a remote container image is created", func() {
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
	})
})
