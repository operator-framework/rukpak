package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/operator-framework/rukpak/internal/unit"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestStoreAndLoad(t *testing.T) {
	// Setup envtest client
	kubeclient, err := unit.SetupClient()
	require.NoError(t, err, "failed to create kube client")

	for _, tt := range []struct {
		name  string
		cms   ConfigMaps
		owner client.Object
		owned []client.Object
		err   error
	}{
		{
			name: "can store a value correctly",
			cms:  ConfigMaps{Client: kubeclient, Namespace: "default"},
			owner: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "default"},
			},
			owned: []client.Object{
				&appsv1.Deployment{
					TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
					ObjectMeta: metav1.ObjectMeta{Name: "test-owned"},
				},
			},
			err: nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Create a client.Object to use as an owner for storing via ConfigMaps
			err = kubeclient.Create(context.Background(), tt.owner)
			if err != nil {
				require.ErrorIs(t, err, tt.err, "failed to create sample client.Object:", err)
			}

			// Run the store function and provide the owner with some owned client.Objects
			err = tt.cms.Store(context.Background(), tt.owner, tt.owned)
			if err != nil {
				require.ErrorIs(t, err, tt.err, "failed to store owned objects:", err)
			}

			// Validate that the values were store correctly
			actual, err := tt.cms.Load(context.Background(), tt.owner)
			if !errors.Is(err, tt.err) {
				require.ErrorIs(t, err, tt.err, "failed to load from ConfigMaps:", err)
			}

			for _, expected := range tt.owned {
				bytes, err := json.Marshal(expected)
				if err != nil {
					require.ErrorIs(t, err, tt.err, "failed to marshal expected value for comparison:", err)
				}

				var owned unstructured.Unstructured
				err = json.Unmarshal(bytes, &owned)
				if err != nil {
					require.ErrorIs(t, err, tt.err, "failed to unmarshal expected value for comparison:", err)
				}

				require.Contains(t, actual, owned)
			}
		})
	}
}
