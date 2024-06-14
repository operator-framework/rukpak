package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	registryprovisioner "github.com/operator-framework/rukpak/pkg/provisioner/registry"
)

var _ = Describe("registry provisioner bundle", func() {
	When("a BundleDeployment targets a registry+v1 Bundle", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()
			var secret corev1.Secret

			name := types.NamespacedName{
				Name:      "rukpak-e2e-registry",
				Namespace: "rukpak-e2e",
			}
			err := c.Get(ctx, name, &secret)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret.Type).To(Equal(corev1.SecretTypeTLS))
			data, ok := secret.Data["ca.crt"]
			Expect(ok).To(BeTrue())
			Expect(data).ToNot(BeNil())
			certData := string(data[:])

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "prometheus",
					Labels: map[string]string{
						"app.kubernetes.io/name": "prometheus",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: registryprovisioner.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref:                   fmt.Sprintf("%v/%v", ImageRepo, "registry:valid"),
							InsecureSkipTLSVerify: false,
							CertificateData:       certData,
						},
					},
				},
			}
			err = c.Create(ctx, bd)
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
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
			))
		})
	})
	When("a BundleDeployment targets an invalid registry+v1 Bundle", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cincinnati",
					Labels: map[string]string{
						"app.kubernetes.io/name": "cincinnati",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					InstallNamespace:     "default",
					ProvisionerClassName: registryprovisioner.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref:                   fmt.Sprintf("%v/%v", ImageRepo, "registry:invalid"),
							InsecureSkipTLSVerify: true,
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing BD resource")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should eventually write a failed conversion state to the bundledeployment status", func() {
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("convert registry+v1 bundle to plain+v0 bundle:")),
			))
		})
	})
})
