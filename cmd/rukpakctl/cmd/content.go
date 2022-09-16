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
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

type options struct {
	*kubernetes.Clientset
	runtimeclient.Client
	namespace string
}

func newContentCmd() *cobra.Command {
	var opt options

	contentCmd := &cobra.Command{
		Use:   "content <bundle name>",
		Short: "display contents of the specified bundle.",
		Long:  `display contents of the specified bundle.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			sch := runtime.NewScheme()
			if err := rukpakv1alpha1.AddToScheme(sch); err != nil {
				log.Fatalf("failed to add rukpak types to scheme: %v", err)
			}

			cfg, err := config.GetConfig()
			if err != nil {
				log.Fatalf("failed to load kubeconfig: %v", err)
			}

			opt.Client, err = runtimeclient.New(cfg, runtimeclient.Options{
				Scheme: sch,
			})
			if err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			if opt.Clientset, err = kubernetes.NewForConfig(cfg); err != nil {
				log.Fatalf("failed to create kubernetes client: %v", err)
			}

			if err := content(cmd.Context(), opt, args); err != nil {
				log.Fatalf("content command failed: %v", err)
			}
		},
	}
	contentCmd.Flags().StringVar(&opt.namespace, "namespace", util.DefaultSystemNamespace, "namespace to run content query job.")
	return contentCmd
}

func content(ctx context.Context, opt options, args []string) error {
	// Create a temporary ClusterRoleBinding to bind the ServiceAccount to bundle-reader ClusterRole
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rukpakctl-crb",
			Namespace: opt.namespace,
		},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "rukpakctl-sa", Namespace: opt.namespace}},
		RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "bundle-reader"},
	}
	crb, err := opt.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a cluster role bindings: %v", err)
	}
	defer deletecrb(ctx, opt.Clientset)

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
	_, err = opt.CoreV1().ServiceAccounts(opt.namespace).Create(ctx, &sa, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a service account: %v", err)
	}

	bundle := &rukpakv1alpha1.Bundle{}
	err = opt.Get(ctx, runtimeclient.ObjectKey{Name: args[0]}, bundle)
	if err != nil {
		return err
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
	job, err = opt.BatchV1().Jobs(opt.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create a job: %v", err)
	}

	// Wait for Job completion
	if err := wait.PollImmediateUntil(time.Second, func() (bool, error) {
		deployedJob, err := opt.BatchV1().Jobs(opt.namespace).Get(ctx, job.ObjectMeta.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to get a job: %v", err)
		}
		return deployedJob.Status.CompletionTime != nil, nil
	}, ctx.Done()); err != nil {
		return fmt.Errorf("failed waiting for job to complete: %v", err)
	}

	// Get Pod for the Job
	podSelector := labels.Set{"job-name": job.Name}.AsSelector()
	pods, err := opt.CoreV1().Pods(opt.namespace).List(ctx, metav1.ListOptions{LabelSelector: podSelector.String()})
	if err != nil {
		return fmt.Errorf("failed to list pods for job: %v", err)
	}
	const expectedPods = 1
	if len(pods.Items) != expectedPods {
		return fmt.Errorf("unexpected number of pods found for job: expected %d, found %d", expectedPods, len(pods.Items))
	}

	// Get logs of the Pod
	logReader, err := opt.CoreV1().Pods(opt.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pod logs: %v", err)
	}
	defer logReader.Close()
	if _, err := io.Copy(os.Stdout, logReader); err != nil {
		return fmt.Errorf("failed to read log: %v", err)
	}
	return nil
}

func deletecrb(ctx context.Context, kube kubernetes.Interface) {
	if err := kube.RbacV1().ClusterRoleBindings().Delete(ctx, "rukpakctl-crb", metav1.DeleteOptions{}); err != nil {
		fmt.Printf("failed to delete clusterrolebinding: %v", err)
	}
}
