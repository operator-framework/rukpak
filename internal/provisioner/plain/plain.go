package plain

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/operator-framework/rukpak/internal/provisioner/common"
	"github.com/operator-framework/rukpak/internal/provisioner/plain/controllers"
)

// SetupProvisionerWithManager takes the necessary parameters in order to setup
// the controllers for the plain provisioner
func SetupProvisionerWithManager(mgr manager.Manager, br common.BundleReconciler, bdr common.BundleDeploymentReconciler) error {
	if err := (&controllers.BundleReconciler{
		BundleReconciler: br,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err := (&controllers.BundleDeploymentReconciler{
		BundleDeploymentReconciler: bdr,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	return nil
}
