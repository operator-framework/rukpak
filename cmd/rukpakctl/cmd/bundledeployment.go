/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/cmd/rukpakctl/utils"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
)

type bundleDeploymentOptions struct {
	*kubernetes.Clientset
	runtimeclient.Client
	namespace string
}

// contentCmd represents the content command
func newBundleDeploymentCmd() *cobra.Command {
	var bundleDeploymentOpt bundleDeploymentOptions

	bdCmd := &cobra.Command{
		Use:   "bundledeployment <manifest directory> <bundledeployment name prefix>",
		Short: "create rukpak bundledeployment resource",
		Long:  `create rukpak bundledeployment resource with specified contents.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("requires argument: <manifest file directory>  <bundledeployment name prefix>")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			sch := scheme.Scheme
			if err := rukpakv1alpha1.AddToScheme(sch); err != nil {
				log.Fatal(err)
			}

			cfg, err := config.GetConfig()
			if err != nil {
				log.Fatalf("failed to find kubeconfig location: %v", err)
			}

			bundleDeploymentOpt.Client, err = runtimeclient.New(cfg, runtimeclient.Options{
				Scheme: sch,
			})
			if err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			if bundleDeploymentOpt.Clientset, err = kubernetes.NewForConfig(cfg); err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			if err := bundleDeployment(cmd.Context(), bundleDeploymentOpt, args); err != nil {
				log.Fatalf("bundledeployment command failed: %v", err)
			}
		},
	}
	bdCmd.Flags().StringVar(&bundleDeploymentOpt.namespace, "namespace", "rukpak-system", "namespace for target or work resources")
	return bdCmd
}

func bundleDeployment(ctx context.Context, opt bundleDeploymentOptions, args []string) error {
	namePrefix := "rukpakctl-bd-"
	if len(args) > 1 {
		namePrefix = args[1]
	}
	// Create a bundle configmap
	configmapName, err := utils.CreateConfigmap(ctx, opt.CoreV1(), namePrefix, args[0], opt.namespace)
	if err != nil {
		return fmt.Errorf("failed to create a configmap: %v", err)
	}

	bundleDeployment := &rukpakv1alpha1.BundleDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BundleDeployment",
			APIVersion: "core.rukpak.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
		},
		Spec: rukpakv1alpha1.BundleDeploymentSpec{
			ProvisionerClassName: plain.ProvisionerID,
			Template: &rukpakv1alpha1.BundleTemplate{
				Spec: rukpakv1alpha1.BundleSpec{
					ProvisionerClassName: plain.ProvisionerID,
					Source: rukpakv1alpha1.BundleSource{
						Type: rukpakv1alpha1.SourceTypeLocal,
						Local: &rukpakv1alpha1.LocalSource{
							ConfigMapRef: &rukpakv1alpha1.ConfigMapRef{
								Name:      configmapName,
								Namespace: opt.namespace,
							},
						},
					},
				},
			},
		},
	}
	err = opt.Create(ctx, bundleDeployment)
	if err != nil {
		return fmt.Errorf("failed to create bundledeployment: %v", err)
	}
	fmt.Printf("bundledeployment %q created\n", bundleDeployment.ObjectMeta.Name)

	return nil
}
