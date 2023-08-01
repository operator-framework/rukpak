package bundledeployment

import (
	"bytes"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/postrender"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

func TestBundleDeploymentController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BundleDeployment Controller Suite")
}

var _ = Describe("BundleDeployment", func() {
	var _ = Describe("PostRenderer", func() {
		var (
			postren postrender.PostRenderer
			pod     corev1.Pod
			inBuf   bytes.Buffer
		)

		Context("should add labels defined in the postrenderer", func() {
			BeforeEach(func() {
				postren = &postrenderer{
					labels: map[string]string{
						util.CoreOwnerKindKey: rukpakv1alpha1.BundleDeploymentKind,
						util.CoreOwnerNameKey: "test-owner",
					},
				}

				pod = corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "testPod",
						Labels: map[string]string{
							"testKey": "testValue",
						},
					},
				}
				pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})

			})

			It("when object has no labels", func() {
				pod.SetLabels(nil)

				By("encoding pod")
				Expect(json.NewEncoder(&inBuf).Encode(pod)).NotTo(HaveOccurred())

				By("verifying if postrender ran successfully")
				outBuf, err := postren.Run(&inBuf)
				Expect(err).NotTo(HaveOccurred())
				Expect(outBuf.String()).NotTo(BeEmpty())

				By("converting output string back to pod")
				renderedPod := &corev1.Pod{}
				Expect(json.Unmarshal(outBuf.Bytes(), renderedPod)).NotTo(HaveOccurred())
				Expect(renderedPod).NotTo(BeNil())

				labels := renderedPod.GetLabels()
				Expect(len(labels)).To(BeEquivalentTo(2))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-kind", "BundleDeployment"))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-name", "test-owner"))
			})

			It("when object's label has an empty map", func() {
				pod.SetLabels(map[string]string{})

				By("encoding pod")
				Expect(json.NewEncoder(&inBuf).Encode(pod)).NotTo(HaveOccurred())

				By("verifying if postrender ran successfully")
				outBuf, err := postren.Run(&inBuf)
				Expect(err).NotTo(HaveOccurred())
				Expect(outBuf.String()).NotTo(BeEmpty())

				By("converting output string back to pod")
				renderedPod := &corev1.Pod{}
				Expect(json.Unmarshal(outBuf.Bytes(), renderedPod)).NotTo(HaveOccurred())
				Expect(renderedPod).NotTo(BeNil())

				labels := renderedPod.GetLabels()
				Expect(len(labels)).To(BeEquivalentTo(2))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-kind", "BundleDeployment"))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-name", "test-owner"))
			})

			It("when object has custom labels", func() {
				By("encoding pod")
				Expect(json.NewEncoder(&inBuf).Encode(pod)).NotTo(HaveOccurred())

				By("verifying if postrender ran successfully")
				outBuf, err := postren.Run(&inBuf)
				Expect(err).NotTo(HaveOccurred())
				Expect(outBuf.String()).NotTo(BeEmpty())

				By("converting output string back to pod")
				renderedPod := &corev1.Pod{}
				Expect(json.Unmarshal(outBuf.Bytes(), renderedPod)).NotTo(HaveOccurred())
				Expect(renderedPod).NotTo(BeNil())

				labels := renderedPod.GetLabels()
				Expect(len(labels)).To(BeEquivalentTo(3))
				Expect(labels).Should(HaveKeyWithValue("testKey", "testValue"))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-kind", "BundleDeployment"))
				Expect(labels).Should(HaveKeyWithValue("core.rukpak.io/owner-name", "test-owner"))
			})

		})
	})
})
