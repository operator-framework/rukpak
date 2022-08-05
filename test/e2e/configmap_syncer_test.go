package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ConfigMapSyncer", func() {
	ctx := context.Background()
	It("should populate rukpak-ca configmap", func() {
		By("fetching rukpak-ca secret")
		secret := &corev1.Secret{}
		Eventually(func() (map[string][]byte, error) {
			err := c.Get(ctx, types.NamespacedName{Namespace: defaultSystemNamespace, Name: "rukpak-ca"}, secret)
			return secret.Data, err
		}).Should(And(
			HaveKey("ca.crt"),
			HaveKey("tls.crt"),
			HaveKey("tls.key"),
		))

		By("fetching rukpak-ca configmap")
		cm := &corev1.ConfigMap{}
		Eventually(func() (map[string]string, error) {
			err := c.Get(ctx, types.NamespacedName{Namespace: defaultSystemNamespace, Name: defaultCAConfigMapName}, cm)
			return cm.Data, err
		}).Should(HaveKey("ca-bundle.crt"))

		By("comparing expected injected value")
		Expect(string(secret.Data["tls.crt"])).To(Equal(cm.Data["ca-bundle.crt"]))
	})
})
