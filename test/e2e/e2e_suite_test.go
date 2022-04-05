package e2e

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	c client.Client
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(1 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	config := ctrl.GetConfigOrDie()

	scheme := runtime.NewScheme()
	err := rukpakv1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	err = corev1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	err = apiextensionsv1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	c, err = client.New(config, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())
})
