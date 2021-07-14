package provisioner

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	client.Client
	log             logr.Logger
	globalNamespace string
	unpackImage     string
	serveImage      string
}

type ReconcilerOption func(*Reconciler)

// NewReconciler constructs and returns an BundleReconciler.
// As a side effect, the given scheme has operator discovery types added to it.
func NewReconciler(scheme *runtime.Scheme, opts ...ReconcilerOption) (*Reconciler, error) {
	// Add watched types to scheme.
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}

	reconciler := &Reconciler{}
	for _, opt := range opts {
		opt(reconciler)
	}

	return reconciler, nil
}

func WithClient(client client.Client) ReconcilerOption {
	return func(r *Reconciler) {
		r.Client = client
	}
}

func WithLogger(logger logr.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = logger
	}
}

func WithGlobalNamespace(namespace string) ReconcilerOption {
	return func(r *Reconciler) {
		r.globalNamespace = namespace
	}
}

func WithUnpackImage(image string) ReconcilerOption {
	return func(r *Reconciler) {
		r.unpackImage = image
	}
}

func WithServeImage(image string) ReconcilerOption {
	return func(r *Reconciler) {
		r.serveImage = image
	}
}
