package e2e_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/rukpak/api/v1alpha1"
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
	err := v1alpha1.AddToScheme(scheme)
	Expect(err).To(BeNil())

	c, err = client.New(config, client.Options{Scheme: scheme})
	Expect(err).To(BeNil())
})
