package e2e

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

var (
	cfg        *rest.Config
	c          client.Client
	kubeClient kubernetes.Interface
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(1 * time.Minute)
	SetDefaultEventuallyPollingInterval(1 * time.Second)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	cfg = ctrl.GetConfigOrDie()

	scheme := runtime.NewScheme()
	Expect(rukpakv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(rbacv1.AddToScheme(scheme)).To(Succeed())
	Expect(batchv1.AddToScheme(scheme)).To(Succeed())
	Expect(operatorsv1.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())

	var err error
	c, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
})
