package bundle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	plainv0 "github.com/operator-framework/rukpak/pkg/bundle/plain/v0"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
	"github.com/operator-framework/rukpak/test/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Bundle", func() {
	Describe("converting a registry+v1 bundle", func() {
		var (
			srcBundle registryv1.Bundle
			opts      []func(*plainv0.Bundle)
		)

		BeforeEach(func() {
			srcBundle = registryv1.New(testutil.NewRegistryV1FS())
		})

		var bundle *plainv0.Bundle

		JustBeforeEach(func() {
			var err error
			bundle, err = plainv0.FromRegistryV1(srcBundle, opts...)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return a plain+v0 bundle", func() {
			Expect(bundle).ToNot(BeNil())
		})

		It("should have the deployment from the CSV", func() {
			By("finding all the deployments")
			deps, err := bundle.Objects(func(obj client.Object) bool {
				_, ok := obj.(*appsv1.Deployment)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(deps).To(HaveLen(1))
			Expect(deps[0].GetName()).To(Equal("memcached-operator-controller-manager"))

			By("finding the service account for the deployment")
			svcAccs, err := bundle.Objects(func(obj client.Object) bool {
				svcAcc, ok := obj.(*corev1.ServiceAccount)
				if !ok {
					return false
				}

				return svcAcc.GetName() == deps[0].(*appsv1.Deployment).Spec.Template.Spec.ServiceAccountName
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(svcAccs).To(HaveLen(1))
		})

		It("should have all the RBAC from the CSV", func() {
			By("extracting the roles")
			roles, err := bundle.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.Role)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(HaveLen(1))

			By("extracting the role bindings")
			roleBindings, err := bundle.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.RoleBinding)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roleBindings).To(HaveLen(1))
		})

		It("should have all the cluster RBAC from the CSV", func() {
			By("extracting the cluster roles")
			roles, err := bundle.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.ClusterRole)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(HaveLen(2))

			By("extracting the cluster role bindings")
			roleBindings, err := bundle.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.ClusterRoleBinding)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roleBindings).To(HaveLen(1))
		})
	})
})
