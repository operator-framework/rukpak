package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/provisioner/helm"
)

var _ = Describe("helm provisioner bundledeployment", func() {
	When("a BundleDeployment targets a valid Bundle", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeHTTP,
						HTTP: &rukpakv1alpha2.HTTPSource{
							URL: "https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
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
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
			))
		})

		When("the underlying helm chart contains a deployment manifest", func() {
			It("should eventually result in an available deployment resource", func() {
				By("eventually install helm chart successfully")
				deployment := &appsv1.Deployment{}

				Eventually(func() (*appsv1.DeploymentCondition, error) {
					if err := c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment); err != nil {
						return nil, err
					}
					for _, c := range deployment.Status.Conditions {
						if c.Type == appsv1.DeploymentAvailable {
							return &c, nil
						}
					}
					return nil, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *appsv1.DeploymentCondition) appsv1.DeploymentConditionType { return c.Type }, Equal(appsv1.DeploymentAvailable)),
					WithTransform(func(c *appsv1.DeploymentCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Reason }, Equal("MinimumReplicasAvailable")),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Message }, ContainSubstring("Deployment has minimum availability.")),
				))
			})
			It("should re-create a deployment resource when manually deleted", func() {
				deployment := &appsv1.Deployment{}

				Eventually(func() error {
					return c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment)
				}).Should(Succeed())

				By("deleting the deployment resource in the helm chart")
				Expect(c.Delete(ctx, deployment)).To(Succeed())

				By("verifying the deleted deployment resource in the helm chart gets recreated")
				Eventually(func() (*appsv1.DeploymentCondition, error) {
					if err := c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment); err != nil {
						return nil, err
					}
					for _, c := range deployment.Status.Conditions {
						if c.Type == appsv1.DeploymentAvailable {
							return &c, nil
						}
					}
					return nil, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *appsv1.DeploymentCondition) appsv1.DeploymentConditionType { return c.Type }, Equal(appsv1.DeploymentAvailable)),
					WithTransform(func(c *appsv1.DeploymentCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Reason }, Equal("MinimumReplicasAvailable")),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Message }, ContainSubstring("Deployment has minimum availability.")),
				))
			})
		})
	})

	When("a BundleDeployment targets a Bundle with an invalid url", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeHTTP,
						HTTP: &rukpakv1alpha2.HTTPSource{
							URL: "https://github.com/helm/examples/releases/download/hello-world-0.1.0/xxx",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should fail rolling out the bundle contents", func() {
			By("eventually writing an installation state back to the bundledeployment status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha2.TypeUnpacked), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeUnpacked)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`unexpected status "404 Not Found"`)),
			))
		})
	})
	When("a BundleDeployment targets a Bundle with a none-tgz file url", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeHTTP,
						HTTP: &rukpakv1alpha2.HTTPSource{
							URL: "https://raw.githubusercontent.com/helm/examples/main/LICENSE",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should fail rolling out the bundle contents", func() {
			By("eventually writing an installation state back to the bundledeployment status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha2.TypeUnpacked), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeUnpacked)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("gzip: invalid header")),
			))
		})
	})
	When("a BundleDeployment targets a Bundle with a none chart tgz url", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeHTTP,
						HTTP: &rukpakv1alpha2.HTTPSource{
							URL: "https://github.com/helm/examples/archive/refs/tags/hello-world-0.1.0.tar.gz",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(Succeed())
		})

		It("should fail rolling out the bundle contents", func() {
			By("eventually writing an installation state back to the bundledeployment status")
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
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Chart.yaml file is missing")),
			))
		})
	})
	When("a BundleDeployment targets a valid Bundle in Github", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/helm/examples",
							Directory:  "./charts",
							Ref: rukpakv1alpha2.GitRef{
								Branch: "main",
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
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
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
			))
		})

		When("the underlying helm chart contains a deployment manifest", func() {
			It("should eventually result in an available deployment resource", func() {
				By("eventually install helm chart successfully")
				deployment := &appsv1.Deployment{}

				Eventually(func() (*appsv1.DeploymentCondition, error) {
					if err := c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment); err != nil {
						return nil, err
					}
					for _, c := range deployment.Status.Conditions {
						if c.Type == appsv1.DeploymentAvailable {
							return &c, nil
						}
					}
					return nil, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *appsv1.DeploymentCondition) appsv1.DeploymentConditionType { return c.Type }, Equal(appsv1.DeploymentAvailable)),
					WithTransform(func(c *appsv1.DeploymentCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Reason }, Equal("MinimumReplicasAvailable")),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Message }, ContainSubstring("Deployment has minimum availability.")),
				))
			})

			// The helm chart provisioner doesn't appear to support dynamically reconciling the underlying
			// chart contents after they have been installed. In the case that a deployment resource has
			// been manually created, that deletion event won't trigger a new reconciliation for the
			// provisioner. Disabling this test until this functionality is added.
			//
			// See https://github.com/operator-framework/rukpak/issues/514 for more information.
			PIt("should re-create a deployment resource when manually deleted", func() {
				deployment := &appsv1.Deployment{}

				Eventually(func() error {
					return c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment)
				}).Should(Succeed())

				By("deleting the deployment resource in the helm chart")
				Expect(c.Delete(ctx, deployment)).To(Succeed())

				By("verifying the deleted deployment resource in the helm chart gets recreated")
				Eventually(func() (*appsv1.DeploymentCondition, error) {
					if err := c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-hello-world", Namespace: defaultSystemNamespace}, deployment); err != nil {
						return nil, err
					}
					for _, c := range deployment.Status.Conditions {
						if c.Type == appsv1.DeploymentAvailable {
							return &c, nil
						}
					}
					return nil, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *appsv1.DeploymentCondition) appsv1.DeploymentConditionType { return c.Type }, Equal(appsv1.DeploymentAvailable)),
					WithTransform(func(c *appsv1.DeploymentCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Reason }, Equal("MinimumReplicasAvailable")),
					WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Message }, ContainSubstring("Deployment has minimum availability.")),
				))
			})
		})
	})
	When("a BundleDeployment targets a valid Bundle with no chart directory in Github", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/helm/examples",
							Directory:  "./charts/hello-world",
							Ref: rukpakv1alpha2.GitRef{
								Branch: "main",
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
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
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
			))
		})
	})
	When("a BundleDeployment targets a valid Bundle with values", func() {
		var (
			bd  *rukpakv1alpha2.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "ahoy-",
					Labels: map[string]string{
						"app.kubernetes.io/name": "ahoy",
					},
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: helm.ProvisionerID,
					Config:               runtime.RawExtension{Raw: []byte(`{"values": "# Default values for hello-world.\n# This is a YAML-formatted file.\n# Declare variables to be passed into your templates.\nreplicaCount: 1\nimage:\n  repository: nginx\n  pullPolicy: IfNotPresent\n  # Overrides the image tag whose default is the chart appVersion.\n  tag: \"\"\nnameOverride: \"fromvalues\"\nfullnameOverride: \"\"\nserviceAccount:\n  # Specifies whether a service account should be created\n  create: true\n  # Annotations to add to the service account\n  annotations: {}\n  # The name of the service account to use.\n  # If not set and create is true, a name is generated using the fullname template\n  name: \"\"\nservice:\n  type: ClusterIP\n  port: 80\n"}`)},
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeHTTP,
						HTTP: &rukpakv1alpha2.HTTPSource{
							URL: "https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz",
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing resources")
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
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
			))

			By("eventually install helm chart successfully")
			deployment := &appsv1.Deployment{}

			Eventually(func() (*appsv1.DeploymentCondition, error) {
				if err := c.Get(ctx, types.NamespacedName{Name: bd.GetName() + "-fromvalues", Namespace: defaultSystemNamespace}, deployment); err != nil {
					return nil, err
				}
				for _, c := range deployment.Status.Conditions {
					if c.Type == appsv1.DeploymentAvailable {
						return &c, nil
					}
				}
				return nil, nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *appsv1.DeploymentCondition) appsv1.DeploymentConditionType { return c.Type }, Equal(appsv1.DeploymentAvailable)),
				WithTransform(func(c *appsv1.DeploymentCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
				WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Reason }, Equal("MinimumReplicasAvailable")),
				WithTransform(func(c *appsv1.DeploymentCondition) string { return c.Message }, ContainSubstring("Deployment has minimum availability.")),
			))
		})
	})
})
