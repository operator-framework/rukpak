package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	// TODO: make this is a CLI flag?
	defaultSystemNamespace = "rukpak-system"
	plainProvisionerID     = "core.rukpak.io/plain"
)

func Logf(f string, v ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Fprintf(GinkgoWriter, f, v...)
}

var _ = Describe("plain provisioner bundle", func() {
	When("a valid Bundle referencing a remote container image is created", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-valid",
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
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})

		It("should eventually report a successful state", func() {
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() (string, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return "", err
					}
					return bundle.Status.Phase, nil
				}).Should(Equal(rukpakv1alpha1.PhaseUnpacked))
			})

			By("eventually writing a non-empty image digest to the status", func() {
				Eventually(func() (string, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return "", err
					}
					return bundle.Status.Digest, nil
				}).Should(Not(Equal("")))
			})

			By("eventually writing a non-empty list of unpacked objects to the status", func() {
				Eventually(func() (int, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return -1, err
					}
					if bundle.Status.Info == nil {
						return -1, fmt.Errorf("bundle.Status.Info is nil")
					}
					return len(bundle.Status.Info.Objects), nil
				}).Should(Equal(8))
			})
		})

		It("should re-create the bundle configmaps", func() {
			var (
				cm *corev1.ConfigMap
			)

			By("getting the metadata configmap for the bundle")
			// TODO(tflannag): Create a constant for the "core.rukpak.io/configmap-type" label
			// and update the internal/controller packages.
			configMapTypeRequirement, err := labels.NewRequirement("core.rukpak.io/configmap-type", selection.Equals, []string{"metadata"})
			Expect(err).To(BeNil())

			selector := util.NewBundleLabelSelector(bundle)
			selector = selector.Add(*configMapTypeRequirement)

			Eventually(func() bool {
				cms := &corev1.ConfigMapList{}
				if err := c.List(ctx, cms, &client.ListOptions{
					Namespace:     defaultSystemNamespace,
					LabelSelector: selector,
				}); err != nil {
					return false
				}
				if len(cms.Items) != 1 {
					return false
				}
				cm = &cms.Items[0]

				return true
			}).Should(BeTrue())

			By("deleting the metadata configmap and checking if it gets recreated")
			originalUID := cm.GetUID()

			Eventually(func() error {
				return c.Delete(ctx, cm)
			}).Should(Succeed())

			Eventually(func() (types.UID, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(cm), cm)
				return cm.GetUID(), err
			}).ShouldNot(Equal(originalUID))
		})

		It("should re-create underlying system resources", func() {
			var (
				pod *corev1.Pod
			)

			By("getting the underlying bundle unpacking pod")
			selector := util.NewBundleLabelSelector(bundle)
			Eventually(func() bool {
				pods := &corev1.PodList{}
				if err := c.List(ctx, pods, &client.ListOptions{
					Namespace:     defaultSystemNamespace,
					LabelSelector: selector,
				}); err != nil {
					return false
				}
				if len(pods.Items) != 1 {
					return false
				}
				pod = &pods.Items[0]
				return true
			}).Should(BeTrue())

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

	When("an invalid Bundle referencing a remote container image is created", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-invalid",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:non-existent-tag",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})

		It("checks the bundle's phase is stuck in pending", func() {
			By("waiting until the pod is reporting ImagePullBackOff state")
			Eventually(func() bool {
				pod := &corev1.Pod{}
				if err := c.Get(ctx, types.NamespacedName{
					Name:      util.PodName("plain", bundle.GetName()),
					Namespace: defaultSystemNamespace,
				}, pod); err != nil {
					return false
				}
				if pod.Status.Phase != corev1.PodPending {
					return false
				}
				for _, status := range pod.Status.ContainerStatuses {
					if status.State.Waiting != nil && status.State.Waiting.Reason == "ImagePullBackOff" {
						return true
					}
				}
				return false
			}).Should(BeTrue())

			By("waiting for the bundle to report back that state")
			Eventually(func() bool {
				err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle)
				if err != nil {
					return false
				}
				if bundle.Status.Phase != rukpakv1alpha1.PhasePending {
					return false
				}
				unpackPending := meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.PhaseUnpacked)
				if unpackPending == nil {
					return false
				}
				if unpackPending.Message != fmt.Sprintf(`Back-off pulling image "%s"`, bundle.Spec.Source.Image.Ref) {
					return false
				}
				return true
			}).Should(BeTrue())
		})
	})

	When("a bundle containing no manifests is created", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-unsupported",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:empty",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})
		It("reports an unpack error when the manifests directory is missing", func() {
			By("waiting for the bundle to report back that state")
			Eventually(func() (*metav1.Condition, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle)
				if err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.TypeUnpacked), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeUnpacked)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`readdir manifests: file does not exist`)),
			))
		})
	})

	When("a bundle containing an empty manifests directory is created", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-unsupported",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:no-manifests",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})
		It("reports an unpack error when the manifests directory contains no objects", func() {
			By("waiting for the bundle to report back that state")
			Eventually(func() (*metav1.Condition, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle)
				if err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.TypeUnpacked), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeUnpacked)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`found zero objects: plain+v0 bundles are required to contain at least one object`)),
			))
		})
	})

	When("Bundles are backed by a git repository", func() {
		var (
			bundles []*rukpakv1alpha1.Bundle
			ctx     context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the git based Bundles")
			bundles = []*rukpakv1alpha1.Bundle{
				{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-commit",
					},
					Spec: rukpakv1alpha1.BundleSpec{
						ProvisionerClassName: plainProvisionerID,
						Source: rukpakv1alpha1.BundleSource{
							Type: rukpakv1alpha1.SourceTypeGit,
							Git: &rukpakv1alpha1.GitSource{
								Repository: "https://github.com/exdx/combo-bundle",
								Ref: rukpakv1alpha1.GitRef{
									Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-tag",
					},
					Spec: rukpakv1alpha1.BundleSpec{
						ProvisionerClassName: plainProvisionerID,
						Source: rukpakv1alpha1.BundleSource{
							Type: rukpakv1alpha1.SourceTypeGit,
							Git: &rukpakv1alpha1.GitSource{
								Repository: "https://github.com/exdx/combo-bundle",
								Ref: rukpakv1alpha1.GitRef{
									Tag: "v0.0.1",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-defaults",
					},
					Spec: rukpakv1alpha1.BundleSpec{
						ProvisionerClassName: plainProvisionerID,
						Source: rukpakv1alpha1.BundleSource{
							Type: rukpakv1alpha1.SourceTypeGit,
							Git: &rukpakv1alpha1.GitSource{
								Repository: "https://github.com/exdx/combo-bundle.git",
								Ref: rukpakv1alpha1.GitRef{
									Branch: "main",
								},
							},
						},
					},
				},
			}

			for _, bundle := range bundles {
				err := c.Create(ctx, bundle)
				Expect(err).To(BeNil())
			}
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			for _, bundle := range bundles {
				err := c.Delete(ctx, bundle)
				Expect(err).To(BeNil())
			}
		})

		It("should source the git content specified and unpack it to the cluster successfully", func() {
			By("eventually reporting an Unpacked phase", func() {
				for _, bundle := range bundles {
					Eventually(func() bool {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
							return false
						}
						return bundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked
					}).Should(BeTrue())
				}
			})

			By("eventually writing a non-empty list of unpacked objects to the status", func() {
				for _, bundle := range bundles {
					Eventually(func() bool {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
							return false
						}
						if bundle.Status.Info == nil {
							return false
						}
						/*
							manifests
							├── 00_namespace.yaml
							├── 01_cluster_role.yaml
							├── 01_service_account.yaml
							├── 02_deployment.yaml
							├── 03_cluster_role_binding.yaml
							├── combo.io_combinations.yaml
							└── combo.io_templates.yaml
						*/
						return len(bundle.Status.Info.Objects) == 7
					}).Should(BeTrue())
				}
			})
		})
	})

	When("a bundle containing nested directry is created", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			ctx    context.Context
		)
		const (
			manifestsDir = "manifests"
			subdirName   = "emptydir"
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "namespace-subdirs",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:subdir",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())
		})
		It("reports an unpack error when the manifests directory contains directories", func() {
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundle), bundle); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.TypeUnpacked), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeUnpacked)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonUnpackFailed)),
					WithTransform(func(c *metav1.Condition) string { return c.Message },
						ContainSubstring(fmt.Sprintf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, subdirName)))),
				))
			})
		})
	})
})

