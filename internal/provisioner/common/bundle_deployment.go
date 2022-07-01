package common

import (
	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/operator-framework/rukpak/internal/storage"
)

// TODO (tylerslaton): Implement a more deduplicated BundleDeploymentReconciler
//
// As we continue to onboard new Provisioners, having the ability to quickly
// spin up shared logic between this will prove useful. We have already
// made a large portion of the BundleReconciler code deduplicated so we
// should take this struct and work from there in a similar way.
type BundleDeploymentReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Controller         controller.Controller
	ActionClientGetter helmclient.ActionClientGetter
	BundleStorage      storage.Storage
	ReleaseNamespace   string
}
