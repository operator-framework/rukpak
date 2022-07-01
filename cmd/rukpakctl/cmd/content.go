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
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type options struct {
	*kubernetes.Clientset
	runtimeclient.Client
	namespace string
}

var opt options

var contentCmd = &cobra.Command{
	Use:   "content <bundle name>",
	Short: "display contents of the specified bundle.",
	Long:  `display contents of the specified bundle.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires 2 argument: <bundle name>")
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

		opt.Client, err = runtimeclient.New(config, runtimeclient.Options{
			Scheme: scheme,
		})
		if err != nil {
			fmt.Printf("failed to create kubernetes client: %+v\n", err)
			return
		}

		if opt.Clientset, err = kubernetes.NewForConfig(config); err != nil {
			fmt.Printf("failed to create kubernetes client: %+v\n", err)
			return
		}

		opt.namespace, err = cmd.Flags().GetString("namespace")
		if err != nil {
			opt.namespace = "rukpak-system"
		}

		err = content(opt, args)
		if err != nil {
			fmt.Printf("content command failed: %+v\n", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(contentCmd)
}

func content(opt options, args []string) error {
	// Create a temporary ClusterRoleBinding to bind the ServiceAccount to bundle-reader ClusterRole
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rukpakctl-crb",
			Namespace: opt.namespace,
		},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "rukpakctl-sa", Namespace: opt.namespace}},
		RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "bundle-reader"},
	}
	crb, err := opt.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a cluster role bindings: %+v", err)
	}
	defer deletecrb()

	// Create a temporary ServiceAccount
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rukpakctl-sa",
			Namespace: opt.namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRoleBinding",
					Name:       "rukpakctl-crb",
					UID:        crb.ObjectMeta.UID,
				},
			},
		},
	}
	_, err = opt.CoreV1().ServiceAccounts(opt.namespace).Create(context.Background(), &sa, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a service account: %+v", err)
	}

	bundle := &rukpakv1alpha1.Bundle{}
	err = opt.Get(context.Background(), runtimeclient.ObjectKey{Name: args[0]}, bundle)
	if err != nil {
		return fmt.Errorf("error : %+v", err)
	}
	url := bundle.Status.ContentURL
	if url == "" {
		return errors.New("error: url is not available")
	}

	// Create a Job that reads from the URL and outputs contents in the pod log
	mounttoken := true
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "rukpakctl-job-",
			Namespace:    opt.namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "rbac.authorization.k8s.io/v1",
					Kind:       "ClusterRoleBinding",
					Name:       "rukpakctl-crb",
					UID:        crb.ObjectMeta.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "rukpakctl",
							Image:   "curlimages/curl",
							Command: []string{"sh", "-c", "curl -sSLk -H \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" -o - " + url + " | tar ztv"},
						},
					},
					ServiceAccountName:           "rukpakctl-sa",
					RestartPolicy:                "Never",
					AutomountServiceAccountToken: &mounttoken,
				},
			},
		},
	}
	job, err = opt.BatchV1().Jobs(opt.namespace).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a job: %+v", err)
	}

	// Wait for Job completion
	for {
		deployedJob, err := opt.BatchV1().Jobs(opt.namespace).Get(context.Background(), job.ObjectMeta.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get a job: %+v", err)
		}
		if deployedJob.Status.CompletionTime != nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Get Pod for the Job
	pods, err := opt.CoreV1().Pods(opt.namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "job-name=" + job.ObjectMeta.Name})
	if err != nil {
		return fmt.Errorf("failed to find pods for job: %+v", err)
	}
	if len(pods.Items) != 1 {
		return fmt.Errorf("there are more than 1 pod found for the job")
	}

	// Get logs of the Pod
	logReader, err := opt.CoreV1().Pods(opt.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).Stream(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get pod logs: %+v", err)
	}
	defer logReader.Close()
	if _, err := io.Copy(os.Stdout, logReader); err != nil {
		return fmt.Errorf("failed to read log: %+v", err)
	}
	return nil
}

func deletecrb() {
	if err := opt.RbacV1().ClusterRoleBindings().Delete(context.Background(), "rukpakctl-crb", metav1.DeleteOptions{}); err != nil {
		fmt.Printf("failed to delete clusterrolebinding: %+v", err)
	}
}
