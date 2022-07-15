package rukpakctl

import (
	"context"
	"crypto/x509"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetRukpakCA(ctx context.Context, cl client.Reader) (*x509.CertPool, error) {
	rukpakCASecret := &corev1.Secret{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: "rukpak-system", Name: "rukpak-ca"}, rukpakCASecret); err != nil {
		return nil, fmt.Errorf("get rukpak certificate authority: %v", err)
	}
	rootCertPool := x509.NewCertPool()
	if !rootCertPool.AppendCertsFromPEM(rukpakCASecret.Data["ca.crt"]) {
		return nil, fmt.Errorf("failed to load rukpak-ca certificate authority into client root CAs: malformed PEM?")
	}
	return rootCertPool, nil
}