var _ = Describe("plain provisioner bundleinstance", func() {
	When("a BundleInstance targets a valid Bundle", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			bi     *rukpakv1alpha1.BundleInstance
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds",
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
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())

			bi = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           bundle.GetName(),
				},
			}
			err = c.Create(ctx, bi)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, bundle))
			}).Should(Succeed())

			By("deleting the testing BundleInstance resource")
			Eventually(func() error {
				return c.Delete(ctx, bi)
			}).Should(Succeed())
		})

		It("should rollout the bundle contents successfully", func() {
			By("eventually writing a successful installation state back to the bundleinstance status")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return false
				}
				if bi.Status.InstalledBundleName != bundle.GetName() {
					return false
				}
				existing := meta.FindStatusCondition(bi.Status.Conditions, "Installed")
				if existing == nil {
					return false
				}
				expected := metav1.Condition{
					Type:    "Installed",
					Status:  metav1.ConditionStatus(corev1.ConditionTrue),
					Reason:  "InstallationSucceeded",
					Message: "",
				}
				return conditionsSemanticallyEqual(expected, *existing)
			}).Should(BeTrue())

			By("eventually reseting a bundle lookup failure when the targeted bundle has been deleted")
			Eventually(func() error {
				return c.Delete(ctx, bundle)
			}).Should(Succeed())

			By("eventually having a status indicating that the bundle lookup failed but installation succeeded")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return false
				}

				// Should still have the original InstalledBundleName in the status
				if bi.Status.InstalledBundleName == "" {
					return false
				}

				// Find the installed condition
				existingInstalledStatus := meta.FindStatusCondition(bi.Status.Conditions, "Installed")
				if existingInstalledStatus == nil {
					return false
				}
				expectedInstalledStatus := metav1.Condition{
					Type:    "Installed",
					Status:  metav1.ConditionStatus(corev1.ConditionTrue),
					Reason:  "InstallationSucceeded",
					Message: "",
				}

				// Find the HasValidBundle status
				existingBundleStatus := meta.FindStatusCondition(bi.Status.Conditions, "HasValidBundle")
				if existingBundleStatus == nil {
					return false
				}
				expectedBundleStatus := metav1.Condition{
					Type:    "HasValidBundle",
					Status:  metav1.ConditionStatus(corev1.ConditionFalse),
					Reason:  "BundleLookupFailed",
					Message: fmt.Sprintf(`Bundle.core.rukpak.io "%s" not found`, bundle.GetName()),
				}

				return conditionsSemanticallyEqual(expectedInstalledStatus, *existingInstalledStatus) &&
					conditionsSemanticallyEqual(expectedBundleStatus, *existingBundleStatus)
			}).Should(BeTrue())
		})
	})

	When("a BundleInstance targets an invalid Bundle", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			bi     *rukpakv1alpha1.BundleInstance
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-apis",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:invalid-missing-crds",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())

			bi = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-apis",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           bundle.GetName(),
				},
			}
			err = c.Create(ctx, bi)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, bundle))
			}).Should(Succeed())

			By("deleting the testing BundleInstance resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, bi))
			}).Should(Succeed())
		})

		It("should project a failed installation state", func() {
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return nil, err
				}
				if bi.Status.InstalledBundleName != "" {
					return nil, fmt.Errorf("bi.Status.InstalledBundleName is non-empty (%q)", bi.Status.InstalledBundleName)
				}
				return meta.FindStatusCondition(bi.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, And(
					// TODO(tflannag): Add a custom error type for API-based Bundle installations that
					// are missing the requisite CRDs to be able to deploy the unpacked Bundle successfully.
					ContainSubstring(`no matches for kind "CatalogSource" in version "operators.coreos.com/v1alpha1"`),
					ContainSubstring(`no matches for kind "ClusterServiceVersion" in version "operators.coreos.com/v1alpha1"`),
					ContainSubstring(`no matches for kind "OLMConfig" in version "operators.coreos.com/v1"`),
					ContainSubstring(`no matches for kind "OperatorGroup" in version "operators.coreos.com/v1"`),
				)),
			))
		})
	})

	When("a BundleInstance is dependent on another BundleInstance", func() {
		var (
			ctx             context.Context
			dependentBundle *rukpakv1alpha1.Bundle
			dependentBI     *rukpakv1alpha1.BundleInstance
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing dependent Bundle resource")
			dependentBundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-bundle-dependent-",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:dependent",
						},
					},
				},
			}
			err := c.Create(ctx, dependentBundle)
			Expect(err).To(BeNil())

			By("creating the testing dependent BundleInstance resource")
			dependentBI = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-bi-dependent-",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           dependentBundle.GetName(),
				},
			}
			err = c.Create(ctx, dependentBI)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing dependent Bundle resource")
			Expect(client.IgnoreNotFound(c.Delete(ctx, dependentBundle))).To(BeNil())

			By("deleting the testing dependent BundleInstance resource")
			Expect(client.IgnoreNotFound(c.Delete(ctx, dependentBI))).To(BeNil())

		})
		When("the providing BundleInstance does not exist", func() {
			It("should eventually project a failed installation for the dependent BundleInstance", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(dependentBI), dependentBI); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(dependentBI.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallFailed)),
					WithTransform(func(c *metav1.Condition) string { return c.Message },
						ContainSubstring(`unable to recognize "": no matches for kind "OperatorGroup" in version "operators.coreos.com/v1"`)),
				))
			})
		})
		When("the providing BundleInstance is created", func() {
			var (
				providesBundle *rukpakv1alpha1.Bundle
				providesBI     *rukpakv1alpha1.BundleInstance
			)
			BeforeEach(func() {
				ctx = context.Background()

				By("creating the testing providing Bundle resource")
				providesBundle = &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-bundle-providing-",
					},
					Spec: rukpakv1alpha1.BundleSpec{
						ProvisionerClassName: plainProvisionerID,
						Source: rukpakv1alpha1.BundleSource{
							Type: rukpakv1alpha1.SourceTypeImage,
							Image: &rukpakv1alpha1.ImageSource{
								Ref: "testdata/bundles/plain-v0:provides",
							},
						},
					},
				}
				err := c.Create(ctx, providesBundle)
				Expect(err).To(BeNil())

				By("creating the testing providing BI resource")
				providesBI = &rukpakv1alpha1.BundleInstance{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-bi-providing-",
					},
					Spec: rukpakv1alpha1.BundleInstanceSpec{
						ProvisionerClassName: plainProvisionerID,
						BundleName:           providesBundle.GetName(),
					},
				}
				err = c.Create(ctx, providesBI)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				By("deleting the testing providing Bundle resource")
				Expect(client.IgnoreNotFound(c.Delete(ctx, providesBundle))).To(BeNil())

				By("deleting the testing providing BundleInstance resource")
				Expect(client.IgnoreNotFound(c.Delete(ctx, providesBI))).To(BeNil())

			})
			It("should eventually project a successful installation for the dependent BundleInstance", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(dependentBI), dependentBI); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(dependentBI.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallationSucceeded)),
					WithTransform(func(c *metav1.Condition) string { return c.Message }, Equal("")),
				))
			})
		})
	})

	When("a BundleInstance targets a Bundle that contains CRDs and instances of those CRDs", func() {
		var (
			bundle *rukpakv1alpha1.Bundle
			bi     *rukpakv1alpha1.BundleInstance
			ctx    context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-bundle-crds-and-crs",
				},
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plainProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeImage,
						Image: &rukpakv1alpha1.ImageSource{
							Ref: "testdata/bundles/plain-v0:invalid-crds-and-crs",
						},
					},
				},
			}
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())

			By("creating the testing BI resource")
			bi = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-bi-crds-and-crs",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           bundle.GetName(),
				},
			}
			err = c.Create(ctx, bi)
			Expect(err).To(BeNil())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundle)
			Expect(err).To(BeNil())

			By("deleting the testing BI resource")
			err = c.Delete(ctx, bi)
			Expect(err).To(BeNil())
		})
		It("eventually reports a failed installation state due to missing APIs on the cluster", func() {
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bi.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`no matches for kind "CatalogSource" in version "operators.coreos.com/v1alpha1"`)),
			))
		})
	})

	When("a BundleInstance targets a valid bundle but the bundle contains resources that already exist", func() {
		var (
			bundle      *rukpakv1alpha1.Bundle
			biOriginal  *rukpakv1alpha1.BundleInstance
			biDuplicate *rukpakv1alpha1.BundleInstance
			ctx         context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-valid",
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
			err := c.Create(ctx, bundle)
			Expect(err).To(BeNil())

			biOriginal = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-apis-original",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           bundle.GetName(),
				},
			}

			err = c.Create(ctx, biOriginal)
			Expect(err).To(BeNil(), "failed to create original bundle instance")

			// ensure the original BI owns the underlying bundle before creating the duplicate
			By("projecting a successful installation status for the original BundleInstance")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(biOriginal), biOriginal); err != nil {
					return false
				}
				if biOriginal.Status.InstalledBundleName != bundle.GetName() {
					return false
				}
				existing := meta.FindStatusCondition(biOriginal.Status.Conditions, rukpakv1alpha1.TypeInstalled)
				if existing == nil {
					return false
				}
				expected := metav1.Condition{
					Type:   rukpakv1alpha1.TypeInstalled,
					Status: metav1.ConditionStatus(corev1.ConditionTrue),
					Reason: rukpakv1alpha1.ReasonInstallationSucceeded,
				}
				return conditionsSemanticallyEqual(*existing, expected)
			}).Should(BeTrue())

			biDuplicate = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-apis-duplicate",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           bundle.GetName(),
				},
			}

			err = c.Create(ctx, biDuplicate)
			Expect(err).To(BeNil(), "failed to create duplicate bundle instance")
		})

		AfterEach(func() {
			By("deleting the testing duplicate BundleInstance resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, biDuplicate))
			}).Should(Succeed())

			By("deleting the testing original BundleInstance resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, biOriginal))
			}).Should(Succeed())

			By("deleting the testing Bundle resource")
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, bundle))
			}).Should(Succeed())

		})

		It("should fail for the duplicate BundleInstance", func() {
			By("projecting a failed installation status for the duplicate BundleInstance")
			Eventually(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(biDuplicate), biDuplicate); err != nil {
					return false
				}
				if biDuplicate.Status.InstalledBundleName != "" {
					return false
				}
				existing := meta.FindStatusCondition(biDuplicate.Status.Conditions, rukpakv1alpha1.TypeInstalled)
				if existing == nil {
					return false
				}
				// Create what the message should contain
				looselyExpectedMessage := "rendered manifests contain a resource that already exists"
				expected := metav1.Condition{
					Type:    rukpakv1alpha1.TypeInstalled,
					Status:  metav1.ConditionStatus(corev1.ConditionFalse),
					Reason:  rukpakv1alpha1.ReasonInstallFailed,
					Message: looselyExpectedMessage,
				}

				return conditionsLooselyEqual(expected, *existing)
			}).Should(BeTrue())
		})
	})

	When("a BundleInstance is pivoted between Bundles that share a CRD", func() {
		var (
			ctx            context.Context
			originalBundle *rukpakv1alpha1.Bundle
			pivotedBundle  *rukpakv1alpha1.Bundle
			bi             *rukpakv1alpha1.BundleInstance
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the original testing Bundle resource")
			originalBundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-original-bundle-crd-pivoting",
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
			err := c.Create(ctx, originalBundle)
			Expect(err).To(BeNil())

			By("creating the pivoted testing Bundle resource")
			pivotedBundle = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-pivoted-bundle-crd-pivoting",
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
			err = c.Create(ctx, pivotedBundle)
			Expect(err).To(BeNil())

			By("creating the testing BI resource")
			bi = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-bi-crd-pivoting",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           originalBundle.GetName(),
				},
			}
			err = c.Create(ctx, bi)
			Expect(err).To(BeNil())

			By("waiting for the BI to eventually report a successful install status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bi.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, Equal("")),
			))
		})
		AfterEach(func() {
			By("deleting the original testing Bundle resource")
			err := c.Delete(ctx, originalBundle)
			Expect(err).To(BeNil())

			By("deleting the pivoted testing Bundle resource")
			err = c.Delete(ctx, pivotedBundle)
			Expect(err).To(BeNil())

			By("deleting the testing BI resource")
			err = c.Delete(ctx, bi)
			Expect(err).To(BeNil())
		})
		When("a custom resource is instantiated", func() {
			var (
				og *operatorsv1.OperatorGroup
				ns *corev1.Namespace
			)
			BeforeEach(func() {
				By("creating the testing Namespace resource")
				ns = &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-crd-pivoting",
					},
				}
				Expect(c.Create(ctx, ns)).To(BeNil())

				By("creating the testing OperatorGroup custom resource")
				og = &operatorsv1.OperatorGroup{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-catsrc-crd-pivoting",
						Namespace:    ns.GetName(),
					},
					Spec: operatorsv1.OperatorGroupSpec{},
				}
				Expect(c.Create(ctx, og)).To(BeNil())
			})
			AfterEach(func() {
				By("deleting the testing OperatorGroup resource")
				Expect(c.Delete(ctx, og)).To(BeNil())

				By("deleting the testing Namespace resource")
				Expect(c.Delete(ctx, ns)).To(BeNil())
			})
			It("should gracefully transfer ownership to the pivoted bundle", func() {
				By("pivoting the testing BI resource from the original Bundle to the new Bundle")
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
						return err
					}
					bi.Spec.BundleName = pivotedBundle.GetName()

					return c.Update(ctx, bi)
				}).Should(Succeed())

				By("ensuring that the OperatorGroup custom resource consistently exists after transferring ownership")
				Consistently(func() error {
					return c.Get(ctx, client.ObjectKeyFromObject(og), og)
				}).Should(Succeed())
			})
		})
	})
})

