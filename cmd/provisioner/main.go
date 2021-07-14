package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/operator-framework/rukpak/pkg/k8s/provisioner"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	cmd := &cobra.Command{
		Use:   "provisioner",
		Short: "Runs the Rukpak.io provisioner controller(s)",
		RunE:  run,
	}

	cmd.Flags().String("namespace", "rukpak", "configures the global namespace that will house the underlying pvc/job resources")
	cmd.Flags().String("unpack-image", "quay.io/rukpak/unpacker:latest", "configures the container image used for unpacking arbitrary bundle content")
	cmd.Flags().String("serve-image", "quay.io/rukpak/server:latest", "configures the container image used for serving filesystem content")

	if err := cmd.Execute(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to initialize the schema(s): %v", err)
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	unpackImage, err := cmd.Flags().GetString("unpack-image")
	if err != nil {
		return err
	}
	serveImage, err := cmd.Flags().GetString("serve-image")
	if err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme:    scheme,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to setup manager instance: %v", err)
	}

	r, err := provisioner.NewReconciler(
		scheme,
		provisioner.WithClient(mgr.GetClient()),
		provisioner.WithGlobalNamespace(namespace),
		provisioner.WithLogger(setupLog),
		provisioner.WithUnpackImage(unpackImage),
		provisioner.WithServeImage(serveImage),
	)
	if err != nil {
		return fmt.Errorf("failed to create a new reconciler instance: %v", err)
	}

	if err := r.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to attach the provisioner controllers to the manager instance: %v", err)
	}

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("problem running the manager: %v", err)
	}
	setupLog.Info("exiting manager")
	return nil
}
