package v0_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/pkg/bundle"
	plainv0 "github.com/operator-framework/rukpak/pkg/bundle/plain/v0"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
	"github.com/operator-framework/rukpak/test/testutil"
)

var _ = Describe("Converting from other formats", func() {
	Describe("Converting from registry+v1", func() {
		var (
			in   registryv1.Bundle
			opts []func(*plainv0.Bundle)
		)

		BeforeEach(func() {
			in = registryv1.New(testutil.NewRegistryV1FS())
		})

		var out *plainv0.Bundle

		JustBeforeEach(func() {
			var err error
			out, err = bundle.Convert(in, opts...)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return a plain+v0 bundle", func() {
			Expect(out).ToNot(BeNil())
		})

		It("should have the deployment from the CSV", func() {
			By("finding all the deployments")
			deps, err := out.Objects(func(obj client.Object) bool {
				_, ok := obj.(*appsv1.Deployment)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(deps).To(HaveLen(1))
			Expect(deps[0].GetName()).To(Equal("memcached-operator-controller-manager"))

			By("finding the service account for the deployment")
			svcAccs, err := out.Objects(func(obj client.Object) bool {
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
			roles, err := out.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.Role)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(HaveLen(0))

			By("extracting the role bindings")
			roleBindings, err := out.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.RoleBinding)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roleBindings).To(HaveLen(0))
		})

		It("should have all the cluster RBAC from the CSV", func() {
			By("extracting the cluster roles")
			roles, err := out.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.ClusterRole)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roles).To(HaveLen(1))

			By("extracting the cluster role bindings")
			roleBindings, err := out.Objects(func(obj client.Object) bool {
				_, ok := obj.(*rbacv1.ClusterRoleBinding)
				return ok
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(roleBindings).To(HaveLen(0))
		})
	})
})
