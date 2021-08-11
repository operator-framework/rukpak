package cmd

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	k8scontroller "github.com/operator-framework/rukpak/provisioner/k8s/controller"
)

func init() {
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:          "run",
	Short:        "run the provisioner",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctrl.SetLogger(rootLog)

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme: runtime.NewScheme(),
		})
		if err != nil {
			return err
		}

		controller, err := k8scontroller.NewController(
			mgr.GetClient(),
			ctrl.Log.WithName("run"),
		)
		if err != nil {
			return nil
		}

		if err = controller.ManageWith(mgr); err != nil {
			return err
		}

		return mgr.Start(signals.SetupSignalHandler())
	},
}
