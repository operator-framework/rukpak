package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/rukpak/internal/util"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultTestingCrdName  = "samplecrd"
	defaultTestingCrName   = "samplecr"
	defaultTestingCrdGroup = "e2e.io"
)

var _ = Describe("crd validation webhook", func() {
	When("a crd event is emitted", func() {
		var ctx context.Context

		BeforeEach(func() { ctx = context.Background() })
		AfterEach(func() { ctx.Done() })

		When("an incoming crd event is safe", func() {
			var crd *apiextensionsv1.CustomResourceDefinition

			BeforeEach(func() {
				crd = util.NewTestingCRD(defaultTestingCrdName, defaultTestingCrdGroup, true,
					[]apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  true,
							Storage: true,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type:        "object",
									Description: "my crd schema",
								},
							},
						},
					},
				)

				Eventually(func() error {
					return c.Create(ctx, crd)
				}).Should(Succeed(), "should be able to create a safe crd but was not")
			})

			AfterEach(func() {
				By("deleting the testing crd")
				Expect(c.Delete(ctx, crd)).To(BeNil())
			})

			It("should allow the crd update event to occur", func() {
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
						return err
					}

					crd.Spec.Versions[0].Storage = false

					crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
						Name:    "v1alpha2",
						Served:  true,
						Storage: true,
						Schema: &apiextensionsv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
								Type:        "object",
								Description: "my crd schema",
							},
						},
					})

					return c.Update(ctx, crd)
				}).Should(Succeed())
			})
		})

		// TODO (tylerslaton): Check CRDValidator safe storage logic
		//
		// This test is currently trying to simulate a situtation where an incoming
		// CRD removes a stored version. However, it does not work as expected because
		// something (potentially the apiserver) is intervening first and not allowing
		// it through. This is fine and ultimately what the safe storage logic of the
		// CRDValidator was designed to prevent but is unknown why it is occurring. We
		// Should come back to this test case, figure out what is preventing it from
		// hitting the webhook and decide if we want to keep that logic or not.
		PWhen("an incoming crd event removes a stored version", func() {
			var crd *apiextensionsv1.CustomResourceDefinition

			BeforeEach(func() {
				crd = util.NewTestingCRD(defaultTestingCrdName, defaultTestingCrdGroup, true,
					[]apiextensionsv1.CustomResourceDefinitionVersion{
						{
							Name:    "v1alpha1",
							Served:  true,
							Storage: true,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type:        "object",
									Description: "my crd schema",
								},
							},
						},
						{
							Name:    "v1alpha2",
							Served:  true,
							Storage: false,
							Schema: &apiextensionsv1.CustomResourceValidation{
								OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
									Type:        "object",
									Description: "my crd schema",
								},
							},
						},
					},
				)

				Eventually(func() error {
					return c.Create(ctx, crd)
				}).Should(Succeed(), "should be able to create a safe crd but was not")
			})

			AfterEach(func() {
				By("deleting the testing crd")
				Expect(c.Delete(ctx, crd)).To(BeNil())
			})

			It("should deny admission", func() {
				Eventually(func() string {
					if err := c.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
						return err.Error()
					}

					newCRD := util.NewTestingCRD(crd.Spec.Names.Singular, defaultTestingCrdGroup, false,
						[]apiextensionsv1.CustomResourceDefinitionVersion{
							{
								Name:    "v1alpha2",
								Served:  true,
								Storage: true,
								Schema: &apiextensionsv1.CustomResourceValidation{
									OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
										Type:        "object",
										Description: "my crd schema",
									},
								},
							},
							{
								Name:    "v1alpha3",
								Served:  true,
								Storage: false,
								Schema: &apiextensionsv1.CustomResourceValidation{
									OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
										Type:        "object",
										Description: "my crd schema",
									},
								},
							},
						},
					)

					newCRD.SetResourceVersion(crd.ResourceVersion)
					newCRD.Status.StoredVersions = []string{"v1alpha2"}

					err := c.Update(ctx, newCRD)
					if err != nil {
						return err.Error()
					}
					return ""
				}).Should(ContainSubstring("cannot remove stored versions"))
			})
		})

		When("an incoming crd event modifies the schema in a way that breaks an existing cr", func() {
			var crd *apiextensionsv1.CustomResourceDefinition

			BeforeEach(func() {
				crd = util.NewTestingCRD(defaultTestingCrdName, defaultTestingCrdGroup, true, []apiextensionsv1.CustomResourceDefinitionVersion{{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:        "object",
							Description: "my crd schema",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"sampleProperty": {Type: "string"},
							},
						},
					},
				}},
				)

				Eventually(func() error {
					return c.Create(ctx, crd)
				}).Should(Succeed(), "should be able to create a safe crd but was not")

				// Build up a CR to create out of unstructured.Unstructured
				sampleCR := util.NewTestingCR(defaultTestingCrName, crd.Spec.Names.Singular, defaultTestingCrdGroup, "v1alpha1")
				Eventually(func() error {
					return c.Create(ctx, sampleCR)
				}).Should(Succeed(), "should be able to create a cr for the sample crd but was not")

			})

			AfterEach(func() {
				By("deleting the testing crd")
				Expect(c.Delete(ctx, crd)).To(BeNil())
			})

			It("should deny admission", func() {
				Eventually(func() string {
					if err := c.Get(ctx, client.ObjectKeyFromObject(crd), crd); err != nil {
						return err.Error()
					}

					// Update the v1alpha1 schema to invalidate existing CR created in BeforeEach()
					crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Required = []string{"sampleProperty"}

					err := c.Update(ctx, crd)
					if err != nil {
						return err.Error()
					}
					return ""
				}).Should(ContainSubstring("error validating existing CRs against new CRD's schema"))
			})
		})
	})
})
