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

func GetClusterCA(ctx context.Context, cl client.Reader, ns, secretName string) (*x509.CertPool, error) {
	caSecret := &corev1.Secret{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretName}, caSecret); err != nil {
		return nil, fmt.Errorf("get rukpak certificate authority: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(caSecret.Data["ca.crt"]) {
		return nil, errors.New("failed to load certificate authority into cert pool: malformed PEM?")
	}
	return certPool, nil
}
