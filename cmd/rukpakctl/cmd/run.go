/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"os"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/rukpakctl"
	"github.com/operator-framework/rukpak/internal/util"
)

// newRunCmd creates the run command
func newRunCmd() *cobra.Command {
	var (
		systemNamespace                      string
		uploadServiceName                    string
		caSecretName                         string
		bundleDeploymentProvisionerClassName string
		bundleProvisionerClassName           string
	)

	cmd := &cobra.Command{
		Use:   "run <bundleDeploymentName> <bundleDir>",
		Short: "Run a bundle from an upload of a local bundle directory.",
		Long: `Run a bundle from an upload of a local bundle directory.

The run subcommand allows bundle developers to quickly iterate on bundles
they are developing, and to test how their bundle deployment pivots from
one version to the next.
`,
		Example: `
  #
  # Initial creation of memcached-api bundle deployment:
  #
  $ rukpakctl run memcached-api ./memcached-api-v0.1.0/
  bundledeployment.core.rukpak.io "memcached-api" applied
  successfully uploaded bundle content for "memcached-api-5b9bbf8799"

  #
  # Pivot to a new bundle for the existing memcached-api bundle-deployment
  #
  $ rukpakctl run memcached-api ./memcached-api-v0.2.0/
  bundledeployment.core.rukpak.io "memcached-api" applied
  successfully uploaded bundle content for "memcached-api-8578dfddf9"

  #
  # Run the same command again
  #
  $ rukpakctl run memcached-api ./memcached-api-v0.2.0/
  bundledeployment.core.rukpak.io "memcached-api" applied
  bundle "memcached-api-8578dfddf9" is already up-to-date
`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bundleDeploymentName, bundleDir := args[0], args[1]
			ctx := signals.SetupSignalHandler()

			cfg := ctrl.GetConfigOrDie()

			sch := scheme.Scheme
			if err := rukpakv1alpha1.AddToScheme(sch); err != nil {
				log.Fatal(err)
			}
			cl, err := client.New(cfg, client.Options{Scheme: sch})
			if err != nil {
				log.Fatal(err)
			}

			bundleFS := os.DirFS(bundleDir)
			digest := fnv.New64a()
			if err := util.FSToTarGZ(digest, bundleFS); err != nil {
				log.Fatal(err)
			}

			bundleLabels := map[string]string{
				"app":          bundleDeploymentName,
				"bundleDigest": fmt.Sprintf("%x", digest.Sum(nil)),
			}

			bd := buildBundleDeployment(bundleDeploymentName, bundleLabels, bundleDeploymentProvisionerClassName, bundleProvisionerClassName)
			if err := cl.Patch(ctx, bd, client.Apply, client.ForceOwnership, client.FieldOwner("rukpakctl")); err != nil {
				log.Fatalf("apply bundle deployment: %v", err)
			}
			fmt.Printf("bundledeployment.core.rukpak.io %q applied\n", bundleDeploymentName)

			rukpakCAs, err := rukpakctl.GetClusterCA(ctx, cl, systemNamespace, caSecretName)
			if err != nil {
				log.Fatal(err)
			}

			bundleName, err := getBundleName(ctx, cfg, bundleLabels)
			if err != nil {
				log.Fatalf("failed to get bundle name: %v", err)
			}
			bu := rukpakctl.BundleUploader{
				UploadServiceName:      uploadServiceName,
				UploadServiceNamespace: systemNamespace,
				Cfg:                    cfg,
				RootCAs:                rukpakCAs,
			}
			modified, err := bu.Upload(ctx, bundleName, bundleFS)
			if err != nil {
				log.Fatalf("failed to upload bundle: %v", err)
			}
			if !modified {
				fmt.Printf("bundle %q is already up-to-date\n", bundleName)
				return
			}

			fmt.Printf("successfully uploaded bundle content for %q\n", bundleName)
		},
	}
	cmd.Flags().StringVar(&systemNamespace, "system-namespace", "rukpak-system", "the namespace in which the rukpak controllers are deployed.")
	cmd.Flags().StringVar(&uploadServiceName, "upload-service-name", "upload-manager", "the name of the service of the upload manager.")
	cmd.Flags().StringVar(&caSecretName, "ca-secret-name", "rukpak-ca", "the name of the secret in the system namespace containing the root CAs used to authenticate the upload service.")
	cmd.Flags().StringVar(&bundleDeploymentProvisionerClassName, "bundle-deployment-provisioner-class", "core.rukpak.io/plain", "Provisioner class name to set on bundle deployment.")
	cmd.Flags().StringVar(&bundleProvisionerClassName, "bundle-provisioner-class", "core.rukpak.io/plain", "Provisioner class name to set on bundle.")
	return cmd
}

func buildBundleDeployment(bdName string, bundleLabels map[string]string, biPCN, bPNC string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": rukpakv1alpha1.GroupVersion.String(),
		"kind":       rukpakv1alpha1.BundleDeploymentKind,
		"metadata": map[string]interface{}{
			"name": bdName,
		},
		"spec": map[string]interface{}{
			"provisionerClassName": biPCN,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": bundleLabels,
				},
				"spec": map[string]interface{}{
					"provisionerClassName": bPNC,
					"source": map[string]interface{}{
						"type":   rukpakv1alpha1.SourceTypeUpload,
						"upload": &rukpakv1alpha1.UploadSource{},
					},
				},
			},
		},
	}}
}

func getBundleName(ctx context.Context, cfg *rest.Config, bundleLabels map[string]string) (string, error) {
	dynCl, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("build dynamic client: %v", err)
	}

	watch, err := dynCl.Resource(rukpakv1alpha1.GroupVersion.WithResource("bundles")).Watch(ctx, metav1.ListOptions{Watch: true, LabelSelector: labels.FormatLabels(bundleLabels)})
	if err != nil {
		return "", fmt.Errorf("watch bundles: %v", err)
	}
	defer watch.Stop()

	select {
	case evt := <-watch.ResultChan():
		return evt.Object.(client.Object).GetName(), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
