package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/provisioner/kustomize"
)

var _ = Describe("kustomize provisioner bundledeployment", func() {
	When("a BundleDeployment targets a valid Bundle", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "kustomuze-",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: kustomize.ProvisionerID,
					Config:               runtime.RawExtension{Raw: []byte(`{"path": "dev"}`)},
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "test-kustomize",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: kustomize.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeGit,
								Git: &rukpakv1alpha1.GitSource{
									Repository: "https://github.com/akihikokuroda/kustomize.git",
									Directory:  "./manifests",
									Ref: rukpakv1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(BeNil())
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

		When("the generated resource contains a pod manifest", func() {
			It("should eventually result in an available pod resource", func() {
				By("eventually install generated resource successfully")
				pod := &corev1.Pod{}

				Eventually(func() (*corev1.PodCondition, error) {
					if err := c.Get(ctx, types.NamespacedName{Name: "dev-myapp-pod", Namespace: defaultSystemNamespace}, pod); err != nil {
						return nil, err
					}
					for _, c := range pod.Status.Conditions {
						if c.Type == corev1.PodReady {
							return &c, nil
						}
					}
					return nil, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *corev1.PodCondition) corev1.PodConditionType { return c.Type }, Equal(corev1.PodReady)),
					WithTransform(func(c *corev1.PodCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
				))
			})

			It("should re-create a deployment resource when manually deleted", func() {
				pod := &corev1.Pod{}

				Eventually(func() error {
					return c.Get(ctx, types.NamespacedName{Name: "dev-myapp-pod", Namespace: defaultSystemNamespace}, pod)
				}).Should(Succeed())

				By("storing the pod's original UID")
				originalUID := pod.GetUID()

				By("deleting the underlying pod and waiting for it to be re-created")
				err := c.Delete(context.Background(), pod)
				Expect(err).To(BeNil())

				By("verifying the pod's UID has changed")
				Eventually(func() (types.UID, error) {
					err := c.Get(ctx, client.ObjectKeyFromObject(pod), pod)
					return pod.GetUID(), err
				}).ShouldNot(Equal(originalUID))
			})
		})
	})
	When("a BundleDeployment targets a valid Bundle specified path from repo root", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "kustomuze-",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: kustomize.ProvisionerID,
					Config:               runtime.RawExtension{Raw: []byte(`{"path": "manifests/dev"}`)},
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "test-kustomize",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: kustomize.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeGit,
								Git: &rukpakv1alpha1.GitSource{
									Repository: "https://github.com/akihikokuroda/kustomize.git",
									Ref: rukpakv1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(BeNil())
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

	When("a BundleDeployment targets a valid Bundle without specified path", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "kustomuze-",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: kustomize.ProvisionerID,
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "test-kustomize",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: kustomize.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeGit,
								Git: &rukpakv1alpha1.GitSource{
									Repository: "https://github.com/akihikokuroda/kustomize.git",
									Directory:  "./manifests",
									Ref: rukpakv1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(BeNil())
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
		It("should eventually result in an available pod resource", func() {
			By("eventually install generated resource successfully")
			pod := &corev1.Pod{}

			Eventually(func() (*corev1.PodCondition, error) {
				if err := c.Get(ctx, types.NamespacedName{Name: "cluster-a-dev-myapp-pod", Namespace: defaultSystemNamespace}, pod); err != nil {
					return nil, err
				}
				for _, c := range pod.Status.Conditions {
					if c.Type == corev1.PodReady {
						return &c, nil
					}
				}
				return nil, nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *corev1.PodCondition) corev1.PodConditionType { return c.Type }, Equal(corev1.PodReady)),
				WithTransform(func(c *corev1.PodCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
			))
			Eventually(func() (*corev1.PodCondition, error) {
				if err := c.Get(ctx, types.NamespacedName{Name: "cluster-a-prod-myapp-pod", Namespace: defaultSystemNamespace}, pod); err != nil {
					return nil, err
				}
				for _, c := range pod.Status.Conditions {
					if c.Type == corev1.PodReady {
						return &c, nil
					}
				}
				return nil, nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *corev1.PodCondition) corev1.PodConditionType { return c.Type }, Equal(corev1.PodReady)),
				WithTransform(func(c *corev1.PodCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
			))
			Eventually(func() (*corev1.PodCondition, error) {
				if err := c.Get(ctx, types.NamespacedName{Name: "cluster-a-stag-myapp-pod", Namespace: defaultSystemNamespace}, pod); err != nil {
					return nil, err
				}
				for _, c := range pod.Status.Conditions {
					if c.Type == corev1.PodReady {
						return &c, nil
					}
				}
				return nil, nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *corev1.PodCondition) corev1.PodConditionType { return c.Type }, Equal(corev1.PodReady)),
				WithTransform(func(c *corev1.PodCondition) corev1.ConditionStatus { return c.Status }, Equal(corev1.ConditionTrue)),
			))
		})
	})

	When("a BundleDeployment targets an invalid Bundle", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "kustomuze-",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: kustomize.ProvisionerID,
					Config:               runtime.RawExtension{Raw: []byte(`{"path": "invalid"}`)},
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "test-kustomize",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: kustomize.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeGit,
								Git: &rukpakv1alpha1.GitSource{
									Repository: "https://github.com/akihikokuroda/kustomize.git",
									Directory:  "./invalid",
									Ref: rukpakv1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(BeNil())
		})

		It("should fail kustomizing resources", func() {
			By("eventually writing a bundle load fail state back to the bundledeployment status")
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
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Failed to unpack")),
			))
		})
	})
	When("a kuztomize path is invalid", func() {
		var (
			bd  *rukpakv1alpha1.BundleDeployment
			ctx context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			bd = &rukpakv1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "kustomuze-",
				},
				Spec: rukpakv1alpha1.BundleDeploymentSpec{
					ProvisionerClassName: kustomize.ProvisionerID,
					Config:               runtime.RawExtension{Raw: []byte(`{"path": "invalid"}`)},
					Template: &rukpakv1alpha1.BundleTemplate{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name": "test-kustomize",
							},
						},
						Spec: rukpakv1alpha1.BundleSpec{
							ProvisionerClassName: kustomize.ProvisionerID,
							Source: rukpakv1alpha1.BundleSource{
								Type: rukpakv1alpha1.SourceTypeGit,
								Git: &rukpakv1alpha1.GitSource{
									Repository: "https://github.com/akihikokuroda/kustomize.git",
									Directory:  "./manifests",
									Ref: rukpakv1alpha1.GitRef{
										Branch: "main",
									},
								},
							},
						},
					},
				},
			}
			err := c.Create(ctx, bd)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing resources")
			Expect(c.Delete(ctx, bd)).To(BeNil())
		})

		It("should fail kustomizing resources", func() {
			By("eventually writing a bundle load fail state back to the bundledeployment status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha1.TypeHasValidBundle), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeHasValidBundle)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonBundleLoadFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("kustomize error: must build at directory: not a valid directory")),
			))
		})
	})
})