var _ = Describe("plain provisioner garbage collection", func() {
	When("a Bundle has been deleted", func() {
		var (
			ctx context.Context
			b   *rukpakv1alpha1.Bundle
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			b = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-ownerref-bundle-valid",
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
			Expect(c.Create(ctx, b)).To(BeNil())

			By("eventually reporting an Unpacked phase")
			Eventually(func() (string, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(b), b); err != nil {
					return "", err
				}
				return b.Status.Phase, nil
			}).Should(Equal(rukpakv1alpha1.PhaseUnpacked))
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Expect(c.Get(ctx, client.ObjectKeyFromObject(b), &rukpakv1alpha1.Bundle{})).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should result in the underlying bundle unpack pod being deleted", func() {
			By("deleting the test Bundle resource")
			Expect(c.Delete(ctx, b)).To(BeNil())

			By("waiting until all the configmaps have been deleted")
			selector := util.NewBundleLabelSelector(b)
			Eventually(func() bool {
				pods := &corev1.PodList{}
				if err := c.List(ctx, pods, &client.ListOptions{
					Namespace:     defaultSystemNamespace,
					LabelSelector: selector,
				}); err != nil {
					return false
				}
				return len(pods.Items) == 0
			}).Should(BeTrue())
		})
		It("should result in the underlying metadata/object configmaps being deleted", func() {
			By("deleting the test Bundle resource")
			Expect(c.Delete(ctx, b)).To(BeNil())

			By("waiting until all the configmaps have been deleted")
			selector := util.NewBundleLabelSelector(b)
			Eventually(func() bool {
				cms := &corev1.ConfigMapList{}
				if err := c.List(ctx, cms, &client.ListOptions{
					Namespace:     defaultSystemNamespace,
					LabelSelector: selector,
				}); err != nil {
					return false
				}
				return len(cms.Items) == 0
			}).Should(BeTrue())
		})
	})

	When("a BundleInstance has been deleted", func() {
		var (
			ctx context.Context
			b   *rukpakv1alpha1.Bundle
			bi  *rukpakv1alpha1.BundleInstance
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			b = &rukpakv1alpha1.Bundle{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-ownerref-bundle-valid",
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
			Expect(c.Create(ctx, b)).To(BeNil())

			By("creating the testing BI resource")
			bi = &rukpakv1alpha1.BundleInstance{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "e2e-ownerref-bi-valid",
				},
				Spec: rukpakv1alpha1.BundleInstanceSpec{
					ProvisionerClassName: plainProvisionerID,
					BundleName:           b.GetName(),
				},
			}
			Expect(c.Create(ctx, bi)).To(BeNil())

			By("waiting for the BI to eventually report a successful install status")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bi), bi); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bi.Status.Conditions, rukpakv1alpha1.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha1.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha1.ReasonInstallationSucceeded)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, Equal("")),
			))
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			Expect(c.Delete(ctx, b)).To(BeNil())

			By("deleting the testing BI resource")
			Expect(c.Get(ctx, client.ObjectKeyFromObject(bi), &rukpakv1alpha1.BundleInstance{})).To(WithTransform(apierrors.IsNotFound, BeTrue()))
		})
		It("should eventually result in the installed CRDs being deleted", func() {
			By("deleting the testing BI resource")
			Expect(c.Delete(ctx, bi)).To(BeNil())

			By("waiting until all the installed CRDs have been deleted")
			selector := util.NewBundleInstanceLabelSelector(bi)
			Eventually(func() bool {
				crds := &apiextensionsv1.CustomResourceDefinitionList{}
				if err := c.List(ctx, crds, &client.ListOptions{
					LabelSelector: selector,
				}); err != nil {
					return false
				}
				return len(crds.Items) == 0
			}).Should(BeTrue())
		})
	})
})

func conditionsSemanticallyEqual(a, b metav1.Condition) bool {
	return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && a.Message == b.Message
}

func conditionsLooselyEqual(a, b metav1.Condition) bool {
	return a.Type == b.Type && a.Status == b.Status && a.Reason == b.Reason && strings.Contains(b.Message, a.Message)
}
