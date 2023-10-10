package probing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_newCELProbe(t *testing.T) {
	t.Parallel()

	_, err := newCELProbe(`self.test`, "")
	require.ErrorIs(t, err, ErrCELInvalidEvaluationType)
}

func Test_celProbe(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		rule, message string
		obj           *unstructured.Unstructured

		success bool
	}{
		{
			name:    "simple success",
			rule:    `self.metadata.name == "hans"`,
			message: "aaaaaah!",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "hans",
					},
				},
			},
			success: true,
		},
		{
			name:    "simple failure",
			rule:    `self.metadata.name == "hans"`,
			message: "aaaaaah!",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "nothans",
					},
				},
			},
			success: false,
		},
		{
			name:    "OpenShift Route success simple",
			rule:    `self.status.ingress.all(i, i.conditions.all(c, c.type == "Ready" && c.status == "True"))`,
			message: "aaaaaah!",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"test": []interface{}{"1", "2", "3"},
						"ingress": []interface{}{
							map[string]interface{}{
								"host": "hostname.xxx.xxx",
								"conditions": []interface{}{
									map[string]interface{}{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
						},
					},
				},
			},
			success: true,
		},
		{
			name:    "OpenShift Route failure",
			rule:    `self.status.ingress.all(i, i.conditions.all(c, c.type == "Ready" && c.status == "True"))`,
			message: "aaaaaah!",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"test": []interface{}{"1", "2", "3"},
						"ingress": []interface{}{
							map[string]interface{}{
								"host": "hostname.xxx.xxx",
								"conditions": []interface{}{
									map[string]interface{}{
										"type":   "Ready",
										"status": "True",
									},
								},
							},
							map[string]interface{}{
								"host": "otherhost.xxx.xxx",
								"conditions": []interface{}{
									map[string]interface{}{
										"type":   "Ready",
										"status": "False",
									},
								},
							},
						},
					},
				},
			},
			success: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			p, err := newCELProbe(test.rule, test.message)
			require.NoError(t, err)

			success, outMsg := p.Probe(test.obj)
			assert.Equal(t, test.success, success)
			assert.Equal(t, test.message, outMsg)
		})
	}
}
