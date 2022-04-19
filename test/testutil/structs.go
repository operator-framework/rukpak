package testutil

import (
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewTestingCRD(name, group string, versions []apiextensionsv1.CustomResourceDefinitionVersion) *apiextensionsv1.CustomResourceDefinition {
	if name == "" {
		name = GenName(DefaultCrdName)
	}
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%v.%v", name, group),
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Scope:    apiextensionsv1.ClusterScoped,
			Group:    group,
			Versions: versions,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   name,
				Singular: name,
				Kind:     name,
				ListKind: name + "List",
			},
		},
	}
}

func NewTestingCR(name, group, version, kind string) *unstructured.Unstructured {
	newTestingCr := &unstructured.Unstructured{}
	newTestingCr.SetKind(kind)
	newTestingCr.SetAPIVersion(group + "/" + version)
	newTestingCr.SetGenerateName(name)
	return newTestingCr
}
