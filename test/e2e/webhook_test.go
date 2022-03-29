package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("bundle api validating webhook", func() {
	When("Bundle is valid", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
			err    error
		)

		BeforeEach(func() {
			By("creating the valid Bundle resource")
			ctx = context.Background()

			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "valid-bundle-",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
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
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})
		It("should create the bundle resource", func() {
			Expect(err).To(BeNil())
		})
	})
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
					ProvisionerClassName: plainProvisionerID,
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
			err := c.Delete(ctx, bundle)
			Expect(err).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should fail the long name bundle creation", func() {
			Expect(err).To(MatchError(ContainSubstring(fmt.Sprintf("bundle name %s is too long: maximum allowed name length is 40", bundle.GetName()))))
		})
	})
})
