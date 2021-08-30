package controller

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(
		kscheme.AddToScheme,
		v1alpha1.AddToScheme,
	)

	// AddToScheme adds all types necessary for the controller to operate.
	addToScheme = schemeBuilder.AddToScheme
)

const (
	ProvisionerID v1alpha1.ProvisionerID = "rukpack.io/k8s"
)

type manageable interface {
	manageWith(mgr ctrl.Manager) error
}

type manageables []manageable

func (m manageables) manageWith(mgr ctrl.Manager) error {
	for _, man := range m {
		if m == nil {
			// Panic because this case indicates a bug
			panic("failed to register controller with manager: cannot be nil")
		}

		if err := man.manageWith(mgr); err != nil {
			// Bail out if any controller fails to register with the manager
			return err
		}
	}

	return nil
}

type controller struct {
	client.Client

	log     logr.Logger
	managed manageables
}

// NewReconciler constructs and returns a controller.
func NewController(cli client.Client, log logr.Logger) (*controller, error) {
	return &controller{
		Client: cli,
		log:    log,
		managed: manageables{
			&provisionerClassController{
				Client: cli,
				log:    log.WithValues("controller", "provisionerclass"),
			},
			&bundleController{
				Client: cli,
				log:    log.WithValues("controller", "bundle"),
			},
		},
	}, nil
}

// ManageWith adds the controller to the given controller manager.
func (c *controller) ManageWith(mgr ctrl.Manager) error {
	// Add shared watched types to scheme
	if err := addToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	return c.managed.manageWith(mgr)
}
