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
)

type bundleOptions struct {
	*kubernetes.Clientset
	runtimeclient.Client
	namespace string
}

// bundleCmd represents the bundle command
func newBundleCmd() *cobra.Command {
	var bundleOpt bundleOptions

	bundleCmd := &cobra.Command{
		Use:   "bundle <manifest directory>  <bundle name prefix>",
		Short: "create rukpak bundle resource.",
		Long:  `create rukpak bundle resource with specified contents.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("requires argument: <manifest file directory> <bundle name prefix>")
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

			bundleOpt.Client, err = runtimeclient.New(cfg, runtimeclient.Options{
				Scheme: sch,
			})
			if err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			if bundleOpt.Clientset, err = kubernetes.NewForConfig(cfg); err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			err = bundle(cmd.Context(), bundleOpt, args)
			if err != nil {
				log.Fatalf("bundle command failed: %v", err)
			}
		},
	}
	bundleCmd.Flags().StringVar(&bundleOpt.namespace, "namespace", "rukpak-system", "namespace for target or work resources")
	return bundleCmd
}

func bundle(ctx context.Context, opt bundleOptions, args []string) error {
	namePrefix := "rukpakctl-bundle-"
	if len(args) > 1 {
		namePrefix = args[1]
	}
	// Create a bundle configmap
	configmapName, err := utils.CreateConfigmap(ctx, opt.CoreV1(), namePrefix, args[0], opt.namespace)
	if err != nil {
		return fmt.Errorf("failed to create a configmap: %v", err)
	}

	bundle := &rukpakv1alpha1.Bundle{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Bundle",
			APIVersion: "core.rukpak.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: namePrefix,
		},
		Spec: rukpakv1alpha1.BundleSpec{
			ProvisionerClassName: "core.rukpak.io/plain",
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
	}
	err = opt.Create(ctx, bundle)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %v", err)
	}
	fmt.Printf("bundle %q created\n", bundle.ObjectMeta.Name)

	return nil
}
