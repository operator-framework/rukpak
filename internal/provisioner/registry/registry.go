package registry

import (
	"github.com/operator-framework/rukpak/internal/provisioner/common"
	"github.com/operator-framework/rukpak/internal/provisioner/registry/controllers"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupProvisionerWithManager takes the necessary parameters in order to setup
// the controllers for the registry provisioner
func SetupProvisionerWithManager(mgr manager.Manager, br common.BundleReconciler, _ common.BundleDeploymentReconciler) error {
	if err := (&controllers.BundleReconciler{
		BundleReconciler: br,
	}).SetupWithManager(mgr); err != nil {
		return err
	}
	return nil
}
