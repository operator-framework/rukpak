package cmd

import (
	"bytes"
	"fmt"
	"log"
	"testing/fstest"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"

	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
	"github.com/operator-framework/rukpak/internal/rukpakctl"
	"github.com/operator-framework/rukpak/internal/util"
)

func newAlphaBootstrapCmd() *cobra.Command {
	var (
		systemNamespace   string
		uploadServiceName string
		caSecretName      string
	)
	cmd := &cobra.Command{
		Use:    "bootstrap <bundleDeploymentName>",
		Hidden: true,
		Short:  "Bootstrap or adopt a manifest into rukpak's management.",
		Long: `Bootstrap or adopt a manifest into rukpak's management.

The bootstrap subcommand allows administrators to deploy or update an
existing set of arbitrary kubernetes objects such that they become
managed by rukpak. This is useful for bootstrapping rukpak itself or
in migration scenarios where existing cluster objects need to be moved
under the management of a rukpak BundleDeployment.'
`,
		Example: `
  #
  # Bootstrap a rukpak release manifest into a BundleDeployment
  #
  $ curl -sSL https://github.com/operator-framework/rukpak/releases/download/<version>/rukpak.yaml | rukpakctl alpha bootstrap rukpak
  successfully bootstrapped bundle deployment "rukpak"

  #
  # Adopt an existing set of resources into a BundleDeployment
  #
  $ kubectl apply -f stuff.yaml
  $ kubectl get -f stuff.yaml -o yaml | rukpakctl alpha bootstrap stuff
  successfully bootstrapped bundle deployment "stuff"
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			bundleDeploymentName := args[0]
			result := resource.NewLocalBuilder().Unstructured().Flatten().Stdin().Do()
			if err := result.Err(); err != nil {
				log.Fatal(err)
			}
			items, err := result.Infos()
			if err != nil {
				log.Fatal(err)
			}

			cfg, err := config.GetConfig()
			if err != nil {
				log.Fatal(err)
			}
			cl, err := client.New(cfg, client.Options{})
			if err != nil {
				log.Fatal(err)
			}

			for _, item := range items {
				obj := item.Object.DeepCopyObject().(client.Object)
				util.AdoptObject(obj, systemNamespace, bundleDeploymentName)
				if err := cl.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner("rukpakctl")); err != nil {
					log.Fatal(err)
				}
			}

			manifest := bytes.Buffer{}
			for _, item := range items {
				manifest.WriteString("---\n")
				data, err := yaml.Marshal(item.Object)
				if err != nil {
					log.Fatal(err)
				}
				manifest.Write(data)
			}

			bundleFS := &fstest.MapFS{
				"manifests/manifest.yaml": &fstest.MapFile{Data: manifest.Bytes()},
			}

			r := rukpakctl.Run{
				Config:            cfg,
				SystemNamespace:   systemNamespace,
				UploadServiceName: uploadServiceName,
				CASecretName:      caSecretName,
			}
			modified, err := r.Run(ctx, bundleDeploymentName, bundleFS, rukpakctl.RunOptions{
				BundleDeploymentProvisionerClassName: plain.ProvisionerID,
				BundleProvisionerClassName:           plain.ProvisionerID,
			})
			if err != nil {
				log.Fatal(err)
			}
			if !modified {
				fmt.Printf("bundle deployment %q is already up-to-date\n", bundleDeploymentName)
			} else {
				fmt.Printf("successfully bootstrapped bundle deployment %q\n", bundleDeploymentName)
			}
		},
	}
	cmd.Flags().StringVar(&systemNamespace, "system-namespace", "rukpak-system", "Namespace in which the core rukpak provisioners are running.")
	cmd.Flags().StringVar(&uploadServiceName, "upload-service-name", "core", "the name of the service of the upload manager.")
	cmd.Flags().StringVar(&caSecretName, "ca-secret-name", "rukpak-ca", "the name of the secret in the system namespace containing the root CAs used to authenticate the upload service.")
	return cmd
}
