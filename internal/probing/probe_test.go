package probing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ Prober = (*proberMock)(nil)

type proberMock struct {
	mock.Mock
}

func (m *proberMock) Probe(obj *unstructured.Unstructured) (
	success bool, message string,
) {
	args := m.Called(obj)
	return args.Bool(0), args.String(1)
}

func TestList(t *testing.T) {
	t.Parallel()
	prober1 := &proberMock{}
	prober2 := &proberMock{}

	prober1.
		On("Probe", mock.Anything).
		Return(false, "error from prober1")
	prober2.
		On("Probe", mock.Anything).
		Return(false, "error from prober2")

	l := list{prober1, prober2}

	s, m := l.Probe(&unstructured.Unstructured{})
	assert.False(t, s)
	assert.Equal(t, "error from prober1, error from prober2", m)
}

func TestCondition(t *testing.T) {
	t.Parallel()
	c := &conditionProbe{
		Type:   "Available",
		Status: "False",
	}

	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		succeeds bool
		message  string
	}{
		{
			name: "succeeds",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":               "Banana",
								"status":             "True",
								"observedGeneration": int64(1), // up to date
							},
							map[string]interface{}{
								"type":               "Available",
								"status":             "False",
								"observedGeneration": int64(1), // up to date
							},
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: true,
			message:  "",
		},
		{
			name: "outdated",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":               "Available",
								"status":             "False",
								"observedGeneration": int64(1), // up to date
							},
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(42),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": outdated`,
		},
		{
			name: "wrong status",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":               "Available",
								"status":             "Unknown",
								"observedGeneration": int64(1), // up to date
							},
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": wrong status`,
		},
		{
			name: "not reported",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":               "Banana",
								"status":             "True",
								"observedGeneration": int64(1), // up to date
							},
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": not reported`,
		},
		{
			name: "malformed condition type int",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							42, 56,
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": malformed`,
		},
		{
			name: "malformed condition type string",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							"42", "56",
						},
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": malformed`,
		},
		{
			name: "malformed conditions array",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": 42,
					},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": malformed`,
		},
		{
			name: "missing conditions",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{},
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": missing .status.conditions`,
		},
		{
			name: "missing status",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": int64(1),
					},
				},
			},
			succeeds: false,
			message:  `condition "Available" == "False": missing .status.conditions`,
		},
	}

	for i := range tests {
		test := tests[i]

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, m := c.Probe(test.obj)
			assert.Equal(t, test.succeeds, s)
			assert.Equal(t, test.message, m)
		})
	}
}

func TestFieldsEqual(t *testing.T) {
	t.Parallel()
	fe := &fieldsEqualProbe{
		FieldA: ".spec.fieldA",
		FieldB: ".spec.fieldB",
	}

	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		succeeds bool
		message  string
	}{
		{
			name: "simple succeeds",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": "test",
						"fieldB": "test",
					},
				},
			},
			succeeds: true,
		},
		{
			name: "simple not equal",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": "test",
						"fieldB": "not test",
					},
				},
			},
			succeeds: false,
			message:  `".spec.fieldA" == ".spec.fieldB": "test" != "not test"`,
		},
		{
			name: "complex succeeds",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": map[string]interface{}{
							"fk": "fv",
						},
						"fieldB": map[string]interface{}{
							"fk": "fv",
						},
					},
				},
			},
			succeeds: true,
		},
		{
			name: "simple not equal",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": map[string]interface{}{
							"fk": "fv",
						},
						"fieldB": map[string]interface{}{
							"fk": "something else",
						},
					},
				},
			},
			succeeds: false,
			message:  `".spec.fieldA" == ".spec.fieldB": "map[fk:fv]" != "map[fk:something else]"`,
		},
		{
			name: "int not equal",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": map[string]interface{}{
							"fk": 1.0,
						},
						"fieldB": map[string]interface{}{
							"fk": 2.0,
						},
					},
				},
			},
			succeeds: false,
			message:  `".spec.fieldA" == ".spec.fieldB": "map[fk:1]" != "map[fk:2]"`,
		},
		{
			name: "fieldA missing",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldB": "test",
					},
				},
			},
			succeeds: false,
			message:  `".spec.fieldA" == ".spec.fieldB": ".spec.fieldA" missing`,
		},
		{
			name: "fieldB missing",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"fieldA": "test",
					},
				},
			},
			succeeds: false,
			message:  `".spec.fieldA" == ".spec.fieldB": ".spec.fieldB" missing`,
		},
	}

	for i := range tests {
		test := tests[i]

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, m := fe.Probe(test.obj)
			assert.Equal(t, test.succeeds, s)
			assert.Equal(t, test.message, m)
		})
	}
}

func TestStatusObservedGeneration(t *testing.T) {
	t.Parallel()
	properMock := &proberMock{}
	og := &statusObservedGenerationProbe{
		Prober: properMock,
	}

	properMock.On("Probe", mock.Anything).Return(true, "banana")

	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		succeeds bool
		message  string
	}{
		{
			name: "outdated",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": int64(4),
					},
					"status": map[string]interface{}{
						"observedGeneration": int64(2),
					},
				},
			},
			succeeds: false,
			message:  ".status outdated",
		},
		{
			name: "up-to-date",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": int64(4),
					},
					"status": map[string]interface{}{
						"observedGeneration": int64(4),
					},
				},
			},
			succeeds: true,
			message:  "banana",
		},
		{
			name: "not reported",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"generation": int64(4),
					},
					"status": map[string]interface{}{},
				},
			},
			succeeds: true,
			message:  "banana",
		},
	}

	for i := range tests {
		test := tests[i]

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			s, m := og.Probe(test.obj)
			assert.Equal(t, test.succeeds, s)
			assert.Equal(t, test.message, m)
		})
	}
}
