package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
)

var _ = Describe("bundle deployment api validating webhook", func() {
	When("Bundle Deployment is valid", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
			err              error
		)

		BeforeEach(func() {
			By("creating the valid Bundle resource")
			ctx = context.Background()

			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "valid-bundle-",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: "localhost/testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err = c.Create(ctx, bundleDeployment)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		It("should create the bundle resource", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})
	When("the bundle source type is git and git properties are not set", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
			err              error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenamegit",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: "localhost/testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err = c.Create(ctx, bundleDeployment)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundleDeployment)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("bundledeployment.spec.source.git must be set for source type \"git\"")))
		})
	})
	When("the bundle source type is image and image properties are not set", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
			err              error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenameimage",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha2.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
							},
						},
					},
				},
			}
			err = c.Create(ctx, bundleDeployment)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundleDeployment)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))

		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("bundledeployment.spec.source.image must be set for source type \"image\"")))
		})
	})
})
