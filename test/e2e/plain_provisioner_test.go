package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	defaultSystemNamespace = util.DefaultSystemNamespace
	testdataDir            = "../../testdata"
)

func Logf(f string, v ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	fmt.Fprintf(GinkgoWriter, f, v...)
}

var _ = Describe("plain provisioner bundle", func() {
	When("a valid Bundle references the wrong unique provisioner ID", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-valid",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: "non-existent-class-name",
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:valid"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		It("should consistently contain an empty status", func() {
			Consistently(func() bool {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
					return false
				}
				return len(bundleDeployment.Status.Conditions) == 0
			}, 10*time.Second, 1*time.Second).Should(BeTrue())
		})
	})
	When("a valid Bundle Deployment referencing a remote container image is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-valid",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:valid"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should eventually report a successful state", func() {

			By("eventually writing a non-empty image digest to the status", func() {
				Eventually(func() (*rukpakv1alpha2.BundleSource, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return nil, err
					}
					return bundleDeployment.Status.ResolvedSource, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(s *rukpakv1alpha2.BundleSource) rukpakv1alpha2.SourceType { return s.Type }, Equal(rukpakv1alpha2.SourceTypeImage)),
					WithTransform(func(s *rukpakv1alpha2.BundleSource) *rukpakv1alpha2.ImageSource { return s.Image }, And(
						Not(BeNil()),
						WithTransform(func(i *rukpakv1alpha2.ImageSource) string { return i.Ref }, Not(Equal(""))),
					)),
				))
			})
		})

		It("should re-create underlying system resources", func() {
			var (
				pod *corev1.Pod
			)

			By("getting the underlying bundle unpacking pod")
			selector := util.NewBundleDeploymentLabelSelector(bundleDeployment)
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
			Expect(err).ToNot(HaveOccurred())

			By("verifying the pod's UID has changed")
			Eventually(func() (types.UID, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(pod), pod)
				return pod.GetUID(), err
			}).ShouldNot(Equal(originalUID))
		})
	})

	When("a valid Bundle Deployment referencing a remote private container image is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-valid",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref:                 "docker-registry.rukpak-e2e.svc.cluster.local:5000/bundles/plain-v0:valid",
							ImagePullSecretName: "registrysecret",
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should eventually report a successful state", func() {
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
					WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
				))
			})

			By("eventually writing a non-empty image digest to the status", func() {
				Eventually(func() (*rukpakv1alpha2.BundleSource, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return nil, err
					}
					return bundleDeployment.Status.ResolvedSource, nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(s *rukpakv1alpha2.BundleSource) rukpakv1alpha2.SourceType { return s.Type }, Equal(rukpakv1alpha2.SourceTypeImage)),
					WithTransform(func(s *rukpakv1alpha2.BundleSource) *rukpakv1alpha2.ImageSource { return s.Image }, And(
						Not(BeNil()),
						WithTransform(func(i *rukpakv1alpha2.ImageSource) string { return i.Ref }, Not(Equal(""))),
					)),
				))
			})
		})
	})

	When("an invalid Bundle Deployment referencing a remote container image is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-invalid",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:non-existent-tag"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		It("checks the bundle's phase is stuck in pending", func() {
			By("waiting until the pod is reporting ImagePullBackOff state")
			Eventually(func() bool {
				pod := &corev1.Pod{}
				if err := c.Get(ctx, types.NamespacedName{
					Name:      bundleDeployment.GetName(),
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
				err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment)
				if err != nil {
					return false
				}
				unpackPending := meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.PhaseUnpacked)
				if unpackPending == nil {
					return false
				}
				if unpackPending.Message != fmt.Sprintf(`Back-off pulling image "%s"`, bundleDeployment.Spec.Source.Image.Ref) {
					return false
				}
				return true
			}).Should(BeTrue())
		})
	})

	When("a bundle deployment containing no manifests is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-unsupported",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:empty"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		It("reports an unpack error when the manifests directory is missing", func() {
			By("waiting for the bundle to report back that state")
			Eventually(func() (*metav1.Condition, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment)
				if err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`readdir manifests: file does not exist`)),
			))
		})
	})

	When("a bundle containing an empty manifests directory is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "olm-crds-unsupported",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:no-manifests"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		It("reports an unpack error when the manifests directory contains no objects", func() {
			By("waiting for the bundle to report back that state")
			Eventually(func() (*metav1.Condition, error) {
				err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment)
				if err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`found zero objects: plain+v0 bundles are required to contain at least one object`)),
			))
		})
	})

	When("Bundles are backed by a git repository", func() {
		var (
			ctx context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
		})

		When("the bundle is backed by a git commit", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-commit",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: "https://github.com/exdx/combo-bundle",
								Ref: rukpakv1alpha2.GitRef{
									Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
								},
							},
						},
					},
				}
				err := c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return err
					}

					provisionerPods := &corev1.PodList{}
					if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
						return err
					}
					if len(provisionerPods.Items) != 1 {
						return errors.New("expected exactly 1 provisioner pod")
					}

					return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
				}).Should(BeNil())
			})
		})
		When("the bundle deployment is backed by a git tag", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-tag",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: "https://github.com/exdx/combo-bundle",
								Ref: rukpakv1alpha2.GitRef{
									Tag: "v0.0.1",
								},
							},
						},
					},
				}
				err := c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				By("eventually unpacking the bundle", func() {
					Eventually(func() error {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
							return err
						}

						provisionerPods := &corev1.PodList{}
						if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
							return err
						}
						if len(provisionerPods.Items) != 1 {
							return errors.New("expected exactly 1 provisioner pod")
						}

						return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
					}).Should(BeNil())
				})

				By("eventually writing a non-empty commit hash to the status", func() {
					Eventually(func() (*rukpakv1alpha2.BundleSource, error) {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
							return nil, err
						}
						return bundleDeployment.Status.ResolvedSource, nil
					}).Should(And(
						Not(BeNil()),
						WithTransform(func(s *rukpakv1alpha2.BundleSource) rukpakv1alpha2.SourceType { return s.Type }, Equal(rukpakv1alpha2.SourceTypeGit)),
						WithTransform(func(s *rukpakv1alpha2.BundleSource) *rukpakv1alpha2.GitSource { return s.Git }, And(
							Not(BeNil()),
							WithTransform(func(i *rukpakv1alpha2.GitSource) string { return i.Ref.Commit }, Not(Equal(""))),
						)),
					))
				})
			})
		})

		When("the bundle deployment is backed by a git branch", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-branch",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: "https://github.com/exdx/combo-bundle.git",
								Ref: rukpakv1alpha2.GitRef{
									Branch: "main",
								},
							},
						},
					},
				}
				err := c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				By("eventually unpacking the bundle", func() {
					Eventually(func() error {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
							return err
						}

						provisionerPods := &corev1.PodList{}
						if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
							return err
						}
						if len(provisionerPods.Items) != 1 {
							return errors.New("expected exactly 1 provisioner pod")
						}

						return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
					}).Should(BeNil())
				})

				By("eventually writing a non-empty commit hash to the status", func() {
					Eventually(func() (*rukpakv1alpha2.BundleSource, error) {
						if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
							return nil, err
						}
						return bundleDeployment.Status.ResolvedSource, nil
					}).Should(And(
						Not(BeNil()),
						WithTransform(func(s *rukpakv1alpha2.BundleSource) rukpakv1alpha2.SourceType { return s.Type }, Equal(rukpakv1alpha2.SourceTypeGit)),
						WithTransform(func(s *rukpakv1alpha2.BundleSource) *rukpakv1alpha2.GitSource { return s.Git }, And(
							Not(BeNil()),
							WithTransform(func(i *rukpakv1alpha2.GitSource) string { return i.Ref.Commit }, Not(Equal(""))),
						)),
					))
				})
			})
		})

		When("the bundle deployment has a custom manifests directory", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-custom-dir",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: "https://github.com/exdx/combo-bundle",
								Directory:  "./dev/deploy",
								Ref: rukpakv1alpha2.GitRef{
									Branch: "main",
								},
							},
						},
					},
				}
				err := c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return err
					}

					provisionerPods := &corev1.PodList{}
					if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
						return err
					}
					if len(provisionerPods.Items) != 1 {
						return errors.New("expected exactly 1 provisioner pod")
					}

					return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
				}).Should(BeNil())
			})
		})

		When("the bundle deployment is backed by a private repository", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
				secret           *corev1.Secret
				privateRepo      string
			)
			BeforeEach(func() {
				privateRepo = os.Getenv("PRIVATE_GIT_REPO")
				username := os.Getenv("PRIVATE_REPO_USERNAME")
				password := os.Getenv("PRIVATE_REPO_PASSWORD")
				if privateRepo == "" {
					Skip("Private repository information is not set.")
				}
				Expect(privateRepo[:4]).To(Equal("http"))

				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "gitsecret-",
						Namespace:    defaultSystemNamespace,
					},
					Data: map[string][]byte{"username": []byte(username), "password": []byte(password)},
					Type: "Opaque",
				}
				err := c.Create(ctx, secret)
				Expect(err).ToNot(HaveOccurred())
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-branch",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: privateRepo,
								Ref: rukpakv1alpha2.GitRef{
									Branch: "main",
								},
								Auth: rukpakv1alpha2.Authorization{
									Secret: corev1.LocalObjectReference{
										Name: secret.Name,
									},
								},
							},
						},
					},
				}
				err = c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
				err = c.Delete(ctx, secret)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return err
					}

					provisionerPods := &corev1.PodList{}
					if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
						return err
					}
					if len(provisionerPods.Items) != 1 {
						return errors.New("expected exactly 1 provisioner pod")
					}

					return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
				}).Should(BeNil())
			})
		})

		When("the bundle deployment is backed by a local git repository", func() {
			var (
				bundleDeployment *rukpakv1alpha2.BundleDeployment
				privateRepo      string
			)
			BeforeEach(func() {
				privateRepo = "ssh://git@local-git.rukpak-e2e.svc.cluster.local:2222/git-server/repos/combo"
				bundleDeployment = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "combo-git-branch",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeGit,
							Git: &rukpakv1alpha2.GitSource{
								Repository: privateRepo,
								Ref: rukpakv1alpha2.GitRef{
									Branch: "main",
								},
								Auth: rukpakv1alpha2.Authorization{
									Secret: corev1.LocalObjectReference{
										Name: "gitsecret",
									},
									InsecureSkipVerify: true,
								},
							},
						},
					},
				}
				err := c.Create(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				err := c.Delete(ctx, bundleDeployment)
				Expect(err).ToNot(HaveOccurred())
			})

			It("Can create and unpack the bundle successfully", func() {
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return err
					}

					provisionerPods := &corev1.PodList{}
					if err := c.List(context.Background(), provisionerPods, client.MatchingLabels{"app": "core"}); err != nil {
						return err
					}
					if len(provisionerPods.Items) != 1 {
						return errors.New("expected exactly 1 provisioner pod")
					}

					return checkProvisionerBundleDeployment(ctx, bundleDeployment, provisionerPods.Items[0].Name)
				}).Should(BeNil())
			})
		})
	})

	When("the bundle deployment is backed by a configmap", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			configmap        *corev1.ConfigMap
			ctx              context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()

			data := map[string]string{}
			err := filepath.Walk(filepath.Join(testdataDir, "bundles/plain-v0/valid/manifests"), func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				c, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				data[info.Name()] = string(c)
				return nil
			})
			Expect(err).ToNot(HaveOccurred())
			configmap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "bundle-configmap-valid-",
					Namespace:    defaultSystemNamespace,
				},
				Data:      data,
				Immutable: pointer.Bool(true),
			}
			err = c.Create(ctx, configmap)
			Expect(err).ToNot(HaveOccurred())
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "combo-local-",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeConfigMaps,
						ConfigMaps: []rukpakv1alpha2.ConfigMapSource{{
							ConfigMap: corev1.LocalObjectReference{Name: configmap.ObjectMeta.Name},
							Path:      "manifests",
						}},
					},
				},
			}
			err = c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(c.Delete(ctx, bundleDeployment))).To(Succeed())
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, configmap))
			}).Should(Succeed())
		})

		It("Can create and unpack the bundle successfully", func() {
			Eventually(func() error {
				return c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment)
			}).Should(BeNil())
		})
	})

	When("the bundle is backed by a non-existent configmap", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "combo-local-",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeConfigMaps,
						ConfigMaps: []rukpakv1alpha2.ConfigMapSource{{
							ConfigMap: corev1.LocalObjectReference{Name: "non-exist"},
							Path:      "manifests",
						}},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err := c.Delete(ctx, bundleDeployment)
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
		It("eventually results in a failing bundle state", func() {
			By("waiting until the bundle is reporting Failing state")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeUnpacked), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeUnpacked)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonUnpackFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message },
					ContainSubstring(fmt.Sprintf("source bundle content: get configmap %[1]s/%[2]s: ConfigMap %[2]q not found", defaultSystemNamespace, "non-exist"))),
			))
		})
	})

	When("the bundle is backed by an invalid configmap", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			configmap        *corev1.ConfigMap
			ctx              context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
			data := map[string]string{}
			err := filepath.Walk(filepath.Join(testdataDir, "bundles/plain-v0/subdir/manifests"), func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				c, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				data[info.Name()] = string(c)
				return nil
			})
			Expect(err).ToNot(HaveOccurred())
			configmap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "bundle-configmap-invalid-",
					Namespace:    defaultSystemNamespace,
				},
				Data:      data,
				Immutable: pointer.Bool(true),
			}
			err = c.Create(ctx, configmap)
			Expect(err).ToNot(HaveOccurred())
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "combo-local-",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeConfigMaps,
						ConfigMaps: []rukpakv1alpha2.ConfigMapSource{{
							ConfigMap: corev1.LocalObjectReference{Name: configmap.ObjectMeta.Name},
							Path:      "manifests",
						}},
					},
				},
			}
			err = c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(c.Delete(ctx, bundleDeployment))).To(Succeed())
			Eventually(func() error {
				return client.IgnoreNotFound(c.Delete(ctx, configmap))
			}).Should(Succeed())
		})
		It("checks the bundle's phase gets failing", func() {
			By("waiting until the bundle is reporting Failing state")
			Eventually(func() (*metav1.Condition, error) {
				if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
					return nil, err
				}
				return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
			}).Should(And(
				Not(BeNil()),
				WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
				WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
				WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
				WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("json: cannot unmarshal string into Go value")),
			))
		})
	})

	When("a bundle deployment containing nested directory is created", func() {
		var (
			bundleDeployment *rukpakv1alpha2.BundleDeployment
			ctx              context.Context
		)
		const (
			manifestsDir = "manifests"
			subdirName   = "emptydir"
		)
		BeforeEach(func() {
			ctx = context.Background()

			By("creating the testing Bundle resource")
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "namespace-subdirs",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeImage,
						Image: &rukpakv1alpha2.ImageSource{
							Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:subdir"),
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			By("deleting the testing Bundle resource")
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		It("reports an unpack error when the manifests directory contains directories", func() {
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
					WithTransform(func(c *metav1.Condition) string { return c.Message },
						ContainSubstring(fmt.Sprintf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, subdirName)))),
				))
			})
		})
	})

	When("valid  bundle is created", func() {
		var (
			ctx              context.Context
			bundleDeployment *rukpakv1alpha2.BundleDeployment
		)
		BeforeEach(func() {
			ctx = context.Background()
			bundleDeployment = &rukpakv1alpha2.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "combo-git-commit",
				},
				Spec: rukpakv1alpha2.BundleDeploymentSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha2.BundleSource{
						Type: rukpakv1alpha2.SourceTypeGit,
						Git: &rukpakv1alpha2.GitSource{
							Repository: "https://github.com/exdx/combo-bundle",
							Ref: rukpakv1alpha2.GitRef{
								Commit: "9e3ab7f1a36302ef512294d5c9f2e9b9566b811e",
							},
						},
					},
				},
			}
			err := c.Create(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
			By("eventually reporting an Unpacked phase", func() {
				Eventually(func() (*metav1.Condition, error) {
					if err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment); err != nil {
						return nil, err
					}
					return meta.FindStatusCondition(bundleDeployment.Status.Conditions, rukpakv1alpha2.TypeUnpacked), nil
				}).Should(And(
					Not(BeNil()),
					WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeUnpacked)),
					WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
					WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonUnpackSuccessful)),
				))
			})

			By("eventually writing a content URL to the status", func() {
				Eventually(func() (string, error) {
					err := c.Get(ctx, client.ObjectKeyFromObject(bundleDeployment), bundleDeployment)
					Expect(err).ToNot(HaveOccurred())
					return bundleDeployment.Status.ContentURL, nil
				}).Should(Not(BeEmpty()))
			})
		})
		AfterEach(func() {
			err := c.Delete(ctx, bundleDeployment)
			Expect(err).ToNot(HaveOccurred())
		})
		When("start server for bundle contents", func() {
			var (
				sa  corev1.ServiceAccount
				crb rbacv1.ClusterRoleBinding
				job batchv1.Job
				pod corev1.Pod
			)
			BeforeEach(func() {
				// Create a temporary ServiceAccount
				sa = corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rukpak-svr-sa",
						Namespace: defaultSystemNamespace,
					},
				}
				err := c.Create(ctx, &sa)
				Expect(err).ToNot(HaveOccurred())

				// Create a temporary ClusterRoleBinding to bind the ServiceAccount to bundle-reader ClusterRole
				crb = rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rukpak-svr-crb",
						Namespace: defaultSystemNamespace,
					},

					Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "rukpak-svr-sa", Namespace: defaultSystemNamespace}},
					RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "bundle-reader"},
				}

				err = c.Create(ctx, &crb)
				Expect(err).ToNot(HaveOccurred())
				url := bundleDeployment.Status.ContentURL

				// Create a Job that reads from the URL and outputs contents in the pod log
				mounttoken := true
				job = batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rukpak-svr-job",
						Namespace: defaultSystemNamespace,
					},
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:    "rukpak-svr",
										Image:   "curlimages/curl",
										Command: []string{"sh", "-c", "curl -sSLk -H \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" -o - " + url + " | tar ztv"},
									},
								},
								ServiceAccountName:           "rukpak-svr-sa",
								RestartPolicy:                "Never",
								AutomountServiceAccountToken: &mounttoken,
							},
						},
					},
				}
				err = c.Create(ctx, &job)
				Expect(err).ToNot(HaveOccurred())
				Eventually(func() (bool, error) {
					err = c.Get(ctx, types.NamespacedName{Name: "rukpak-svr-job", Namespace: defaultSystemNamespace}, &job)
					if err != nil {
						return false, err
					}
					return job.Status.CompletionTime != nil && job.Status.Succeeded == 1, err
				}).Should(BeTrue())
			})
			AfterEach(func() {
				Eventually(func() bool {
					errJob := c.Delete(ctx, &job)
					errPod := c.Delete(ctx, &pod)
					errCrb := c.Delete(ctx, &crb)
					errSa := c.Delete(ctx, &sa)
					return client.IgnoreNotFound(errJob) == nil && client.IgnoreNotFound(errPod) == nil && client.IgnoreNotFound(errCrb) == nil && client.IgnoreNotFound(errSa) == nil
				}).Should(BeTrue())
			})
			It("reads the pod log", func() {
				// Get Pod for the Job
				pods := &corev1.PodList{}
				Eventually(func() (bool, error) {
					err := c.List(context.Background(), pods, client.MatchingLabels{"job-name": "rukpak-svr-job"})
					if err != nil {
						return false, err
					}
					return len(pods.Items) == 1, nil
				}).Should(BeTrue())

				Eventually(func() (bool, error) {
					// Get logs of the Pod
					pod = pods.Items[0]
					logReader, err := kubeClient.CoreV1().Pods(defaultSystemNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(context.Background())
					if err != nil {
						return false, err
					}
					buf := new(bytes.Buffer)
					_, err = buf.ReadFrom(logReader)
					Expect(err).ToNot(HaveOccurred())
					return strings.Contains(buf.String(), "manifests/00_namespace.yaml") &&
						strings.Contains(buf.String(), "manifests/01_cluster_role.yaml") &&
						strings.Contains(buf.String(), "manifests/01_service_account.yaml") &&
						strings.Contains(buf.String(), "manifests/02_deployment.yaml") &&
						strings.Contains(buf.String(), "manifests/03_cluster_role_binding.yaml") &&
						strings.Contains(buf.String(), "manifests/combo.io_combinations.yaml") &&
						strings.Contains(buf.String(), "manifests/combo.io_templates.yaml"), nil
				}).Should(BeTrue())
			})
		})
	})

	var _ = Describe("plain provisioner bundleDeployment", func() {
		When("a BundleDeployment is dependent on another BundleDeployment", func() {
			var (
				ctx         context.Context
				dependentBD *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				ctx = context.Background()
				By("creating the testing dependent BundleDeployment resource")
				dependentBD = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-bd-dependent-",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeImage,
							Image: &rukpakv1alpha2.ImageSource{
								Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:dependent"),
							},
						},
					},
				}
				err := c.Create(ctx, dependentBD)
				Expect(err).ToNot(HaveOccurred())
			})
			AfterEach(func() {
				By("deleting the testing dependent BundleDeployment resource")
				Expect(client.IgnoreNotFound(c.Delete(ctx, dependentBD))).To(Succeed())

			})
			When("the providing BundleDeployment does not exist", func() {
				It("should eventually project a failed installation for the dependent BundleDeployment", func() {
					Eventually(func() (*metav1.Condition, error) {
						if err := c.Get(ctx, client.ObjectKeyFromObject(dependentBD), dependentBD); err != nil {
							return nil, err
						}
						return meta.FindStatusCondition(dependentBD.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
					}).Should(And(
						Not(BeNil()),
						WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
						WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionFalse)),
						WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallFailed)),
						WithTransform(func(c *metav1.Condition) string { return c.Message },
							ContainSubstring(`the server could not find the requested resource`)),
					))
				})
			})
			When("the providing BundleDeployment is created", func() {
				var (
					providesBD *rukpakv1alpha2.BundleDeployment
				)
				BeforeEach(func() {
					ctx = context.Background()

					By("creating the testing providing BD resource")
					providesBD = &rukpakv1alpha2.BundleDeployment{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "e2e-bd-providing-",
						},
						Spec: rukpakv1alpha2.BundleDeploymentSpec{
							ProvisionerClassName: plain.ProvisionerID,
							Source: rukpakv1alpha2.BundleSource{
								Type: rukpakv1alpha2.SourceTypeImage,
								Image: &rukpakv1alpha2.ImageSource{
									Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:provides"),
								},
							},
						},
					}
					err := c.Create(ctx, providesBD)
					Expect(err).ToNot(HaveOccurred())
				})
				AfterEach(func() {
					By("deleting the testing providing BundleDeployment resource")
					Expect(client.IgnoreNotFound(c.Delete(ctx, providesBD))).To(Succeed())

				})
				It("should eventually project a successful installation for the dependent BundleDeployment", func() {
					Eventually(func() (*metav1.Condition, error) {
						if err := c.Get(ctx, client.ObjectKeyFromObject(dependentBD), dependentBD); err != nil {
							return nil, err
						}
						return meta.FindStatusCondition(dependentBD.Status.Conditions, rukpakv1alpha2.TypeInstalled), nil
					}).Should(And(
						Not(BeNil()),
						WithTransform(func(c *metav1.Condition) string { return c.Type }, Equal(rukpakv1alpha2.TypeInstalled)),
						WithTransform(func(c *metav1.Condition) metav1.ConditionStatus { return c.Status }, Equal(metav1.ConditionTrue)),
						WithTransform(func(c *metav1.Condition) string { return c.Reason }, Equal(rukpakv1alpha2.ReasonInstallationSucceeded)),
						WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring("Instantiated bundle")),
					))
				})
			})
		})

		When("a BundleDeployment targets a Bundle that contains CRDs and instances of those CRDs", func() {
			var (
				bd  *rukpakv1alpha2.BundleDeployment
				ctx context.Context
			)
			BeforeEach(func() {
				ctx = context.Background()

				By("creating the testing BD resource")
				bd = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-bd-crds-and-crs-",
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeImage,
							Image: &rukpakv1alpha2.ImageSource{
								Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:invalid-crds-and-crs"),
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
			It("eventually reports a failed installation state due to missing APIs on the cluster", func() {
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
					WithTransform(func(c *metav1.Condition) string { return c.Message }, ContainSubstring(`the server could not find the requested resource`)),
				))
			})
		})
	})

	var _ = Describe("plain provisioner garbage collection", func() {

		When("a BundleDeployment has been deleted", func() {
			var (
				ctx context.Context
				bd  *rukpakv1alpha2.BundleDeployment
			)
			BeforeEach(func() {
				ctx = context.Background()

				By("creating the testing BD resource")
				bd = &rukpakv1alpha2.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "e2e-ownerref-bd-valid-",
						Labels: map[string]string{
							"app.kubernetes.io/name": "e2e-ownerref-bundle-valid",
						},
					},
					Spec: rukpakv1alpha2.BundleDeploymentSpec{
						ProvisionerClassName: plain.ProvisionerID,
						Source: rukpakv1alpha2.BundleSource{
							Type: rukpakv1alpha2.SourceTypeImage,
							Image: &rukpakv1alpha2.ImageSource{
								Ref: fmt.Sprintf("%v/%v", ImageRepo, "plain-v0:valid"),
							},
						},
					},
				}
				Expect(c.Create(ctx, bd)).To(Succeed())

				By("waiting for the BD to eventually report a successful install status")
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
			AfterEach(func() {
				By("deleting the testing BD resource")
				Expect(c.Get(ctx, client.ObjectKeyFromObject(bd), &rukpakv1alpha2.BundleDeployment{})).To(WithTransform(apierrors.IsNotFound, BeTrue()))
			})
			It("should eventually result in the installed CRDs being deleted", func() {
				By("deleting the testing BD resource")
				Expect(c.Delete(ctx, bd)).To(Succeed())

				By("waiting until all the installed CRDs have been deleted")
				selector := util.NewBundleDeploymentLabelSelector(bd)
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
})

func checkProvisionerBundleDeployment(ctx context.Context, object client.Object, provisionerPodName string) error {
	req := kubeClient.CoreV1().RESTClient().Post().
		Namespace(defaultSystemNamespace).
		Resource("pods").
		Name(provisionerPodName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "manager",
			Command:   []string{"ls", filepath.Join(storage.DefaultBundleCacheDir, fmt.Sprintf("%s.tgz", object.GetName()))},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, runtime.NewParameterCodec(c.Scheme()))

	exec, err := remotecommand.NewSPDYExecutor(cfg, http.MethodPost, req.URL())
	if err != nil {
		return err
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: io.Discard,
		Stderr: io.Discard,
		Tty:    false,
	})
}
