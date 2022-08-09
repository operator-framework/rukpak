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

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
)

var _ = Describe("bundle api validation", func() {
	When("the bundle name is too long", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "olm-crds-too-long-name-for-the-bundle-1234567890-1234567890",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err = c.Create(ctx, bundle)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the long name bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("metadata.name: Too long: may not be longer than 52")))
		})
	})
	When("the bundle with multiple sources", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenamegit",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: "invalid source",
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:valid",
						},
						Git: &rukpakv1alpha1.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha1.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								Tag:    "v0.0.1",
							},
						},
					},
				},
			}
			err = c.Create(ctx, bundle)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("must validate one and only one schema (oneOf). Found 2 valid alternatives")))
		})
	})

	When("the bundle with no sources", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenamegit",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: "invalid source",
					},
				},
			}
			err = c.Create(ctx, bundle)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("must validate one and only one schema (oneOf). Found none valid")))
		})
	})

	When("the bundle source type is git and more than 1 refs are set", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenamemorerefs",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeGit,
						Git: &rukpakv1alpha1.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha1.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								Tag:    "v0.0.1",
							},
						},
					},
				},
			}
			err = c.Create(ctx, bundle)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("\"spec.source.git.ref\" must validate one and only one schema (oneOf). Found 2 valid alternatives")))
		})
	})

	When("the bundle source type is git and no refs are set", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			By("creating the Bundle resource")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlenamemorerefs",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeGit,
						Git: &rukpakv1alpha1.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref:        rukpakv1alpha1.GitRef{},
						},
					},
				},
			}
			err = c.Create(ctx, bundle)
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource for failure case")
			err = c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring("\"spec.source.git.ref\" must validate one and only one schema (oneOf). Found none valid")))
		})
	})
	When("a Bundle references an invalid provisioner class name", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
		})
		AfterEach(func() {
			By("ensuring the testing Bundle does not exist")
			err := c.Get(ctx, client.ObjectKeyFromObject(bundle), &rukpakv1alpha1.Bundle{})
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()), fmt.Sprintf("error was: %v", err))
		})
		It("should fail validation", func() {
			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("bundle-invalid-%s", rand.String(6)),
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: "invalid/class-name",
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:valid",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(And(
				Not(BeNil()),
				WithTransform(apierrors.IsInvalid, Equal(true)),
				MatchError(ContainSubstring(`Invalid value: "invalid/class-name": spec.provisionerClassName`)),
			))
		})
	})
})

var _ = Describe("bundle deployment api validation", func() {
	When("the bundledeployment name is too long", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
			err error
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "olm-crds-too-long-name-for-the-bundledeployment-1234567890",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Template: &rukpakv1alpha1.BundleTemplate{
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: plain.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeImage,
								Image: &rukpakv1alpha1.ImageSource{
									Ref: "testdata/bundles/plain-v0:valid",
								},
							},
						},
					},
				},
			}
			err = c.Create(ctx, bd)
		})
		AfterEach(func() {
			By("deleting the testing BundleDeployment resource for failure case")
			err = c.Delete(ctx, bd)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the long name bundledeployment creation", func() {
			Expect(err).To(MatchError(ContainSubstring("metadata.name: Too long: may not be longer than 45")))
		})
	})
	When("a BundleDeployment references an invalid provisioner class name", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
		})
		AfterEach(func() {
			By("ensuring the testing Bundle does not exist")
			err := c.Get(ctx, client.ObjectKeyFromObject(bd), &rukpakv1alpha1.BundleDeployment{})
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()), fmt.Sprintf("error was: %v", err))
		})
		It("should fail validation", func() {
			By("creating the testing BundleDeployment resource")
			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("bd-invalid-%s", rand.String(6)),
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: "invalid/class-name",
					Template: &rukpakv1alpha1.BundleTemplate{
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: plain.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeImage,
								Image: &rukpakv1alpha1.ImageSource{
									Ref: "testdata/bundles/plain-v0:valid",
								},
							},
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
	When("a BundleDeployment references an invalid provisioner class name in the bundle template", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
		})
		AfterEach(func() {
			By("ensuring the testing BundleDeployment does not exist")
			err := c.Get(ctx, client.ObjectKeyFromObject(bd), &rukpakv1alpha1.BundleDeployment{})
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()), fmt.Sprintf("error was: %v", err))
		})
		It("should fail validation", func() {
			By("creating the testing BundleDeployment resource")
			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("bd-invalid-%s", rand.String(6)),
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Template: &rukpakv1alpha1.BundleTemplate{
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: "invalid/class-name",
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeImage,
								Image: &rukpakv1alpha1.ImageSource{
									Ref: "testdata/bundles/plain-v0:valid",
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(And(
				Not(BeNil()),
				WithTransform(apierrors.IsInvalid, Equal(true)),
				MatchError(ContainSubstring(`Invalid value: "invalid/class-name": spec.template.spec.provisionerClassName`)),
			))
		})
	})
})
