package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
)

var _ = Describe("bundle api validation", func() {
	When("the bundle name is too long", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
			err              error
		)
		BeforeEach(func() {
			ctx = context.Background()

			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "olm-crds-too-long-name-for-the-bundle-1234567890-1234567890",
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
			By("deleting the testing Bundle Deployment resource for failure case")
			err = c.Delete(ctx, bundleDeployment)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the long name of bundle deployment during creation", func() {
			Expect(err).To(MatchError(ContainSubstring("metadata.name: Too long: may not be longer than 52")))
		})
	})
	When("the bundle deployment with multiple sources", func() {
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
						Type: "invalid source",
						Image: &rukpakv1alpha2.ImageSource{
							Ref: "localhost/testdata/bundles/plain-v0:valid",
						},
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha2.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								Tag:    "v0.0.1",
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
			Expect(err).To(MatchError(ContainSubstring("must validate one and only one schema (oneOf). Found 2 valid alternatives")))
		})
	})

	When("the bundle with no sources", func() {
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
						Type: "invalid source",
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
			Expect(err).To(MatchError(ContainSubstring("must validate one and only one schema (oneOf). Found none valid")))
		})
	})

	When("the bundle source type is git and more than 1 refs are set", func() {
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
					Name: "bundlenamemorerefs",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha2.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								Tag:    "v0.0.1",
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
			Expect(err).To(MatchError(ContainSubstring("\"spec.source.git.ref\" must validate one and only one schema (oneOf). Found 2 valid alternatives")))
		})
	})

	When("the bundle source type is git and no refs are set", func() {
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
					Name: "bundlenamemorerefs",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref:        rukpakv1alpha2.GitRef{},
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
			Expect(err).To(MatchError(ContainSubstring("\"spec.source.git.ref\" must validate one and only one schema (oneOf). Found none valid")))
		})
	})
	When("a Bundle references an invalid provisioner class name", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
		})
		AfterEach(func() {
			By("ensuring the testing Bundle does not exist")
			err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), &rukpakv1alpha2.BundleDeployment{})
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()), fmt.Sprintf("error was: %v", err))
		})
		It("should fail validation", func() {
			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("bundle-invalid-%s", rand.String(6)),
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: "invalid/class-name",
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: "localhost/testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).To(And(
				Not(BeNil()),
				WithTransform(apierrors.IsInvalid, Equal(true)),
				MatchError(ContainSubstring(`Invalid value: "invalid/class-name": spec.provisionerClassName`)),
			))
		})
	})
	When("a BundleDeployment references an invalid provisioner class name", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
		})
		AfterEach(func() {
			By("ensuring the testing Bundle does not exist")
			err := c.Get(ctx, client.ObjectKeyFromObject(bd), &rukpakv1alpha2.BundleDeployment{})
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()), fmt.Sprintf("error was: %v", err))
		})
		It("should fail validation", func() {
			By("creating the testing BundleDeployment resource")
			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("bd-invalid-%s", rand.String(6)),
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: "invalid/class-name",
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: "localhost/testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(And(
				Not(BeNil()),
				WithTransform(apierrors.IsInvalid, Equal(true)),
				MatchError(ContainSubstring(`Invalid value: "invalid/class-name": spec.provisionerClassName`)),
			))
		})
	})
})
