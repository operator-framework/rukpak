package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
)

var _ = Describe("bundle api validating test", func() {
	When("the bundle name is too long", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)
		BeforeEach(func() {
			By("creating the Bundle resource with a long name")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bundlename-0123456789012345678901234567891",
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
			Expect(err).To(MatchError(ContainSubstring("metadata.name: Too long: may not be longer than 40")))
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
})
