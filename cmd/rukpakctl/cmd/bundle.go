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

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/cmd/rukpakctl/utils"
)

type BundleOptions struct {
	*kubernetes.Clientset
	runtimeclient.Client
	namespace string
}

var bundleOpt BundleOptions

// bundleCmd represents the bundle command
var bundleCmd = &cobra.Command{
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
		kubeconfig, err := cmd.Flags().GetString("kubeconfig")
		if err != nil {
			fmt.Printf("failed to find kubeconfig location: %+v\n", err)
			return
		}
		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			fmt.Printf("failed to find kubeconfig location: %+v\n", err)
			return
		}

		bundleOpt.Client, err = runtimeclient.New(config, runtimeclient.Options{
			Scheme: scheme,
		})
		if err != nil {
			fmt.Printf("failed to create kubernetes client: %+v\n", err)
			return
		}

		if bundleOpt.Clientset, err = kubernetes.NewForConfig(config); err != nil {
			fmt.Printf("failed to create kubernetes client: %+v\n", err)
			return
		}

		bundleOpt.namespace, err = cmd.Flags().GetString("namespace")
		if err != nil {
			bundleOpt.namespace = "rukpak-system"
		}

		err = bundle(bundleOpt, args)
		if err != nil {
			fmt.Printf("bundle command failed: %+v\n", err)
		}
	},
}

func init() {
	createCmd.AddCommand(bundleCmd)
}

func bundle(opt BundleOptions, args []string) error {
	namePrefix := "rukpakctl-bundle-"
	if len(args) > 1 {
		namePrefix = args[1]
	}
	// Create a bundle configmap
	configmapName, err := utils.CreateConfigmap(opt.CoreV1(), namePrefix, args[0], opt.namespace)
	if err != nil {
		return fmt.Errorf("failed to create a configmap: %+v", err)
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
	err = opt.Create(context.Background(), bundle)
	if err != nil {
		return fmt.Errorf("failed to create bundle: %+v", err)
	}
	fmt.Printf("bundle %q created\n", bundle.ObjectMeta.Name)

	return nil
}
