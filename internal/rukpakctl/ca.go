package rukpakctl

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClusterCA returns an x509.CertPool by reading the contents of a Kubernetes Secret. It uses the provided
// client to get the requested secret and then loads the contents of the secret's "ca.crt" key into the cert pool.
func GetClusterCA(ctx context.Context, cl client.Reader, secretKey types.NamespacedName) (*x509.CertPool, error) {
	caSecret := &corev1.Secret{}
	if err := cl.Get(ctx, secretKey, caSecret); err != nil {
		return nil, fmt.Errorf("get rukpak certificate authority: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caSecret.Data["ca.crt"]) {
		return nil, errors.New("failed to load certificate authority into cert pool: malformed PEM?")
	}
	return certPool, nil
}
