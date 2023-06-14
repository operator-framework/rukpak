package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
	"github.com/operator-framework/rukpak/internal/provisioner/registry"
)

var _ = Describe("registry provisioner bundle", func() {
	When("a BundleDeployment targets a registry+v1 Bundle", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "prometheus",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "prometheus",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: registry.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeImage,
								Image: &rukpakv1alpha1.ImageSource{
									Ref: fmt.Sprintf("%v/%v", ImageRepo, "registry:valid"),
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing BI resource")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should rollout the bundle contents successfully", func() {
			By("eventually writing a successful installation state back to the bundledeployment status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				if bd.Status.ActiveBundle == "" {
					return nil, fmt.Errorf("waiting for bundle name to be populated")
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
			))
		})
	})
	When("a BundleDeployment targets an invalid registry+v1 Bundle", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cincinnati",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "cincinnati",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: registry.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeImage,
								Image: &rukpakv1alpha1.ImageSource{
									Ref: fmt.Sprintf("%v/%v", ImageRepo, "registry:invalid"),
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing BI resource")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should eventually write a failed conversion state to the bundledeployment status", func() {
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha1.TypeHasValidBundle), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeHasValidBundle)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("convert registry+v1 bundle to plain+v0 bundle: AllNamespace install mode must be enabled")),
			))
		})
	})
})
